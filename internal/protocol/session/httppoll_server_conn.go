package session

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

const (
	httppollServerDefaultTimeout = 30 * time.Second
	httppollServerMaxTimeout     = 60 * time.Second
	httppollServerChannelSize   = 100
	packetTypeSize             = 1
	packetBodySizeBytes        = 4
)

// ServerHTTPLongPollingConn 服务器端 HTTP 长轮询连接
// 实现 net.Conn 接口，将 HTTP 请求/响应转换为双向流
type ServerHTTPLongPollingConn struct {
	*dispose.ManagerBase

	clientID int64

	// Base64 数据通道（接收 Base64 编码的数据，来自 HTTP POST）
	base64PushDataChan chan string

	// 下行数据（服务器 → 客户端）：完整数据包（字节流）
	pollDataChan chan []byte
	pollSeq      uint64
	pollMu       sync.Mutex

	// 读取缓冲区（处理部分读取）
	readBuffer []byte
	readBufMu  sync.Mutex

	// 写入缓冲区（缓冲多次 Write 调用，直到完整包）
	writeBuffer bytes.Buffer
	writeBufMu  sync.Mutex
	writeFlush  chan struct{} // 触发刷新缓冲区

	// 连接迁移回调（当 clientID 从 0 变为非 0 时自动调用）
	migrationCallback func(connID string, oldClientID, newClientID int64)
	connectionID      string
	migrationMu        sync.RWMutex

	// 控制
	closed  bool
	closeMu sync.RWMutex

	// 地址信息（用于实现 net.Conn 接口）
	localAddr  net.Addr
	remoteAddr net.Addr
}

// NewServerHTTPLongPollingConn 创建服务器端 HTTP 长轮询连接
func NewServerHTTPLongPollingConn(ctx context.Context, clientID int64) *ServerHTTPLongPollingConn {
	conn := &ServerHTTPLongPollingConn{
		ManagerBase:        dispose.NewManager("ServerHTTPLongPollingConn", ctx),
		clientID:           clientID,
		base64PushDataChan: make(chan string, httppollServerChannelSize),
		pollDataChan:       make(chan []byte, httppollServerChannelSize),
		writeFlush:         make(chan struct{}, 1),
		localAddr:          &httppollServerAddr{network: "httppoll", addr: "server"},
		remoteAddr:         &httppollServerAddr{network: "httppoll", addr: strconv.FormatInt(clientID, 10)},
	}

	conn.AddCleanHandler(conn.onClose)
	
	// 启动写入刷新循环
	go conn.writeFlushLoop()
	
	return conn
}

// onClose 资源清理
func (c *ServerHTTPLongPollingConn) onClose() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	close(c.base64PushDataChan)
	close(c.pollDataChan)

	return nil
}

// Read 实现 io.Reader（从 HTTP POST 读取数据）
func (c *ServerHTTPLongPollingConn) Read(p []byte) (int, error) {
	c.closeMu.RLock()
	closed := c.closed
	c.closeMu.RUnlock()

	if closed {
		return 0, io.EOF
	}

	c.readBufMu.Lock()
	defer c.readBufMu.Unlock()

	// 先检查缓冲区是否有数据
	if len(c.readBuffer) > 0 {
		n := copy(p, c.readBuffer)
		c.readBuffer = c.readBuffer[n:]
		utils.Debugf("HTTP long polling: [READ] read %d bytes from buffer (remaining: %d), clientID=%d", 
			n, len(c.readBuffer), c.clientID)
		return n, nil
	}

	// 缓冲区为空，从 base64PushDataChan 接收 Base64 数据并解码
	utils.Debugf("HTTP long polling: [READ] waiting for Base64 data from base64PushDataChan, clientID=%d", c.clientID)
	// 注意：这里不使用超时，因为 ReadPacket 会持续调用 Read，直到读取完整数据包
	// 如果 channel 为空，应该阻塞等待，而不是返回 EOF
	select {
	case <-c.Ctx().Done():
		utils.Debugf("HTTP long polling: [READ] context canceled, clientID=%d", c.clientID)
		return 0, c.Ctx().Err()
	case base64Data, ok := <-c.base64PushDataChan:
		if !ok {
			utils.Debugf("HTTP long polling: [READ] base64PushDataChan closed, clientID=%d", c.clientID)
			return 0, io.EOF
		}
		
		// Base64 解码
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			utils.Errorf("HTTP long polling: [READ] failed to decode Base64 data: %v, clientID=%d", err, c.clientID)
			return 0, fmt.Errorf("failed to decode Base64 data: %w", err)
		}
		
		utils.Infof("HTTP long polling: [READ] decoded %d bytes from Base64 data, clientID=%d", 
			len(data), c.clientID)
		
		// 追加到 readBuffer
		c.readBuffer = append(c.readBuffer, data...)
		
		// 从 readBuffer 读取
		n := copy(p, c.readBuffer)
		c.readBuffer = c.readBuffer[n:]
		utils.Debugf("HTTP long polling: [READ] read %d bytes (remaining in buffer: %d), clientID=%d", 
			n, len(c.readBuffer), c.clientID)
		return n, nil
	}
}

// Write 实现 io.Writer（通过 HTTP GET 响应发送数据）
// 注意：StreamProcessor.WritePacket() 会多次调用 Write()（包类型、包体大小、包体）
// 我们需要缓冲这些数据，直到收到完整的包后再发送
func (c *ServerHTTPLongPollingConn) Write(p []byte) (int, error) {
	c.closeMu.RLock()
	closed := c.closed
	c.closeMu.RUnlock()

	if closed {
		utils.Warnf("HTTP long polling: [WRITE] connection closed, clientID=%d", c.clientID)
		return 0, io.ErrClosedPipe
	}

	// 将数据写入缓冲区
	c.writeBufMu.Lock()
	n, err := c.writeBuffer.Write(p)
	bufLen := c.writeBuffer.Len()
	c.writeBufMu.Unlock()

	utils.Debugf("HTTP long polling: [WRITE] writing %d bytes to buffer, bufferLen=%d, clientID=%d", 
		len(p), bufLen, c.clientID)

	if err != nil {
		return 0, err
	}

	// 触发刷新检查（非阻塞）
	select {
	case c.writeFlush <- struct{}{}:
	default:
	}

	return n, nil
}

// writeFlushLoop 写入刷新循环（检查完整包并发送）
func (c *ServerHTTPLongPollingConn) writeFlushLoop() {
	utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] started, clientID=%d", c.clientID)
	ticker := time.NewTicker(50 * time.Millisecond) // 每50ms检查一次
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] context canceled, clientID=%d", c.clientID)
			return
		case <-ticker.C:
			// 定期检查缓冲区
		case <-c.writeFlush:
			// 收到刷新信号，立即检查
			utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] received flush signal, clientID=%d", c.clientID)
		}

		// 检查缓冲区是否有完整包
		c.writeBufMu.Lock()
		bufLen := c.writeBuffer.Len()
		
		// 特殊处理：心跳包只有 1 字节（包类型），没有包体大小和包体
		// 如果缓冲区有数据，先检查是否是心跳包
		if bufLen >= 1 {
			bufData := c.writeBuffer.Bytes()
			packetType := packet.Type(bufData[0])
			// 检查是否是心跳包（忽略压缩/加密标志）
			if packetType.IsHeartbeat() {
				// 心跳包只有 1 字节，直接发送
				data := make([]byte, 1)
				copy(data, bufData[:1])
				c.writeBuffer.Next(1)
				c.writeBufMu.Unlock()
				
				utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] sending heartbeat packet (0x%02x), clientID=%d", data[0], c.clientID)
				select {
				case <-c.Ctx().Done():
					utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] context canceled, clientID=%d", c.clientID)
					return
				case c.pollDataChan <- data:
					utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] heartbeat packet sent successfully, clientID=%d", c.clientID)
				}
				continue
			}
		}
		
		if bufLen >= 5 {
			// 至少有一个包类型（1字节）+ 包体大小（4字节）
			bufData := c.writeBuffer.Bytes()
			
			// 解析包体大小（大端序，从第2到第5字节，即索引1-4）
			if len(bufData) < 5 {
				c.writeBufMu.Unlock()
				continue
			}
			
			bodySize := binary.BigEndian.Uint32(bufData[1:5])
			
			// 计算完整包大小：1字节类型 + 4字节大小 + bodySize
			packetSize := 5 + int(bodySize)
			
			// 验证包体大小是否合理（防止解析错误导致无限等待）
			if bodySize > 10*1024*1024 { // 10MB 上限
				utils.Errorf("HTTP long polling: [WRITE_FLUSH_LOOP] invalid bodySize=%d (too large), resetting buffer, clientID=%d", 
					bodySize, c.clientID)
				c.writeBuffer.Reset()
				c.writeBufMu.Unlock()
				continue
			}
			
			utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] checking buffer, bufLen=%d, bodySize=%d, packetSize=%d, clientID=%d", 
				bufLen, bodySize, packetSize, c.clientID)
			
			if bufLen >= packetSize {
				// 有完整包，提取并发送
				data := make([]byte, packetSize)
				copy(data, bufData[:packetSize])
				c.writeBuffer.Next(packetSize)
				c.writeBufMu.Unlock()
				
				// 发送完整数据包到 pollDataChan
				utils.Infof("HTTP long polling: [WRITE_FLUSH_LOOP] sending complete packet, size=%d, clientID=%d", 
					packetSize, c.clientID)
				select {
				case <-c.Ctx().Done():
					utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] context canceled, clientID=%d", c.clientID)
					return
				case c.pollDataChan <- data:
					utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] packet sent successfully, clientID=%d", c.clientID)
				}
			} else {
				// 数据不完整，等待更多数据
				c.writeBufMu.Unlock()
			}
		} else {
			c.writeBufMu.Unlock()
		}
	}
}

// Close 实现 io.Closer
func (c *ServerHTTPLongPollingConn) Close() error {
	return c.Dispose.CloseWithError()
}

// LocalAddr 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// PushData 从 HTTP POST 请求接收 Base64 编码的数据（由 handleHTTPPush 调用）
// 按照 Base64 适配层设计：Base64 数据直接发送到 base64PushDataChan
// Read() 方法会从 base64PushDataChan 接收并解码，追加到 readBuffer
func (c *ServerHTTPLongPollingConn) PushData(base64Data string) error {
	c.closeMu.RLock()
	closed := c.closed
	c.closeMu.RUnlock()

	if closed {
		utils.Warnf("HTTP long polling: [PUSHDATA] connection closed, clientID=%d", c.clientID)
		return io.ErrClosedPipe
	}

	utils.Infof("HTTP long polling: [PUSHDATA] pushing Base64 data (len=%d) to base64PushDataChan, clientID=%d", 
		len(base64Data), c.clientID)
	select {
	case <-c.Ctx().Done():
		utils.Warnf("HTTP long polling: [PUSHDATA] context canceled, clientID=%d", c.clientID)
		return c.Ctx().Err()
	case c.base64PushDataChan <- base64Data:
		utils.Debugf("HTTP long polling: [PUSHDATA] Base64 data pushed successfully, clientID=%d", c.clientID)
		return nil
	default:
		utils.Errorf("HTTP long polling: [PUSHDATA] base64PushDataChan full, clientID=%d", c.clientID)
		return io.ErrShortWrite
	}
}

// PollData 等待数据用于 HTTP GET 响应（由 handleHTTPPoll 调用）
// 返回 Base64 编码的数据，按照 Base64 适配层设计
func (c *ServerHTTPLongPollingConn) PollData(ctx context.Context) (string, error) {
	utils.Debugf("HTTP long polling: [POLLDATA] waiting for data, clientID=%d, pollDataChan len=%d", 
		c.clientID, len(c.pollDataChan))
	select {
	case <-ctx.Done():
		utils.Debugf("HTTP long polling: [POLLDATA] context canceled, clientID=%d", c.clientID)
		return "", ctx.Err()
	case <-c.Ctx().Done():
		utils.Debugf("HTTP long polling: [POLLDATA] connection context canceled, clientID=%d", c.clientID)
		return "", c.Ctx().Err()
	case data, ok := <-c.pollDataChan:
		if !ok {
			utils.Debugf("HTTP long polling: [POLLDATA] channel closed, clientID=%d", c.clientID)
			return "", io.EOF
		}
		utils.Infof("HTTP long polling: [POLLDATA] received %d bytes, encoding to Base64, clientID=%d", len(data), c.clientID)
		
		// Base64 编码
		base64Data := base64.StdEncoding.EncodeToString(data)
		return base64Data, nil
	}
}

// GetClientID 获取客户端 ID
func (c *ServerHTTPLongPollingConn) GetClientID() int64 {
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	return c.clientID
}

// SetConnectionID 设置连接 ID（用于迁移回调）
func (c *ServerHTTPLongPollingConn) SetConnectionID(connID string) {
	c.migrationMu.Lock()
	defer c.migrationMu.Unlock()
	c.connectionID = connID
}

// SetMigrationCallback 设置迁移回调函数
// 当 clientID 从 0 变为非 0 时，会自动调用此回调
func (c *ServerHTTPLongPollingConn) SetMigrationCallback(callback func(connID string, oldClientID, newClientID int64)) {
	c.migrationMu.Lock()
	defer c.migrationMu.Unlock()
	c.migrationCallback = callback
}

// OnHandshakeComplete 握手完成回调（统一接口）
// 当握手成功且 clientID > 0 时，自动调用此方法
func (c *ServerHTTPLongPollingConn) OnHandshakeComplete(clientID int64) {
	c.UpdateClientID(clientID)
}

// UpdateClientID 更新客户端 ID（握手后调用）
// 如果从临时连接（0）迁移到正式连接（非0），自动触发迁移回调
func (c *ServerHTTPLongPollingConn) UpdateClientID(newClientID int64) {
	c.closeMu.Lock()
	oldClientID := c.clientID
	c.clientID = newClientID
	c.closeMu.Unlock()

	utils.Infof("HTTP long polling: [UpdateClientID] updated clientID from %d to %d", oldClientID, newClientID)

	// 自动触发迁移：从临时连接（0）迁移到正式连接（非0）
	if oldClientID == 0 && newClientID > 0 {
		c.migrationMu.RLock()
		connID := c.connectionID
		callback := c.migrationCallback
		c.migrationMu.RUnlock()

		if callback != nil && connID != "" {
			utils.Debugf("HTTP long polling: [UpdateClientID] triggering migration callback, connID=%s, oldClientID=%d, newClientID=%d", 
				connID, oldClientID, newClientID)
			callback(connID, oldClientID, newClientID)
		} else if connID == "" {
			utils.Warnf("HTTP long polling: [UpdateClientID] migration callback not triggered: connectionID not set")
		}
	}
}

// httppollServerAddr 实现 net.Addr 接口
type httppollServerAddr struct {
	network string
	addr    string
}

func (a *httppollServerAddr) Network() string {
	return a.network
}

func (a *httppollServerAddr) String() string {
	return a.addr
}

