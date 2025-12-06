package httppoll

import (
	"bytes"
	"context"
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
	defaultPollTimeout      = 30 * time.Second
	maxRetries              = 3
	retryInterval           = 1 * time.Second
	maxBufferSize           = 1024 * 1024      // 1MB
	responseCacheTTL        = 60 * time.Second // 响应缓存过期时间
	responseCacheMaxSize    = 1000             // 响应缓存最大容量
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

	// 分片重组器（用于处理服务器端发送的分片数据）
	fragmentReassembler *FragmentReassembler

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
		pushURL:             pushURL,
		pollURL:             pollURL,
		clientID:            clientID,
		token:               token,
		instanceID:          instanceID,
		mappingID:           mappingID,
		tunnelType:          connType,
		dataBuffer:          bytes.NewBuffer(nil),
		packetQueue:         make(chan *packet.TransferPacket, 100),
		responseCache:       make(map[string]*cachedResponse),
		pollRequestChan:     make(chan string, 10),    // 缓冲 10 个请求
		fragmentReassembler: NewFragmentReassembler(), // 创建分片重组器
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
	} else {
		// 生成新的 RequestId
		requestID = uuid.New().String()
		sp.pendingPollRequestMu.Unlock()


		// 通知 pollLoop 发送 Poll 请求
		select {
		case sp.pollRequestChan <- requestID:
		case <-sp.Ctx().Done():
			return nil, 0, sp.Ctx().Err()
		default:
			// 通道满，直接返回（pollLoop 会继续处理）
		}
	}

	// 从缓存中查找响应（带超时）
	timeout := time.NewTimer(35 * time.Second) // 比 Poll 超时稍长
	defer timeout.Stop()

	// 优化：先立即检查一次缓存（可能响应已经到达）
	if pkt, exists := sp.getCachedResponse(requestID); exists {
		sp.removeCachedResponse(requestID)

		return pkt, 0, nil
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
			if pkt, exists := sp.getCachedResponse(requestID); exists {
				sp.removeCachedResponse(requestID)

				return pkt, 0, nil
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
	sp.closeMu.RUnlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	// 1. 生成 RequestId（用于匹配请求和响应）
	requestID := uuid.New().String()

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


	// 4. 发送请求（带重试）
	var resp *http.Response
	for retry := 0; retry < maxRetries; retry++ {
		// 使用 StreamProcessor 的 context 作为父 context，确保能接收退出信号
		reqCtx, reqCancel := context.WithTimeout(sp.Ctx(), 10*time.Second)
		reqWithCtx := req.WithContext(reqCtx)

		resp, err = sp.httpClient.Do(reqWithCtx)
		reqCancel() // 立即取消 context，释放资源

		if err == nil {
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
					// 安全地向 packetQueue 写入，使用 recover 捕获可能的 panic（channel 已关闭）
					func() {
						defer func() {
							if r := recover(); r != nil {
								utils.Warnf("HTTPStreamProcessor: WritePacket - panic when writing to packetQueue (likely closed), requestID=%s, connID=%s, error=%v", requestID, sp.connectionID, r)
							}
						}()
						select {
						case sp.packetQueue <- respPkt:
						default:
							// 队列满，丢弃
							utils.Warnf("HTTPStreamProcessor: WritePacket - packetQueue full, dropping response packet, requestID=%s, connID=%s", requestID, sp.connectionID)
						}
					}()
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
		_, readErr := io.ReadAll(resp.Body)
		if readErr != nil && readErr != io.EOF {
			utils.Warnf("HTTPStreamProcessor: WritePacket - failed to read response body: %v, requestID=%s, connID=%s", readErr, requestID, sp.connectionID)
		}
	}

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

	// 获取序列号（客户端也需要序列号，但主要用于日志追踪）
	// 注意：客户端发送数据时，序列号由服务器端分配，这里使用0作为占位符
	// 实际上，客户端发送的分片会在服务器端重新分配序列号
	sequenceNumber := int64(0)

	// 对大数据包进行分片处理（类似服务器端的 WriteExact）
	fragments, err := SplitDataIntoFragments(data, sequenceNumber)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor[%s]: WriteExact - failed to split data into fragments: %v, connID=%s", sp.connectionID, err, sp.connectionID)
		return fmt.Errorf("failed to split data into fragments: %w", err)
	}

	// 发送每个分片
	for i, fragment := range fragments {
		// 序列化分片响应为 JSON
		fragmentJSON, err := MarshalFragmentResponse(fragment)
		if err != nil {
			return fmt.Errorf("failed to marshal fragment: %w", err)
		}

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

		// 将分片 JSON 作为 data 字段发送（服务器端会识别并处理分片）
		reqBody := map[string]interface{}{
			"data":      string(fragmentJSON), // 发送 JSON 字符串，而不是 Base64 编码的原始数据
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
			utils.Errorf("HTTPStreamProcessor[%s]: WriteExact - push request failed for fragment %d/%d: %v, groupID=%s, requestID=%s, connID=%s",
				sp.connectionID, i+1, len(fragments), err, fragment.FragmentGroupID, requestID, sp.connectionID)
			return fmt.Errorf("push data request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			utils.Errorf("HTTPStreamProcessor[%s]: WriteExact - push request failed for fragment %d/%d: status %d, body: %s, groupID=%s, requestID=%s, connID=%s",
				sp.connectionID, i+1, len(fragments), resp.StatusCode, string(body), fragment.FragmentGroupID, requestID, sp.connectionID)
			return fmt.Errorf("push data request failed: status %d, body: %s", resp.StatusCode, string(body))
		}
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
			utils.Errorf("HTTPStreamProcessor[%s]: ReadExact - ReadPacket failed: %v, connID=%s", sp.connectionID, err, sp.connectionID)
			return nil, err
		}
		sp.dataBufMu.Lock()
	}

	data := make([]byte, length)
	n, err := sp.dataBuffer.Read(data)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor[%s]: ReadExact - failed to read from buffer: %v, connID=%s", sp.connectionID, err, sp.connectionID)
		return nil, err
	}
	if n < length {
		utils.Errorf("HTTPStreamProcessor[%s]: ReadExact - read %d bytes, expected %d, connID=%s", sp.connectionID, n, length, sp.connectionID)
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
