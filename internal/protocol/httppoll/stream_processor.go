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
)

const (
	defaultPollTimeout = 30 * time.Second
	maxRetries         = 3
	retryInterval      = 1 * time.Second
	maxBufferSize      = 1024 * 1024 // 1MB
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
}

// NewStreamProcessor 创建 HTTP 长轮询流处理器
func NewStreamProcessor(ctx context.Context, baseURL, pushURL, pollURL string, clientID int64, token, instanceID, mappingID string) *StreamProcessor {
	connType := "control"
	if mappingID != "" {
		connType = "data"
	}

	sp := &StreamProcessor{
		ManagerBase: dispose.NewManager("HTTPStreamProcessor", ctx),
		converter:    NewPacketConverter(),
		httpClient: &http.Client{
			Timeout: defaultPollTimeout + 5*time.Second,
		},
		pushURL:     pushURL,
		pollURL:     pollURL,
		clientID:    clientID,
		token:       token,
		instanceID:  instanceID,
		mappingID:   mappingID,
		tunnelType:  connType,
		dataBuffer:  bytes.NewBuffer(nil),
		packetQueue: make(chan *packet.TransferPacket, 100),
	}

	sp.converter.SetConnectionInfo("", clientID, mappingID, connType)

	sp.AddCleanHandler(sp.onClose)

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
	sp.dataBuffer.Reset()

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

// ReadPacket 从 HTTP Poll 响应读取包
func (sp *StreamProcessor) ReadPacket() (*packet.TransferPacket, int, error) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		return nil, 0, io.EOF
	}

	// 1. 构建 Poll 请求的 TunnelPackage（续连接，只携带连接标识）
	pollPkg := &TunnelPackage{
		ConnectionID: connID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   sp.tunnelType,
		// Type 为空，表示只是续连接，等待服务器响应
	}
	encoded, err := EncodeTunnelPackage(pollPkg)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to encode poll package: %w", err)
	}

	// 2. 发送 Poll 请求
	req, err := http.NewRequestWithContext(sp.Ctx(), "GET", sp.pollURL+"?timeout=30", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create poll request: %w", err)
	}

	req.Header.Set("X-Tunnel-Package", encoded)
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	resp, err := sp.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("poll request failed: %w", err)
	}
	defer resp.Body.Close()

	// 3. 检查是否有控制包（X-Tunnel-Package 中）
	if resp.Header.Get("X-Tunnel-Package") != "" {
		pkt, err := sp.converter.ReadPacket(resp)
		if err != nil {
			return nil, 0, err
		}
		// 更新连接信息（如果响应中包含新的 ConnectionID）
		if pkg, _ := DecodeTunnelPackage(resp.Header.Get("X-Tunnel-Package")); pkg != nil {
			if pkg.ConnectionID != "" {
				sp.SetConnectionID(pkg.ConnectionID)
			}
		}
		return pkt, 0, nil
	}

	// 4. 读取 Body 数据流（如果有）
	var pollResp struct {
		Success bool   `json:"success"`
		Data    string `json:"data,omitempty"`
		Timeout bool   `json:"timeout,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pollResp); err == nil && pollResp.Data != "" {
		// Base64 解码
		data, err := base64.StdEncoding.DecodeString(pollResp.Data)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to decode base64 data: %w", err)
		}
		// 将数据放入缓冲，供 ReadExact 使用
		sp.dataBufMu.Lock()
		if sp.dataBuffer.Len()+len(data) > maxBufferSize {
			sp.dataBufMu.Unlock()
			return nil, 0, fmt.Errorf("buffer overflow")
		}
		sp.dataBuffer.Write(data)
		sp.dataBufMu.Unlock()
	}

	return nil, 0, nil
}

// WritePacket 通过 HTTP Push 发送包
func (sp *StreamProcessor) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	// 1. 更新转换器的连接状态
	sp.converter.SetConnectionInfo(sp.connectionID, sp.clientID, sp.mappingID, sp.tunnelType)

	// 2. 转换为 HTTP Request
	req, err := sp.converter.WritePacket(pkt)
	if err != nil {
		return 0, fmt.Errorf("failed to convert packet: %w", err)
	}

	// 3. 设置请求 URL 和认证
	reqURL, err := url.Parse(sp.pushURL)
	if err != nil {
		return 0, fmt.Errorf("invalid push URL: %w", err)
	}
	req.URL = reqURL
	req = req.WithContext(sp.Ctx())
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	// 4. 发送请求（带重试）
	var resp *http.Response
	for retry := 0; retry < maxRetries; retry++ {
		resp, err = sp.httpClient.Do(req)
		if err == nil {
			break
		}
		if retry < maxRetries-1 {
			time.Sleep(retryInterval * time.Duration(retry+1))
			// 重新创建请求
			req, _ = sp.converter.WritePacket(pkt)
			reqURL, _ := url.Parse(sp.pushURL)
			req.URL = reqURL
			req = req.WithContext(sp.Ctx())
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
		if pkg, _ := DecodeTunnelPackage(resp.Header.Get("X-Tunnel-Package")); pkg != nil {
			if pkg.ConnectionID != "" {
				sp.SetConnectionID(pkg.ConnectionID)
			}
		}
	}

	// 6. 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("push request failed: status %d, body: %s", resp.StatusCode, string(body))
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

	// Base64 编码数据
	encoded := base64.StdEncoding.EncodeToString(data)

	// 构建 HTTP Request
	// 数据流传输时，X-Tunnel-Package 只包含连接标识（用于路由）
	dataPkg := &TunnelPackage{
		ConnectionID: sp.connectionID,
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

