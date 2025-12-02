package httppoll

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"github.com/google/uuid"
)

const (
	defaultPollTimeout = 30 * time.Second
	maxRetries         = 3
	retryInterval      = 1 * time.Second
	maxBufferSize      = 1024 * 1024      // 1MB
	responseCacheTTL   = 60 * time.Second // 响应缓存过期时间
)

// StreamProcessor HTTP 长轮询流处理器
// 实现 stream.PackageStreamer 接口，内部使用 PacketConverter 进行转换
type StreamProcessor struct {
	*dispose.ManagerBase

	converter  *PacketConverter
	httpClient *http.Client
	pushURL    string
	pollURL    string

	// 连接信息
	connectionID string
	clientID     int64
	mappingID    string
	tunnelType   string

	// 数据流缓冲
	dataBuffer  *bytes.Buffer
	dataBufMu   sync.Mutex
	packetQueue chan *packet.TransferPacket

	// 控制
	closed  bool
	closeMu sync.RWMutex

	// 用于客户端：token 和 instanceID
	token      string
	instanceID string

	// Poll 响应缓存（RequestID -> 响应）
	responseCache   map[string]*cachedResponse
	responseCacheMu sync.RWMutex
	pollRequestChan chan string // 用于通知 pollLoop 发送新的 Poll 请求

	// 待使用的 Poll 请求 ID（由 TriggerImmediatePoll 设置，供 ReadPacket 使用）
	pendingPollRequestID string
	pendingPollRequestMu sync.Mutex
}

// cachedResponse 缓存的响应
type cachedResponse struct {
	pkt       *packet.TransferPacket
	expiresAt time.Time
}

// NewStreamProcessor 创建 HTTP 长轮询流处理器
func NewStreamProcessor(ctx context.Context, baseURL, pushURL, pollURL string, clientID int64, token, instanceID, mappingID string) *StreamProcessor {
	connType := "control"
	if mappingID != "" {
		connType = "data"
	}

	sp := &StreamProcessor{
		ManagerBase: dispose.NewManager("HTTPStreamProcessor", ctx),
		converter:   NewPacketConverter(),
		httpClient: &http.Client{
			Timeout: defaultPollTimeout + 5*time.Second,
		},
		pushURL:         pushURL,
		pollURL:         pollURL,
		clientID:        clientID,
		token:           token,
		instanceID:      instanceID,
		mappingID:       mappingID,
		tunnelType:      connType,
		dataBuffer:      bytes.NewBuffer(nil),
		packetQueue:     make(chan *packet.TransferPacket, 100),
		responseCache:   make(map[string]*cachedResponse),
		pollRequestChan: make(chan string, 10), // 缓冲 10 个请求
	}

	sp.converter.SetConnectionInfo("", clientID, mappingID, connType)

	sp.AddCleanHandler(sp.onClose)

	// 启动 Poll 循环
	go sp.pollLoop()

	return sp
}

// onClose 资源清理
func (sp *StreamProcessor) onClose() error {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()

	if sp.closed {
		return nil
	}
	sp.closed = true

	close(sp.packetQueue)
	close(sp.pollRequestChan)
	sp.dataBuffer.Reset()

	// 清理响应缓存
	sp.responseCacheMu.Lock()
	sp.responseCache = make(map[string]*cachedResponse)
	sp.responseCacheMu.Unlock()

	return nil
}

// pollLoop 持续发送 Poll 请求并缓存响应
func (sp *StreamProcessor) pollLoop() {
	for {
		select {
		case <-sp.Ctx().Done():
			return
		case requestID, ok := <-sp.pollRequestChan:
			if !ok {
				return
			}
			// 发送 Poll 请求
			sp.sendPollRequest(requestID)
		}
	}
}

// TriggerImmediatePoll 立即触发一个 Poll 请求（用于发送命令后快速获取响应）
// 返回的 RequestID 应该被 ReadPacket 使用
func (sp *StreamProcessor) TriggerImmediatePoll() string {
	requestID := uuid.New().String()
	// 设置待使用的 RequestID
	sp.pendingPollRequestMu.Lock()
	sp.pendingPollRequestID = requestID
	sp.pendingPollRequestMu.Unlock()

	select {
	case sp.pollRequestChan <- requestID:
		utils.Infof("[CMD_TRACE] [CLIENT] [TRIGGER_POLL_IMMEDIATE] RequestID=%s, ConnID=%s, Time=%s",
			requestID, sp.connectionID, time.Now().Format("15:04:05.000"))
		return requestID
	case <-sp.Ctx().Done():
		sp.pendingPollRequestMu.Lock()
		sp.pendingPollRequestID = ""
		sp.pendingPollRequestMu.Unlock()
		return ""
	default:
		// 通道满，清除待使用的 RequestID
		sp.pendingPollRequestMu.Lock()
		sp.pendingPollRequestID = ""
		sp.pendingPollRequestMu.Unlock()
		utils.Warnf("[CMD_TRACE] [CLIENT] [TRIGGER_POLL_IMMEDIATE_WARN] RequestID=%s, Reason=pollRequestChan_full, Time=%s",
			requestID, time.Now().Format("15:04:05.000"))
		return ""
	}
}

// sendPollRequest 发送单个 Poll 请求并缓存响应
func (sp *StreamProcessor) sendPollRequest(requestID string) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		utils.Debugf("HTTPStreamProcessor: sendPollRequest - connection closed, requestID=%s", requestID)
		return
	}

	pollStartTime := time.Now()
	utils.Infof("[CMD_TRACE] [CLIENT] [POLL_START] RequestID=%s, ConnID=%s, Time=%s",
		requestID, connID, pollStartTime.Format("15:04:05.000"))

	// 构建 Poll 请求的 TunnelPackage
	pollPkg := &TunnelPackage{
		ConnectionID: connID,
		RequestID:    requestID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   sp.tunnelType,
	}
	encoded, err := EncodeTunnelPackage(pollPkg)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to encode poll package: %v, requestID=%s", err, requestID)
		return
	}

	// 发送 Poll 请求
	req, err := http.NewRequestWithContext(sp.Ctx(), "GET", sp.pollURL+"?timeout=30", nil)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to create poll request: %v, requestID=%s", err, requestID)
		return
	}

	req.Header.Set("X-Tunnel-Package", encoded)
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	utils.Infof("HTTPStreamProcessor: sendPollRequest - Poll request sent, requestID=%s, encodedLen=%d", requestID, len(encoded))
	resp, err := sp.httpClient.Do(req)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - Poll request failed: %v, requestID=%s", err, requestID)
		return
	}
	defer resp.Body.Close()

	utils.Infof("HTTPStreamProcessor: sendPollRequest - Poll response received, status=%d, requestID=%s", resp.StatusCode, requestID)

	// 检查是否有控制包（X-Tunnel-Package 中）
	xTunnelPackage := resp.Header.Get("X-Tunnel-Package")
	utils.Infof("HTTPStreamProcessor: sendPollRequest - checking X-Tunnel-Package header, present=%v, len=%d, requestID=%s",
		xTunnelPackage != "", len(xTunnelPackage), requestID)
	if xTunnelPackage != "" {
		// 解码 TunnelPackage 以检查 RequestId
		pkg, err := DecodeTunnelPackage(xTunnelPackage)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to decode tunnel package: %v, requestID=%s", err, requestID)
			return
		}

		utils.Infof("HTTPStreamProcessor: sendPollRequest - decoded tunnel package, requestID in response=%s, expected=%s",
			pkg.RequestID, requestID)

		// 检查 RequestId 是否匹配
		if pkg.RequestID != requestID {
			utils.Warnf("HTTPStreamProcessor: sendPollRequest - RequestId mismatch, expected=%s, got=%s, ignoring response",
				requestID, pkg.RequestID)
			return
		}

		// 转换为 TransferPacket
		pkt, err := sp.converter.ReadPacket(resp)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to read packet: %v, requestID=%s", err, requestID)
			return
		}

		// 更新连接信息
		if pkg.ConnectionID != "" {
			sp.SetConnectionID(pkg.ConnectionID)
		}

		// 缓存响应
		sp.responseCacheMu.Lock()
		sp.responseCache[requestID] = &cachedResponse{
			pkt:       pkt,
			expiresAt: time.Now().Add(responseCacheTTL),
		}
		sp.responseCacheMu.Unlock()

		utils.Infof("HTTPStreamProcessor: sendPollRequest - cached response, requestID=%s, type=0x%02x",
			requestID, byte(pkt.PacketType)&0x3F)
	}

	// 处理数据流（如果有）
	var pollResp struct {
		Success bool   `json:"success"`
		Data    string `json:"data,omitempty"`
		Timeout bool   `json:"timeout,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pollResp); err == nil && pollResp.Data != "" {
		// Base64 解码
		data, err := base64.StdEncoding.DecodeString(pollResp.Data)
		if err == nil {
			// 将数据放入缓冲，供 ReadExact 使用
			sp.dataBufMu.Lock()
			if sp.dataBuffer.Len()+len(data) <= maxBufferSize {
				sp.dataBuffer.Write(data)
			}
			sp.dataBufMu.Unlock()
		}
	}
}

// cleanupExpiredResponses 清理过期的响应缓存
func (sp *StreamProcessor) cleanupExpiredResponses() {
	now := time.Now()
	sp.responseCacheMu.Lock()
	defer sp.responseCacheMu.Unlock()

	for requestID, cached := range sp.responseCache {
		if now.After(cached.expiresAt) {
			delete(sp.responseCache, requestID)
		}
	}
}

// SetConnectionID 设置连接 ID（服务端分配）
func (sp *StreamProcessor) SetConnectionID(connID string) {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()
	sp.connectionID = connID
	sp.converter.SetConnectionInfo(connID, sp.clientID, sp.mappingID, sp.tunnelType)
}

// UpdateClientID 更新客户端 ID
func (sp *StreamProcessor) UpdateClientID(newClientID int64) {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()
	sp.clientID = newClientID
	sp.converter.SetConnectionInfo(sp.connectionID, newClientID, sp.mappingID, sp.tunnelType)
}

// ReadPacket 从响应缓存中读取包
func (sp *StreamProcessor) ReadPacket() (*packet.TransferPacket, int, error) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		return nil, 0, io.EOF
	}

	// 如果 connectionID 为空，说明还没有从服务端获取到 ConnectionID
	// 在握手阶段，客户端会先发送 Push（握手请求），然后立即发送 Poll（等待握手响应）
	// 此时 connectionID 可能还是空的，需要等待服务端在握手响应中分配
	if connID == "" {
		// 先检查 packetQueue 中是否有响应包（Push 请求的响应可能已经在队列中）
		select {
		case pkt := <-sp.packetQueue:
			return pkt, 0, nil
		default:
		}
	}

	// 检查是否有待使用的 Poll 请求 ID（由 TriggerImmediatePoll 设置）
	sp.pendingPollRequestMu.Lock()
	requestID := sp.pendingPollRequestID
	if requestID != "" {
		// 使用已触发的 Poll 请求 ID，并清除
		sp.pendingPollRequestID = ""
		sp.pendingPollRequestMu.Unlock()
		utils.Infof("[CMD_TRACE] [CLIENT] [READ_START] RequestID=%s (from TriggerImmediatePoll), ConnID=%s, Time=%s",
			requestID, connID, time.Now().Format("15:04:05.000"))
	} else {
		// 生成新的 RequestId
		requestID = uuid.New().String()
		sp.pendingPollRequestMu.Unlock()

		readStartTime := time.Now()
		utils.Infof("[CMD_TRACE] [CLIENT] [READ_START] RequestID=%s (new), ConnID=%s, Time=%s",
			requestID, connID, readStartTime.Format("15:04:05.000"))

		// 通知 pollLoop 发送 Poll 请求
		select {
		case sp.pollRequestChan <- requestID:
			utils.Infof("[CMD_TRACE] [CLIENT] [POLL_TRIGGER] RequestID=%s, ConnID=%s, Time=%s",
				requestID, connID, time.Now().Format("15:04:05.000"))
		case <-sp.Ctx().Done():
			return nil, 0, sp.Ctx().Err()
		default:
			// 通道满，直接返回（pollLoop 会继续处理）
			utils.Warnf("[CMD_TRACE] [CLIENT] [POLL_TRIGGER_FAILED] RequestID=%s, ConnID=%s, Reason=channel_full, Time=%s",
				requestID, connID, time.Now().Format("15:04:05.000"))
		}
	}

	readStartTime := time.Now()

	// 从缓存中查找响应（带超时）
	timeout := time.NewTimer(35 * time.Second) // 比 Poll 超时稍长
	defer timeout.Stop()

	// 优化：先立即检查一次缓存（可能响应已经到达）
	sp.responseCacheMu.RLock()
	cached, exists := sp.responseCache[requestID]
	sp.responseCacheMu.RUnlock()
	if exists {
		// 找到响应，从缓存中删除
		sp.responseCacheMu.Lock()
		delete(sp.responseCache, requestID)
		sp.responseCacheMu.Unlock()

		baseType := byte(cached.pkt.PacketType) & 0x3F
		var commandID string
		if cached.pkt.CommandPacket != nil {
			commandID = cached.pkt.CommandPacket.CommandId
		}
		utils.Infof("[CMD_TRACE] [CLIENT] [READ_IMMEDIATE] RequestID=%s, CommandID=%s, PacketType=0x%02x, Time=%s",
			requestID, commandID, baseType, time.Now().Format("15:04:05.000"))
		return cached.pkt, 0, nil
	}

	// 定期清理过期响应
	sp.cleanupExpiredResponses()

	// 使用更短的检查间隔（10ms），提高响应速度
	ticker := time.NewTicker(10 * time.Millisecond) // 每 10ms 检查一次
	defer ticker.Stop()

	// 用于定期清理过期响应（每 1 秒清理一次）
	lastCleanup := time.Now()

	for {
		select {
		case <-sp.Ctx().Done():
			return nil, 0, sp.Ctx().Err()
		case <-timeout.C:
			utils.Debugf("HTTPStreamProcessor: ReadPacket - timeout waiting for response, requestID=%s", requestID)
			return nil, 0, fmt.Errorf("timeout waiting for response")
		case <-ticker.C:
			// 检查缓存
			sp.responseCacheMu.RLock()
			cached, exists = sp.responseCache[requestID]
			sp.responseCacheMu.RUnlock()

			if exists {
				// 找到响应，从缓存中删除
				sp.responseCacheMu.Lock()
				delete(sp.responseCache, requestID)
				sp.responseCacheMu.Unlock()

				readDuration := time.Since(readStartTime)
				baseType := byte(cached.pkt.PacketType) & 0x3F
				var commandID string
				if cached.pkt.CommandPacket != nil {
					commandID = cached.pkt.CommandPacket.CommandId
				}
				utils.Infof("[CMD_TRACE] [CLIENT] [READ_COMPLETE] RequestID=%s, CommandID=%s, PacketType=0x%02x, ReadDuration=%v, Time=%s",
					requestID, commandID, baseType, readDuration, time.Now().Format("15:04:05.000"))
				return cached.pkt, 0, nil
			}

			// 定期清理过期响应（每 1 秒清理一次，避免频繁清理）
			if time.Since(lastCleanup) >= time.Second {
				sp.cleanupExpiredResponses()
				lastCleanup = time.Now()
			}
		}
	}
}

// WritePacket 通过 HTTP Push 发送包
func (sp *StreamProcessor) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	// 1. 生成 RequestId（用于匹配请求和响应）
	requestID := uuid.New().String()

	// [CMD_TRACE] 记录 Push 请求开始
	writeStartTime := time.Now()
	baseType := byte(pkt.PacketType) & 0x3F
	var commandID string
	if pkt.CommandPacket != nil {
		commandID = pkt.CommandPacket.CommandId
	}
	utils.Infof("[CMD_TRACE] [CLIENT] [PUSH_START] RequestID=%s, CommandID=%s, PacketType=0x%02x, ConnID=%s, Time=%s",
		requestID, commandID, baseType, connID, writeStartTime.Format("15:04:05.000"))

	// 2. 更新转换器的连接状态
	sp.converter.SetConnectionInfo(sp.connectionID, sp.clientID, sp.mappingID, sp.tunnelType)

	// 3. 转换为 HTTP Request（携带 RequestId）
	req, err := sp.converter.WritePacket(pkt, requestID)
	if err != nil {
		return 0, fmt.Errorf("failed to convert packet: %w", err)
	}

	// 3. 设置请求 URL 和认证
	reqURL, err := url.Parse(sp.pushURL)
	if err != nil {
		return 0, fmt.Errorf("invalid push URL: %w", err)
	}
	req.URL = reqURL

	// 检查 context 是否已取消
	select {
	case <-sp.Ctx().Done():
		utils.Errorf("HTTPStreamProcessor: WritePacket - context canceled before sending Push request, requestID=%s, connID=%s, err=%v", requestID, sp.connectionID, sp.Ctx().Err())
		return 0, fmt.Errorf("push request failed: context canceled: %w", sp.Ctx().Err())
	default:
	}

	utils.Infof("HTTPStreamProcessor: WritePacket - sending Push request, requestID=%s, connID=%s, type=0x%02x", requestID, sp.connectionID, byte(pkt.PacketType)&0x3F)

	// 4. 发送请求（带重试）
	var resp *http.Response
	for retry := 0; retry < maxRetries; retry++ {
		// 使用独立的 context，避免被主 context 取消影响
		reqCtx, reqCancel := context.WithTimeout(context.Background(), 10*time.Second)
		reqWithCtx := req.WithContext(reqCtx)

		resp, err = sp.httpClient.Do(reqWithCtx)
		reqCancel() // 立即取消 context，释放资源

		if err == nil {
			utils.Infof("HTTPStreamProcessor: WritePacket - Push request sent successfully, requestID=%s, connID=%s", requestID, sp.connectionID)
			break
		}
		utils.Warnf("HTTPStreamProcessor: WritePacket - Push request failed (retry %d/%d), requestID=%s, connID=%s, err=%v", retry+1, maxRetries, requestID, sp.connectionID, err)
		if retry < maxRetries-1 {
			time.Sleep(retryInterval * time.Duration(retry+1))
			// 重新创建请求（使用相同的 RequestId）
			req, _ = sp.converter.WritePacket(pkt, requestID)
			reqURL, _ := url.Parse(sp.pushURL)
			req.URL = reqURL
			if sp.token != "" {
				req.Header.Set("Authorization", "Bearer "+sp.token)
			}
		}
	}

	if err != nil {
		return 0, fmt.Errorf("push request failed: %w", err)
	}
	defer resp.Body.Close()

	// 5. 处理响应（如果有控制包响应，在 X-Tunnel-Package 中）
	if resp.Header.Get("X-Tunnel-Package") != "" {
		// 解码 TunnelPackage 以检查 RequestId
		pkg, err := DecodeTunnelPackage(resp.Header.Get("X-Tunnel-Package"))
		if err == nil {
			// 检查 RequestId 是否匹配
			if pkg.RequestID == requestID {
				// RequestId 匹配，处理响应
				respPkt, _ := sp.converter.ReadPacket(resp)
				// 将响应包放入队列，供后续读取
				if respPkt != nil {
					select {
					case sp.packetQueue <- respPkt:
					default:
						// 队列满，丢弃
					}
				}
				// 更新连接信息
				if pkg.ConnectionID != "" {
					sp.SetConnectionID(pkg.ConnectionID)
				}
			} else {
				utils.Debugf("HTTPStreamProcessor: WritePacket - RequestId mismatch, expected=%s, got=%s, ignoring response",
					requestID, pkg.RequestID)
			}
		}
	}

	// 6. 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("push request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// 读取并丢弃响应 body（确保连接正确关闭）
	// 注意：即使 body 为空，也要读取，否则连接可能不会正确关闭
	if resp.Body != nil {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil && readErr != io.EOF {
			utils.Warnf("HTTPStreamProcessor: WritePacket - failed to read response body: %v, requestID=%s, connID=%s", readErr, requestID, sp.connectionID)
		} else {
			utils.Infof("HTTPStreamProcessor: WritePacket - Push request completed successfully, requestID=%s, connID=%s, bodyLen=%d", requestID, sp.connectionID, len(body))
		}
	}

	// [CMD_TRACE] 记录 Push 请求完成
	writeDuration := time.Since(writeStartTime)
	utils.Infof("[CMD_TRACE] [CLIENT] [PUSH_COMPLETE] RequestID=%s, CommandID=%s, Duration=%v, Time=%s",
		requestID, commandID, writeDuration, time.Now().Format("15:04:05.000"))

	return 0, nil
}

// WriteExact 将数据流写入 HTTP Request Body
func (sp *StreamProcessor) WriteExact(data []byte) error {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return io.ErrClosedPipe
	}

	// Base64 编码数据
	encoded := base64.StdEncoding.EncodeToString(data)

	// 生成 RequestId（用于匹配请求和响应）
	requestID := uuid.New().String()

	// 构建 HTTP Request
	// 数据流传输时，X-Tunnel-Package 只包含连接标识（用于路由）
	dataPkg := &TunnelPackage{
		ConnectionID: sp.connectionID,
		RequestID:    requestID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   "data",
		// Type 为空，表示这是数据流传输
	}
	encodedPkg, err := EncodeTunnelPackage(dataPkg)
	if err != nil {
		return fmt.Errorf("failed to encode data package: %w", err)
	}

	reqBody := map[string]interface{}{
		"data":      encoded,
		"timestamp": time.Now().Unix(),
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(sp.Ctx(), "POST", sp.pushURL, bytes.NewReader(reqJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tunnel-Package", encodedPkg)
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	// 发送请求
	resp, err := sp.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("push data request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push data request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ReadExact 从数据流缓冲读取指定长度
func (sp *StreamProcessor) ReadExact(length int) ([]byte, error) {
	sp.dataBufMu.Lock()
	defer sp.dataBufMu.Unlock()

	// 从缓冲读取，如果不够则触发 Poll 请求获取更多数据
	for sp.dataBuffer.Len() < length {
		sp.dataBufMu.Unlock()
		// 触发 Poll 获取更多数据
		_, _, err := sp.ReadPacket()
		if err != nil {
			return nil, err
		}
		sp.dataBufMu.Lock()
	}

	data := make([]byte, length)
	n, err := sp.dataBuffer.Read(data)
	if err != nil {
		return nil, err
	}
	if n < length {
		return nil, io.ErrUnexpectedEOF
	}

	return data[:n], nil
}

// GetReader 获取底层 Reader（HTTP 无状态，返回 nil）
func (sp *StreamProcessor) GetReader() io.Reader {
	// HTTP 是无状态的，没有底层的 io.Reader
	// 返回 nil，上层代码应该使用 ReadPacket() 和 ReadExact()
	return nil
}

// GetWriter 获取底层 Writer（HTTP 无状态，返回 nil）
func (sp *StreamProcessor) GetWriter() io.Writer {
	// HTTP 是无状态的，没有底层的 io.Writer
	// 返回 nil，上层代码应该使用 WritePacket() 和 WriteExact()
	return nil
}

// Close 关闭连接
func (sp *StreamProcessor) Close() {
	sp.Dispose.CloseWithError()
}

// 确保 StreamProcessor 实现 stream.PackageStreamer 接口
var _ stream.PackageStreamer = (*StreamProcessor)(nil)
