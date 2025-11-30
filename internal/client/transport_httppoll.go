package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

const (
	httppollDefaultPushTimeout = 30 * time.Second
	httppollDefaultPollTimeout = 30 * time.Second
	httppollMaxRetries         = 3
	httppollRetryInterval      = 1 * time.Second
	httppollMaxRequestSize     = 1024 * 1024 // 1MB
)

// HTTPLongPollingConn HTTP 长轮询连接
// 实现 net.Conn 接口，用于与 StreamProcessor 集成
type HTTPLongPollingConn struct {
	*dispose.ManagerBase

	baseURL  string
	clientID int64
	token    string

	// 上行连接（发送数据）
	pushURL    string
	pushClient *http.Client
	pushSeq    uint64
	pushMu     sync.Mutex

	// 下行连接（接收数据）
	pollURL    string
	pollClient *http.Client
	pollSeq    uint64
	pollMu     sync.Mutex

	// Base64 数据通道（接收 Base64 编码的数据）
	base64DataChan chan string

	// 读取缓冲区（字节流缓冲区，Base64 解码后的数据追加到这里）
	readBuffer []byte
	readBufMu  sync.Mutex

	// 写入缓冲区（缓冲多次 Write 调用，直到完整包）
	writeBuffer bytes.Buffer
	writeBufMu  sync.Mutex
	writeFlush  chan struct{} // 触发刷新缓冲区

	// 控制
	closed  bool
	closeMu sync.Mutex

	// 地址信息（用于实现 net.Conn 接口）
	localAddr  net.Addr
	remoteAddr net.Addr
}

// UpdateClientID 更新客户端 ID（握手后调用）
func (c *HTTPLongPollingConn) UpdateClientID(newClientID int64) {
	c.pushMu.Lock()
	defer c.pushMu.Unlock()
	c.pollMu.Lock()
	defer c.pollMu.Unlock()
	
	oldClientID := c.clientID
	c.clientID = newClientID
	utils.Infof("HTTP long polling: updated clientID from %d to %d", oldClientID, newClientID)
}

// NewHTTPLongPollingConn 创建 HTTP 长轮询连接
func NewHTTPLongPollingConn(ctx context.Context, baseURL string, clientID int64, token string) (*HTTPLongPollingConn, error) {
	// 确保 baseURL 以 / 结尾
	baseURL = strings.TrimSuffix(baseURL, "/")
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	conn := &HTTPLongPollingConn{
		ManagerBase: dispose.NewManager("HTTPLongPollingConn", ctx),
		baseURL:     baseURL,
		clientID:    clientID,
		token:       token,
		pushURL:     baseURL + "/tunnox/v1/push",
		pollURL:     baseURL + "/tunnox/v1/poll",
		pushClient: &http.Client{
			Timeout: httppollDefaultPushTimeout,
		},
		pollClient: &http.Client{
			Timeout: httppollDefaultPollTimeout + 5*time.Second, // 轮询超时 + 缓冲
		},
		base64DataChan: make(chan string, 100),
		writeFlush:     make(chan struct{}, 1),
		localAddr:   &httppollAddr{network: "httppoll", addr: "local"},
		remoteAddr:  &httppollAddr{network: "httppoll", addr: baseURL},
	}

	// 注册清理处理器
	conn.AddCleanHandler(conn.onClose)

	// 启动接收循环
	utils.Debugf("HTTP long polling: starting pollLoop goroutine, clientID=%d, pollURL=%s", conn.clientID, conn.pollURL)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				utils.Errorf("HTTP long polling: pollLoop panic: %v, stack: %s", r, string(debug.Stack()))
			}
		}()
		utils.Debugf("HTTP long polling: pollLoop goroutine started, about to call pollLoop(), clientID=%d", conn.clientID)
		conn.pollLoop()
		utils.Debugf("HTTP long polling: pollLoop goroutine finished, clientID=%d", conn.clientID)
	}()

	// 启动写入刷新循环（定期刷新缓冲区）
	go conn.writeFlushLoop()

	utils.Infof("HTTP long polling: connection established to %s", baseURL)
	return conn, nil
}

// onClose 资源清理
func (c *HTTPLongPollingConn) onClose() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	// 关闭通道
	close(c.base64DataChan)

	return nil
}

// Read 实现 io.Reader 接口（从字节流缓冲区读取数据）
// 按照 Base64 适配层设计：Base64 解码后的数据追加到 readBuffer，Read 从 readBuffer 按字节读取
func (c *HTTPLongPollingConn) Read(p []byte) (int, error) {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()

	if closed {
		return 0, io.EOF
	}

	c.readBufMu.Lock()
	// 先检查缓冲区是否有数据
	if len(c.readBuffer) > 0 {
		n := copy(p, c.readBuffer)
		c.readBuffer = c.readBuffer[n:]
		c.readBufMu.Unlock()
		utils.Debugf("HTTP long polling: Read %d bytes from buffer (remaining: %d)", n, len(c.readBuffer))
		return n, nil
	}
	c.readBufMu.Unlock()

	// readBuffer 为空，从 base64DataChan 接收 Base64 数据并解码
	select {
	case <-c.Ctx().Done():
		return 0, c.Ctx().Err()
	case base64Data, ok := <-c.base64DataChan:
		if !ok {
			return 0, io.EOF
		}
		
		// Base64 解码
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			utils.Errorf("HTTP long polling: failed to decode Base64 data (len=%d): %v", len(base64Data), err)
			// 打印前20个字符用于调试
			preview := base64Data
			if len(preview) > 20 {
				preview = preview[:20]
			}
			utils.Errorf("HTTP long polling: Base64 data preview: %s", preview)
			return 0, fmt.Errorf("failed to decode Base64 data: %w", err)
		}
		
		// 验证解码后的数据不是 Base64 字符串（防止循环编码）
		if len(data) > 0 {
			isBase64Char := func(b byte) bool {
				return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || 
				       (b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
			}
			base64Count := 0
			for i := 0; i < len(data) && i < 10; i++ {
				if isBase64Char(data[i]) {
					base64Count++
				}
			}
			if base64Count >= 8 {
				utils.Errorf("HTTP long polling: decoded data appears to be Base64 string (first %d bytes are Base64 chars), possible double encoding", base64Count)
				return 0, fmt.Errorf("decoded data appears to be Base64 string, possible double encoding")
			}
		}
		
		// 追加到 readBuffer
			c.readBufMu.Lock()
		c.readBuffer = append(c.readBuffer, data...)
		
		// 从 readBuffer 读取
		n := copy(p, c.readBuffer)
		c.readBuffer = c.readBuffer[n:]
			c.readBufMu.Unlock()
		
		// 验证读取的数据不是 Base64 字符串（防止 Base64 数据被错误返回）
		if n > 0 && len(p) > 0 {
			isBase64Char := func(b byte) bool {
				return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || 
				       (b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
			}
			base64Count := 0
			for i := 0; i < n && i < 10; i++ {
				if isBase64Char(p[i]) {
					base64Count++
				}
			}
			if base64Count >= 8 {
				previewLen := 20
				if n < previewLen {
					previewLen = n
				}
				utils.Errorf("HTTP long polling: Read returned Base64-like data (first %d bytes are Base64 chars), possible error", base64Count)
				utils.Errorf("HTTP long polling: Read data preview (first %d bytes): %q, hex: %x", previewLen, string(p[:previewLen]), p[:previewLen])
			}
		}
		
		utils.Debugf("HTTP long polling: Read %d bytes (decoded from Base64, remaining in buffer: %d), firstByte=0x%02x", 
			n, len(c.readBuffer), func() byte { if n > 0 { return p[0] }; return 0 }())
		return n, nil
	}
}

// Write 实现 io.Writer 接口（通过 POST 发送数据）
// 注意：StreamProcessor.WritePacket() 会多次调用 Write()（包类型、包体大小、包体）
// 我们需要缓冲这些数据，直到收到完整的包后再发送
func (c *HTTPLongPollingConn) Write(p []byte) (int, error) {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	// 验证写入的数据不是 Base64 字符串（防止 Base64 数据被错误写入）
	if len(p) > 0 {
		isBase64Char := func(b byte) bool {
			return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || 
			       (b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
		}
		base64Count := 0
		for i := 0; i < len(p) && i < 10; i++ {
			if isBase64Char(p[i]) {
				base64Count++
			}
		}
		if base64Count >= 8 {
			previewLen := 20
			if len(p) < previewLen {
				previewLen = len(p)
			}
			utils.Errorf("HTTP long polling: Write called with Base64-like data (first %d bytes are Base64 chars), possible error", base64Count)
			utils.Errorf("HTTP long polling: Write data preview (first %d bytes): %q, hex: %x", previewLen, string(p[:previewLen]), p[:previewLen])
		}
	}

	// 将数据写入缓冲区
	c.writeBufMu.Lock()
	n, err := c.writeBuffer.Write(p)
	bufLen := c.writeBuffer.Len()
	c.writeBufMu.Unlock()

	firstByte := byte(0)
	if len(p) > 0 {
		firstByte = p[0]
	}
	
	// 如果是心跳包类型（0x43 = 0x03 | 0x40），添加更详细的日志
	if firstByte == 0x43 && len(p) == 1 {
		utils.Debugf("HTTP long polling: Write called with heartbeat packet type (0x43), len=%d, bufferLen=%d", len(p), bufLen)
		// 打印调用栈（仅前 5 层）
		utils.Debugf("HTTP long polling: Write call stack (first 5 frames):")
		for i := 1; i <= 5; i++ {
			pc, file, line, ok := runtime.Caller(i)
			if ok {
				fn := runtime.FuncForPC(pc)
				if fn != nil {
					// 只显示文件名和函数名，不显示完整路径
					fileName := file
					if idx := strings.LastIndex(file, "/"); idx >= 0 {
						fileName = file[idx+1:]
					}
					funcName := fn.Name()
					if idx := strings.LastIndex(funcName, "."); idx >= 0 {
						funcName = funcName[idx+1:]
					}
					utils.Debugf("  [%d] %s:%d %s", i, fileName, line, funcName)
				}
			}
		}
	} else {
		utils.Debugf("HTTP long polling: Write called, len=%d, bufferLen=%d, firstByte=0x%02x", len(p), bufLen, firstByte)
	}

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
func (c *HTTPLongPollingConn) writeFlushLoop() {
	utils.Infof("HTTP long polling: writeFlushLoop started")
	ticker := time.NewTicker(50 * time.Millisecond) // 每50ms检查一次
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			return
		case <-ticker.C:
			// 定期检查缓冲区
		case <-c.writeFlush:
			// 收到刷新信号，立即检查
			utils.Infof("HTTP long polling: writeFlushLoop received flush signal")
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
				
				utils.Debugf("HTTP long polling: writeFlushLoop sending heartbeat packet (0x%02x)", data[0])
				if err := c.sendData(data); err != nil {
					utils.Errorf("HTTP long polling: failed to send heartbeat packet: %v", err)
				}
				continue
			}
		}
		
		if bufLen >= 5 {
			// 至少有一个包类型（1字节）+ 包体大小（4字节）
			bufData := c.writeBuffer.Bytes()
			
			// 解析包体大小（大端序，从第2到第5字节，即索引1-4）
			// 注意：必须确保有足够的字节才能解析
			if len(bufData) < 5 {
				c.writeBufMu.Unlock()
				continue
			}
			
			// 调试：打印前5字节的十六进制值
			utils.Debugf("HTTP long polling: writeFlushLoop buffer first 5 bytes: %02x %02x %02x %02x %02x", 
				bufData[0], bufData[1], bufData[2], bufData[3], bufData[4])
			
			// 检查包类型是否有效（应该是 0x00-0xFF 范围内的值，但通常不会超过 0x3F + 标志位）
			packetType := bufData[0]
			
			// 检查是否是有效的包类型（排除明显无效的值）
			// 包类型的基础值应该在 0x00-0x3F 范围内，加上标志位（Compressed=0x40, Encrypted=0x80）
			// 所以有效范围是 0x00-0xFF，但排除一些明显无效的值
			basePacketType := packetType & 0x3F
			if basePacketType > 0x3F {
				// 基础包类型无效
				utils.Errorf("HTTP long polling: invalid base packet type 0x%02x, resetting buffer", basePacketType)
				c.writeBuffer.Reset()
				c.writeBufMu.Unlock()
				continue
			}
			
			bodySize := binary.BigEndian.Uint32(bufData[1:5])
			
			// 计算完整包大小：1字节类型 + 4字节大小 + bodySize
			packetSize := 5 + int(bodySize)
			
			// 验证包体大小是否合理（防止解析错误导致无限等待）
			// 正常的数据包体大小应该在 0-10MB 范围内
			if bodySize > 10*1024*1024 { // 10MB 上限
				utils.Errorf("HTTP long polling: invalid bodySize=%d (too large), packetType=0x%02x, first 5 bytes: %02x %02x %02x %02x %02x, resetting buffer", 
					bodySize, packetType, bufData[0], bufData[1], bufData[2], bufData[3], bufData[4])
				c.writeBuffer.Reset()
				c.writeBufMu.Unlock()
				continue
			}
			
			// 额外检查：如果前5字节都是相同的值（如 43 43 43 43 43），可能是数据损坏
			if bufData[0] == bufData[1] && bufData[1] == bufData[2] && bufData[2] == bufData[3] && bufData[3] == bufData[4] {
				utils.Errorf("HTTP long polling: suspicious data pattern (all bytes same: 0x%02x), resetting buffer", bufData[0])
				c.writeBuffer.Reset()
				c.writeBufMu.Unlock()
				continue
			}
			
			// 检查是否是 Base64 字符（A-Z, a-z, 0-9, +, /, =）
			// 如果前5字节都是 Base64 字符，可能是 Base64 字符串的字节被错误写入
			isBase64Char := func(b byte) bool {
				return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || 
				       (b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
			}
			if isBase64Char(bufData[0]) && isBase64Char(bufData[1]) && 
			   isBase64Char(bufData[2]) && isBase64Char(bufData[3]) && isBase64Char(bufData[4]) {
				// 检查是否连续多个字节都是 Base64 字符（可能是 Base64 字符串）
				base64Count := 0
				for i := 0; i < len(bufData) && i < 20; i++ {
					if isBase64Char(bufData[i]) {
						base64Count++
					} else {
						break
					}
				}
				if base64Count >= 10 {
					utils.Errorf("HTTP long polling: detected Base64 string in writeBuffer (first %d bytes are Base64 chars), resetting buffer", base64Count)
					c.writeBuffer.Reset()
					c.writeBufMu.Unlock()
					continue
				}
			}
			
			utils.Debugf("HTTP long polling: writeFlushLoop checking buffer, bufLen=%d, bodySize=%d, packetSize=%d", bufLen, bodySize, packetSize)
			
			if bufLen >= packetSize {
				// 有完整包，提取并发送
				data := make([]byte, packetSize)
				copy(data, bufData[:packetSize])
				c.writeBuffer.Next(packetSize)
				c.writeBufMu.Unlock()

				utils.Infof("HTTP long polling: writeFlushLoop sending complete packet, size=%d", packetSize)
				// 发送数据
				if err := c.sendData(data); err != nil {
					utils.Errorf("HTTP long polling: failed to send buffered data: %v", err)
				}
				continue
			}
		}
		c.writeBufMu.Unlock()
	}
}

// sendData 发送数据到服务器
func (c *HTTPLongPollingConn) sendData(data []byte) error {
	// 序列号管理
	c.pushMu.Lock()
	seq := c.pushSeq
	c.pushSeq++
	c.pushMu.Unlock()

	// Base64 编码
	encoded := base64.StdEncoding.EncodeToString(data)

	// 构造请求
	reqBody := map[string]interface{}{
		"data":      encoded,
		"seq":       seq,
		"timestamp": time.Now().Unix(),
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// 发送 POST 请求
	req, err := http.NewRequestWithContext(c.Ctx(), "POST", c.pushURL, bytes.NewReader(reqJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-Client-ID", strconv.FormatInt(c.clientID, 10))
	req.Header.Set("X-Request-ID", generateRequestID())
	
	utils.Infof("HTTP long polling: sending push request, clientID=%d, dataLen=%d, url=%s", c.clientID, len(data), c.pushURL)

	var resp *http.Response
	var retryCount int
	for retryCount < httppollMaxRetries {
		resp, err = c.pushClient.Do(req)
		if err == nil {
			break
		}

		retryCount++
		if retryCount < httppollMaxRetries {
			time.Sleep(httppollRetryInterval * time.Duration(retryCount))
			// 重新创建请求（Body 已被读取）
			req, _ = http.NewRequestWithContext(c.Ctx(), "POST", c.pushURL, bytes.NewReader(reqJSON))
			req.Header.Set("Content-Type", "application/json")
			if c.token != "" {
				req.Header.Set("Authorization", "Bearer "+c.token)
			}
			req.Header.Set("X-Client-ID", strconv.FormatInt(c.clientID, 10))
			req.Header.Set("X-Request-ID", generateRequestID())
		}
	}

	if err != nil {
		utils.Errorf("HTTP long polling: push request failed after %d retries: %v", retryCount, err)
		return fmt.Errorf("push request failed after %d retries: %w", retryCount, err)
	}
	defer resp.Body.Close()

	// 检查响应
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		utils.Errorf("HTTP long polling: push request failed: status %d, body: %s", resp.StatusCode, string(body))
		return fmt.Errorf("push request failed: status %d, body: %s", resp.StatusCode, string(body))
	}
	
	utils.Infof("HTTP long polling: push request succeeded, status=%d, seq=%d", resp.StatusCode, seq)

	// 解析 ACK（可选，用于确认）
	var ackResp struct {
		Success bool   `json:"success"`
		Ack     uint64 `json:"ack"`
	}
	json.NewDecoder(resp.Body).Decode(&ackResp)

	return nil
}

// pollLoop 长轮询循环
func (c *HTTPLongPollingConn) pollLoop() {
	utils.Debugf("HTTP long polling: pollLoop started, clientID=%d, pollURL=%s", c.clientID, c.pollURL)
	defer utils.Debugf("HTTP long polling: pollLoop exiting, clientID=%d", c.clientID)
	
	// 检查 context 是否已取消
	if c.Ctx().Err() != nil {
		utils.Debugf("HTTP long polling: pollLoop context already cancelled: %v", c.Ctx().Err())
		return
	}
	
	for {
		select {
		case <-c.Ctx().Done():
			utils.Debugf("HTTP long polling: pollLoop exiting due to context cancellation: %v", c.Ctx().Err())
			return
		default:
		}

		// 构造 GET 请求
		u, err := url.Parse(c.pollURL)
		if err != nil {
			utils.Errorf("HTTP long polling: failed to parse poll URL: %v", err)
			time.Sleep(httppollRetryInterval)
			continue
		}

		q := u.Query()
		q.Set("timeout", strconv.Itoa(int(httppollDefaultPollTimeout.Seconds())))
		q.Set("since", strconv.FormatUint(c.pollSeq, 10))
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(c.Ctx(), "GET", u.String(), nil)
		if err != nil {
			utils.Errorf("HTTP long polling: failed to create poll request: %v", err)
			time.Sleep(httppollRetryInterval)
			continue
		}

		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		req.Header.Set("X-Client-ID", strconv.FormatInt(c.clientID, 10))
		req.Header.Set("X-Request-ID", generateRequestID())

		utils.Debugf("HTTP long polling: sending poll request, clientID=%d, url=%s", c.clientID, u.String())
		// 发送长轮询请求
		resp, err := c.pollClient.Do(req)
		if err != nil {
			// 如果是 context 取消，直接退出
			if err == context.Canceled || c.Ctx().Err() != nil {
				utils.Debugf("HTTP long polling: poll request cancelled, exiting")
				return
			}
			utils.Debugf("HTTP long polling: poll request failed: %v, retrying...", err)
			time.Sleep(httppollRetryInterval)
			continue
		}
		
		utils.Debugf("HTTP long polling: poll request succeeded, status=%d", resp.StatusCode)

		// 解析响应
		var pollResp struct {
			Success bool   `json:"success"`
			Data    string `json:"data"`
			Seq     uint64 `json:"seq"`
			Timeout bool   `json:"timeout"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&pollResp); err != nil {
			resp.Body.Close()
			utils.Errorf("HTTP long polling: failed to decode poll response: %v", err)
			time.Sleep(httppollRetryInterval)
			continue
		}
		resp.Body.Close()

		// 处理数据：按照 Base64 适配层设计，Base64 数据直接发送到 base64DataChan
		// Read() 方法会从 base64DataChan 接收并解码，追加到 readBuffer
		if pollResp.Data != "" {
			utils.Debugf("HTTP long polling: received Base64 data in poll response, len=%d, seq=%d", len(pollResp.Data), pollResp.Seq)
			
			// 验证 Base64 数据格式（前几个字符应该是有效的 Base64 字符）
			previewLen := 20
			if len(pollResp.Data) < previewLen {
				previewLen = len(pollResp.Data)
			}
			utils.Debugf("HTTP long polling: Base64 data preview (first %d chars): %s", previewLen, pollResp.Data[:previewLen])

			// 更新序列号
			c.pollMu.Lock()
			c.pollSeq = pollResp.Seq + 1
			c.pollMu.Unlock()

			// 发送 Base64 数据到 base64DataChan（Read() 会解码并追加到 readBuffer）
			select {
			case <-c.Ctx().Done():
				return
			case c.base64DataChan <- pollResp.Data:
				utils.Debugf("HTTP long polling: sent Base64 data to base64DataChan, len=%d", len(pollResp.Data))
			default:
				utils.Warnf("HTTP long polling: base64DataChan full, dropping data")
			}
		} else if pollResp.Timeout {
			utils.Debugf("HTTP long polling: poll request timeout (seq=%d), retrying...", pollResp.Seq)
		}

		// 继续循环，立即发起下一个请求（无论是否超时）
		continue
	}
}

// Close 实现 io.Closer 接口
func (c *HTTPLongPollingConn) Close() error {
	return c.Dispose.CloseWithError()
}

// LocalAddr 实现 net.Conn 接口
func (c *HTTPLongPollingConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr 实现 net.Conn 接口
func (c *HTTPLongPollingConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline 实现 net.Conn 接口
func (c *HTTPLongPollingConn) SetDeadline(t time.Time) error {
	// HTTP 长轮询不支持设置 deadline
	return nil
}

// SetReadDeadline 实现 net.Conn 接口
func (c *HTTPLongPollingConn) SetReadDeadline(t time.Time) error {
	// HTTP 长轮询不支持设置 read deadline
	return nil
}

// SetWriteDeadline 实现 net.Conn 接口
func (c *HTTPLongPollingConn) SetWriteDeadline(t time.Time) error {
	// HTTP 长轮询不支持设置 write deadline
	return nil
}

// httppollAddr 实现 net.Addr 接口
type httppollAddr struct {
	network string
	addr    string
}

func (a *httppollAddr) Network() string {
	return a.network
}

func (a *httppollAddr) String() string {
	return a.addr
}

// generateRequestID 生成请求 ID
func generateRequestID() string {
	// 使用时间戳和随机数生成唯一请求ID
	nanos := time.Now().UnixNano()
	// 使用简单的哈希值而不是 binary.BigEndian.Uint64（避免索引越界）
	hash := uint64(0)
	for _, b := range []byte("request") {
		hash = hash*31 + uint64(b)
	}
	return fmt.Sprintf("%d-%d", nanos, hash)
}

// dialHTTPLongPolling 建立 HTTP 长轮询连接
func dialHTTPLongPolling(ctx context.Context, baseURL string, clientID int64, token string) (net.Conn, error) {
	return NewHTTPLongPollingConn(ctx, baseURL, clientID, token)
}

