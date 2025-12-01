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
	httppollServerChannelSize    = 100
	packetTypeSize               = 1
	packetBodySizeBytes          = 4
)

// ServerHTTPLongPollingConn 服务器端 HTTP 长轮询连接
// 实现 net.Conn 接口，将 HTTP 请求/响应转换为双向流
type ServerHTTPLongPollingConn struct {
	*dispose.ManagerBase

	clientID  int64
	mappingID string // 映射ID（隧道连接才有，指令通道为空）

	// Base64 数据通道（接收 Base64 编码的数据，来自 HTTP POST）
	base64PushDataChan chan string

	// 下行数据（服务器 → 客户端）：优先级队列（解决心跳包干扰问题）
	pollDataQueue *PriorityQueue
	pollDataChan  chan []byte // 用于 PollData 的阻塞 channel（单元素 channel，用于阻塞等待）
	pollSeq       uint64
	pollMu        sync.Mutex
	pollWaitChan  chan struct{} // 用于通知 PollData 有数据可用（非阻塞信号）

	// 读取缓冲区（处理部分读取）
	readBuffer []byte
	readBufMu  sync.Mutex

	// 写入缓冲区（缓冲多次 Write 调用，直到完整包）
	writeBuffer bytes.Buffer
	writeBufMu  sync.Mutex
	writeFlush  chan struct{} // 触发刷新缓冲区

	// ConnectionID（唯一标识，在创建时就确定，不会改变）
	connectionID string
	connectionMu sync.RWMutex

	// 控制
	closed  bool
	closeMu sync.RWMutex

	// 流模式标志（隧道建立后切换到流模式，不再解析数据包格式）
	streamMode bool
	streamMu   sync.RWMutex

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
		pollDataQueue:      NewPriorityQueue(3),    // 最多缓存3个心跳包
		pollDataChan:       make(chan []byte, 1),   // 单元素 channel，用于阻塞等待
		pollWaitChan:       make(chan struct{}, 1), // 非阻塞信号，通知 PollData 有数据可用
		writeFlush:         make(chan struct{}, 1),
		localAddr:          &httppollServerAddr{network: "httppoll", addr: "server"},
		remoteAddr:         &httppollServerAddr{network: "httppoll", addr: strconv.FormatInt(clientID, 10)},
	}

	conn.AddCleanHandler(conn.onClose)

	// 启动写入刷新循环
	go conn.writeFlushLoop()

	// 启动优先级队列调度循环
	go conn.pollDataScheduler()

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
	// ✅ 在流模式下，限制每次最多读取一个 MySQL 协议包的大小，避免跨包读取
	// MySQL 协议包结构：前3字节是包长度（小端序），第4字节是序列号
	c.streamMu.RLock()
	streamMode := c.streamMode
	c.streamMu.RUnlock()

	connID := c.GetConnectionID()
	utils.Infof("HTTP long polling: [READ] entry, streamMode=%v, readBuffer len=%d, requested=%d, clientID=%d, connID=%s",
		streamMode, len(c.readBuffer), len(p), c.clientID, connID)

	if streamMode && len(c.readBuffer) >= 4 {
		// 解析 MySQL 协议包长度（前3字节，小端序）
		packetLength := int(c.readBuffer[0]) | int(c.readBuffer[1])<<8 | int(c.readBuffer[2])<<16
		// MySQL 协议包总大小 = 3字节长度 + 1字节序列号 + 包体长度
		packetSize := 4 + packetLength

		// 验证包长度是否合理（防止解析错误）
		// MySQL 协议包最大为 16MB (2^24 - 1)，但实际使用中通常不会超过 64KB
		if packetLength > 0 && packetLength <= 16*1024*1024 {
			// 如果缓冲区有完整包，只读取一个包
			if len(c.readBuffer) >= packetSize {
				readSize := packetSize
				if readSize > len(p) {
					readSize = len(p)
				}
				n := copy(p[:readSize], c.readBuffer[:readSize])
				c.readBuffer = c.readBuffer[n:]
				utils.Infof("HTTP long polling: [READ] read %d bytes (MySQL packet, length=%d, remaining: %d), clientID=%d, connID=%s",
					n, packetLength, len(c.readBuffer), c.clientID, c.GetConnectionID())
				return n, nil
			}
			// ✅ 如果缓冲区没有完整包，尝试从 base64PushDataChan 接收更多数据（非阻塞）
			// 如果 channel 为空，立即返回部分数据，避免阻塞导致超时
			utils.Debugf("HTTP long polling: [READ] incomplete MySQL packet (need %d bytes, have %d), trying to get more data, clientID=%d, connID=%s",
				packetSize, len(c.readBuffer), c.clientID, c.GetConnectionID())
			// 尝试从 base64PushDataChan 接收更多数据（非阻塞）
			select {
			case base64Data, ok := <-c.base64PushDataChan:
				if !ok {
					// channel 已关闭，返回 EOF
					return 0, io.EOF
				}
				// Base64 解码
				data, err := base64.StdEncoding.DecodeString(base64Data)
				if err != nil {
					utils.Errorf("HTTP long polling: [READ] failed to decode Base64 data: %v, clientID=%d", err, c.clientID)
					return 0, fmt.Errorf("failed to decode Base64 data: %w", err)
				}
				// 追加到 readBuffer
				c.readBuffer = append(c.readBuffer, data...)
				utils.Debugf("HTTP long polling: [READ] received %d bytes from channel, buffer size now: %d, clientID=%d",
					len(data), len(c.readBuffer), c.clientID)
				// 重新检查是否有完整包
				if len(c.readBuffer) >= packetSize {
					readSize := packetSize
					if readSize > len(p) {
						readSize = len(p)
					}
					n := copy(p[:readSize], c.readBuffer[:readSize])
					c.readBuffer = c.readBuffer[n:]
					utils.Infof("HTTP long polling: [READ] read %d bytes (MySQL packet, length=%d, remaining: %d), clientID=%d, connID=%s",
						n, packetLength, len(c.readBuffer), c.clientID, c.GetConnectionID())
					return n, nil
				}
				// 仍然没有完整包，返回部分数据（避免阻塞）
				readSize := len(c.readBuffer)
				if readSize > len(p) {
					readSize = len(p)
				}
				n := copy(p[:readSize], c.readBuffer[:readSize])
				c.readBuffer = c.readBuffer[n:]
				utils.Debugf("HTTP long polling: [READ] read %d bytes (partial packet, remaining: %d), clientID=%d",
					n, len(c.readBuffer), c.clientID)
				return n, nil
			default:
				// channel 为空，立即返回部分数据，避免阻塞
				readSize := len(c.readBuffer)
				if readSize > len(p) {
					readSize = len(p)
				}
				n := copy(p[:readSize], c.readBuffer[:readSize])
				c.readBuffer = c.readBuffer[n:]
				utils.Debugf("HTTP long polling: [READ] read %d bytes (partial packet, channel empty, remaining: %d), clientID=%d",
					n, len(c.readBuffer), c.clientID)
				return n, nil
			}
		}
	}

	// 如果缓冲区数据不足4字节，或者无法解析包长度，按原逻辑读取
	if len(c.readBuffer) > 0 {
		readSize := len(c.readBuffer)
		if readSize > len(p) {
			readSize = len(p)
		}
		// ✅ 限制最大读取大小为 64KB，避免将多个 HTTP push 请求合并成一个大的读取
		maxReadSize := 64 * 1024
		if readSize > maxReadSize {
			readSize = maxReadSize
		}
		n := copy(p[:readSize], c.readBuffer[:readSize])
		c.readBuffer = c.readBuffer[n:]
		utils.Debugf("HTTP long polling: [READ] read %d bytes from buffer (remaining: %d), clientID=%d",
			n, len(c.readBuffer), c.clientID)
		return n, nil
	}

	// 缓冲区为空，从 base64PushDataChan 接收 Base64 数据并解码
	utils.Infof("HTTP long polling: [READ] waiting for Base64 data from base64PushDataChan, clientID=%d, connID=%s, channel len=%d, cap=%d",
		c.clientID, c.GetConnectionID(), len(c.base64PushDataChan), cap(c.base64PushDataChan))
	// 注意：这里不使用超时，因为 ReadPacket 会持续调用 Read，直到读取完整数据包
	// 如果 channel 为空，应该阻塞等待，而不是返回 EOF
	select {
	case <-c.Ctx().Done():
		utils.Infof("HTTP long polling: [READ] context canceled, clientID=%d, connID=%s", c.clientID, c.GetConnectionID())
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

		utils.Infof("HTTP long polling: [READ] decoded %d bytes from Base64 data, clientID=%d, connID=%s",
			len(data), c.clientID, c.GetConnectionID())

		// 追加到 readBuffer
		c.readBuffer = append(c.readBuffer, data...)

		// 从 readBuffer 读取
		n := copy(p, c.readBuffer)
		c.readBuffer = c.readBuffer[n:]
		firstByte := byte(0)
		if len(c.readBuffer) > 0 {
			firstByte = c.readBuffer[0]
		}
		utils.Infof("HTTP long polling: [READ] read %d bytes (remaining in buffer: %d), clientID=%d, connID=%s, firstByte=0x%02x",
			n, len(c.readBuffer), c.clientID, c.GetConnectionID(), firstByte)
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

	// 检查是否已切换到流模式
	c.streamMu.RLock()
	streamMode := c.streamMode
	c.streamMu.RUnlock()

	// 流模式下，直接转发数据，不缓冲（但过滤心跳包）
	if streamMode {
		// ✅ 过滤心跳包：心跳包应该只通过控制连接发送，不应该通过隧道连接
		// 如果数据是1字节且是心跳包类型，且是隧道连接（mappingID 不为空），则丢弃
		if len(p) == 1 {
			packetType := packet.Type(p[0])
			if packetType.IsHeartbeat() {
				// 如果是隧道连接，心跳包不应该出现在这里
				if c.mappingID != "" {
					utils.Errorf("HTTP long polling: [WRITE] stream mode: detected heartbeat packet in tunnel connection (mappingID=%s), dropping it, clientID=%d",
						c.mappingID, c.clientID)
					return len(p), nil // 丢弃心跳包，但返回成功
				}
				// 控制连接的心跳包，正常处理
				utils.Debugf("HTTP long polling: [WRITE] stream mode: heartbeat packet on control connection (0x%02x), clientID=%d",
					p[0], c.clientID)
			}
		}

		firstByte := byte(0)
		if len(p) > 0 {
			firstByte = p[0]
		}
		utils.Infof("HTTP long polling: [WRITE] stream mode: pushing %d bytes directly to priority queue, clientID=%d, mappingID=%s, firstByte=0x%02x",
			len(p), c.clientID, c.mappingID, firstByte)
		c.pollDataQueue.Push(p)

		// 立即通知 pollDataScheduler 有数据可用（非阻塞）
		select {
		case c.pollWaitChan <- struct{}{}:
		default:
		}
		return len(p), nil
	}

	// 非流模式：将数据写入缓冲区
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

		// 检查是否已切换到流模式
		c.streamMu.RLock()
		streamMode := c.streamMode
		c.streamMu.RUnlock()

		// 检查缓冲区是否有完整包
		c.writeBufMu.Lock()
		bufLen := c.writeBuffer.Len()

		// 如果已切换到流模式，直接转发原始数据，不再解析数据包格式
		if streamMode && bufLen > 0 {
			data := make([]byte, bufLen)
			copy(data, c.writeBuffer.Bytes())
			c.writeBuffer.Reset()
			c.writeBufMu.Unlock()

			utils.Debugf("HTTP long polling: [WRITE_FLUSH_LOOP] stream mode: pushing %d bytes directly to priority queue, clientID=%d, mappingID=%s",
				len(data), c.clientID, c.mappingID)
			c.pollDataQueue.Push(data)

			// 立即通知 pollDataScheduler 有数据可用（非阻塞）
			select {
			case c.pollWaitChan <- struct{}{}:
			default:
			}
			continue
		}

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

				utils.Infof("HTTP long polling: [WRITE_FLUSH_LOOP] pushing heartbeat packet (0x%02x) to priority queue, clientID=%d", data[0], c.clientID)
				// 心跳包推入优先级队列（会被合并/丢弃多余的心跳包）
				c.pollDataQueue.Push(data)
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

				// 发送完整数据包到优先级队列（优先级队列会自动判断优先级）
				utils.Infof("HTTP long polling: [WRITE_FLUSH_LOOP] pushing complete packet to priority queue, size=%d, clientID=%d, mappingID=%s",
					packetSize, c.clientID, c.mappingID)
				c.pollDataQueue.Push(data)

				// 立即通知 pollDataScheduler 有数据可用（非阻塞）
				select {
				case c.pollWaitChan <- struct{}{}:
				default:
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

// pollDataScheduler 优先级队列调度循环（将队列中的数据推送到 pollDataChan）
func (c *ServerHTTPLongPollingConn) pollDataScheduler() {
	utils.Infof("HTTP long polling: [POLLDATA_SCHEDULER] started, clientID=%d", c.clientID)
	ticker := time.NewTicker(10 * time.Millisecond) // 每10ms检查一次队列
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			utils.Debugf("HTTP long polling: [POLLDATA_SCHEDULER] context canceled, clientID=%d", c.clientID)
			return
		case <-ticker.C:
			// 定期检查队列，如果有数据且 pollDataChan 为空，则推送
			// 持续推送直到队列为空或 channel 满
			for {
				data, ok := c.pollDataQueue.Pop()
				if !ok {
					break // 队列为空
				}
				select {
				case <-c.Ctx().Done():
					// 如果 context 取消，将数据放回队列
					c.pollDataQueue.Push(data)
					return
				case c.pollDataChan <- data:
					utils.Infof("HTTP long polling: [POLLDATA_SCHEDULER] pushed %d bytes to pollDataChan, queueLen=%d, clientID=%d, mappingID=%s",
						len(data), c.pollDataQueue.Len(), c.clientID, c.mappingID)
					// 通知 PollData 有数据可用（非阻塞）
					select {
					case c.pollWaitChan <- struct{}{}:
					default:
					}
					// 继续推送下一个数据包
				default:
					// pollDataChan 已满（有数据正在等待），将数据放回队列（保持优先级）
					c.pollDataQueue.Push(data)
					break // 退出内层循环，等待下次 tick
				}
			}
		}
	}
}

// PollData 等待数据用于 HTTP GET 响应（由 handleHTTPPoll 调用）
// 返回 Base64 编码的数据，按照 Base64 适配层设计
func (c *ServerHTTPLongPollingConn) PollData(ctx context.Context) (string, error) {
	queueLen := c.pollDataQueue.Len()
	utils.Infof("HTTP long polling: [POLLDATA] waiting for data, clientID=%d, queueLen=%d",
		c.clientID, queueLen)

	// 先检查队列中是否有数据（非阻塞）
	if data, ok := c.pollDataQueue.Pop(); ok {
		utils.Infof("HTTP long polling: [POLLDATA] received %d bytes from queue, encoding to Base64, clientID=%d",
			len(data), c.clientID)
		base64Data := base64.StdEncoding.EncodeToString(data)
		return base64Data, nil
	}

	// 队列为空，阻塞等待调度器推送数据
	// 使用 select 同时监听 pollDataChan 和 pollWaitChan
	select {
	case <-ctx.Done():
		utils.Debugf("HTTP long polling: [POLLDATA] context canceled, clientID=%d", c.clientID)
		return "", ctx.Err()
	case <-c.Ctx().Done():
		utils.Debugf("HTTP long polling: [POLLDATA] connection context canceled, clientID=%d", c.clientID)
		return "", c.Ctx().Err()
	case <-c.pollWaitChan:
		// 收到信号，立即检查队列（可能有数据被调度器推送）
		if data, ok := c.pollDataQueue.Pop(); ok {
			utils.Infof("HTTP long polling: [POLLDATA] received %d bytes from queue (after signal), encoding to Base64, clientID=%d",
				len(data), c.clientID)
			base64Data := base64.StdEncoding.EncodeToString(data)
			return base64Data, nil
		}
		// 如果队列仍为空，继续等待 pollDataChan
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-c.Ctx().Done():
			return "", c.Ctx().Err()
		case data, ok := <-c.pollDataChan:
			if !ok {
				return "", io.EOF
			}
			utils.Infof("HTTP long polling: [POLLDATA] received %d bytes from channel, encoding to Base64, clientID=%d",
				len(data), c.clientID)
			base64Data := base64.StdEncoding.EncodeToString(data)
			return base64Data, nil
		}
	case data, ok := <-c.pollDataChan:
		if !ok {
			utils.Debugf("HTTP long polling: [POLLDATA] channel closed, clientID=%d", c.clientID)
			return "", io.EOF
		}
		utils.Infof("HTTP long polling: [POLLDATA] received %d bytes from channel, encoding to Base64, clientID=%d",
			len(data), c.clientID)

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

// GetMappingID 获取映射ID（隧道连接才有，指令通道返回空字符串）
func (c *ServerHTTPLongPollingConn) GetMappingID() string {
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	return c.mappingID
}

// SetMappingID 设置映射ID（隧道连接才有）
func (c *ServerHTTPLongPollingConn) SetMappingID(mappingID string) {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	c.mappingID = mappingID
	utils.Infof("HTTP long polling: [SetMappingID] setting mappingID=%s, clientID=%d, connID=%s",
		mappingID, c.clientID, c.GetConnectionID())
}

// SetStreamMode 切换到流模式（隧道建立后调用）
func (c *ServerHTTPLongPollingConn) SetStreamMode(streamMode bool) {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	oldMode := c.streamMode
	c.streamMode = streamMode
	utils.Infof("HTTP long polling: [SetStreamMode] switching stream mode from %v to %v, clientID=%d, mappingID=%s",
		oldMode, streamMode, c.clientID, c.mappingID)
}

// IsStreamMode 检查是否处于流模式
func (c *ServerHTTPLongPollingConn) IsStreamMode() bool {
	c.streamMu.RLock()
	defer c.streamMu.RUnlock()
	return c.streamMode
}

// ShouldKeepInConnMap 判断是否应该保留在 connMap 中
// HTTP 长轮询连接需要保留，因为读取循环还在运行
func (c *ServerHTTPLongPollingConn) ShouldKeepInConnMap() bool {
	return true
}

// CanCreateTemporaryControlConn 判断是否可以创建临时控制连接
// HTTP 长轮询隧道连接可能没有注册为控制连接，可以创建临时控制连接
func (c *ServerHTTPLongPollingConn) CanCreateTemporaryControlConn() bool {
	return true
}

// SetConnectionID 设置连接 ID（唯一标识，在创建时就确定）
func (c *ServerHTTPLongPollingConn) SetConnectionID(connID string) {
	c.connectionMu.Lock()
	defer c.connectionMu.Unlock()
	c.connectionID = connID
}

// GetConnectionID 获取连接 ID
func (c *ServerHTTPLongPollingConn) GetConnectionID() string {
	c.connectionMu.RLock()
	defer c.connectionMu.RUnlock()
	return c.connectionID
}

// OnHandshakeComplete 握手完成回调（统一接口）
// 当握手成功且 clientID > 0 时，自动调用此方法
func (c *ServerHTTPLongPollingConn) OnHandshakeComplete(clientID int64) {
	c.UpdateClientID(clientID)
}

// UpdateClientID 更新客户端 ID（握手后调用）
// 注意：ConnectionID 不变，只更新 clientID
func (c *ServerHTTPLongPollingConn) UpdateClientID(newClientID int64) {
	c.closeMu.Lock()
	oldClientID := c.clientID
	c.clientID = newClientID
	c.closeMu.Unlock()

	utils.Infof("HTTP long polling: [UpdateClientID] updated clientID from %d to %d, connID=%s",
		oldClientID, newClientID, c.GetConnectionID())
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
