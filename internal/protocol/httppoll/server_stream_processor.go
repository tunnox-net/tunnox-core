package httppoll

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
)

// ServerStreamProcessor 服务端 HTTP 长轮询流处理器
// 实现 stream.PackageStreamer 接口，用于服务端处理 HTTP 请求/响应
type ServerStreamProcessor struct {
	*dispose.ManagerBase

	converter *PacketConverter

	// 连接信息
	connectionID string
	clientID     int64
	mappingID    string
	tunnelType   string

	// 数据队列（用于 Poll 响应）
	pollDataQueue *session.PriorityQueue
	pollDataChan  chan []byte
	pollWaitChan  chan struct{}
	pollMu        sync.Mutex

	// 读取缓冲区（用于从 Push 请求读取数据）
	readBuffer []byte
	readBufMu  sync.Mutex

	// 控制
	closed  bool
	closeMu sync.RWMutex

	// HTTP 请求/响应通道（用于与 HTTP handler 通信）
	pushDataChan chan string // Base64 编码的数据
}

// NewServerStreamProcessor 创建服务端 HTTP 长轮询流处理器
func NewServerStreamProcessor(ctx context.Context, connID string, clientID int64, mappingID string) *ServerStreamProcessor {
	connType := "control"
	if mappingID != "" {
		connType = "data"
	}

	sp := &ServerStreamProcessor{
		ManagerBase:  dispose.NewManager("ServerHTTPStreamProcessor", ctx),
		converter:    NewPacketConverter(),
		connectionID: connID,
		clientID:     clientID,
		mappingID:    mappingID,
		tunnelType:   connType,
		pollDataQueue: session.NewPriorityQueue(3),
		pollDataChan:  make(chan []byte, 1),
		pollWaitChan:  make(chan struct{}, 1),
		pushDataChan: make(chan string, 100),
	}

	sp.converter.SetConnectionInfo(connID, clientID, mappingID, connType)

	sp.AddCleanHandler(sp.onClose)

	// 启动优先级队列调度循环
	go sp.pollDataScheduler()

	return sp
}

// onClose 资源清理
func (sp *ServerStreamProcessor) onClose() error {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()

	if sp.closed {
		return nil
	}
	sp.closed = true

	close(sp.pollDataChan)
	close(sp.pushDataChan)
	// 清空队列（通过不断 Pop 直到为空）
	for {
		if _, ok := sp.pollDataQueue.Pop(); !ok {
			break
		}
	}

	return nil
}

// SetConnectionID 设置连接 ID
func (sp *ServerStreamProcessor) SetConnectionID(connID string) {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()
	sp.connectionID = connID
	sp.converter.SetConnectionInfo(connID, sp.clientID, sp.mappingID, sp.tunnelType)
}

// UpdateClientID 更新客户端 ID
func (sp *ServerStreamProcessor) UpdateClientID(newClientID int64) {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()
	sp.clientID = newClientID
	sp.converter.SetConnectionInfo(sp.connectionID, newClientID, sp.mappingID, sp.tunnelType)
}

// SetMappingID 设置映射 ID
func (sp *ServerStreamProcessor) SetMappingID(mappingID string) {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()
	sp.mappingID = mappingID
	if mappingID != "" {
		sp.tunnelType = "data"
	} else {
		sp.tunnelType = "control"
	}
	sp.converter.SetConnectionInfo(sp.connectionID, sp.clientID, mappingID, sp.tunnelType)
}

// GetConnectionID 获取连接 ID
func (sp *ServerStreamProcessor) GetConnectionID() string {
	sp.closeMu.RLock()
	defer sp.closeMu.RUnlock()
	return sp.connectionID
}

// GetClientID 获取客户端 ID
func (sp *ServerStreamProcessor) GetClientID() int64 {
	sp.closeMu.RLock()
	defer sp.closeMu.RUnlock()
	return sp.clientID
}

// GetMappingID 获取映射 ID
func (sp *ServerStreamProcessor) GetMappingID() string {
	sp.closeMu.RLock()
	defer sp.closeMu.RUnlock()
	return sp.mappingID
}

// ReadPacket 从 HTTP Push 请求读取包
// 注意：服务端不能主动读取，只能从 Push 请求中获取
func (sp *ServerStreamProcessor) ReadPacket() (*packet.TransferPacket, int, error) {
	// 服务端通过 handleHTTPPush 处理 Push 请求
	// 这里返回错误，表示需要通过 HTTP handler 处理
	return nil, 0, fmt.Errorf("server ReadPacket should be called from HTTP handler")
}

// WritePacket 通过 Poll 响应发送包
func (sp *ServerStreamProcessor) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	// 序列化包
	var data []byte
	if pkt.PacketType.IsHeartbeat() {
		// 心跳包只有 1 字节
		data = []byte{byte(pkt.PacketType)}
	} else {
		// 其他包：类型(1) + 大小(4) + 数据
		typeByte := []byte{byte(pkt.PacketType)}
		var bodyData []byte
		if (pkt.PacketType.IsJsonCommand() || pkt.PacketType.IsCommandResp()) && pkt.CommandPacket != nil {
			bodyData, _ = json.Marshal(pkt.CommandPacket)
		} else if len(pkt.Payload) > 0 {
			bodyData = pkt.Payload
		}
		if len(bodyData) > 0 {
			sizeBytes := make([]byte, 4)
			sizeBytes[0] = byte(len(bodyData) >> 24)
			sizeBytes[1] = byte(len(bodyData) >> 16)
			sizeBytes[2] = byte(len(bodyData) >> 8)
			sizeBytes[3] = byte(len(bodyData))
			data = append(typeByte, sizeBytes...)
			data = append(data, bodyData...)
		} else {
			data = typeByte
		}
	}

	// 推送到优先级队列
	sp.pollDataQueue.Push(data)

	// 通知等待的 Poll 请求
	select {
	case sp.pollWaitChan <- struct{}{}:
	default:
	}

	return len(data), nil
}

// WriteExact 将数据流写入 Poll 响应
func (sp *ServerStreamProcessor) WriteExact(data []byte) error {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return io.ErrClosedPipe
	}

	// 推送到优先级队列
	sp.pollDataQueue.Push(data)

	// 通知等待的 Poll 请求
	select {
	case sp.pollWaitChan <- struct{}{}:
	default:
	}

	return nil
}

// ReadExact 从 Push 请求读取数据流
func (sp *ServerStreamProcessor) ReadExact(length int) ([]byte, error) {
	sp.readBufMu.Lock()
	defer sp.readBufMu.Unlock()

	// 从缓冲读取，如果不够则等待更多数据
	for len(sp.readBuffer) < length {
		sp.readBufMu.Unlock()
		// 等待从 pushDataChan 接收更多数据
		select {
		case <-sp.Ctx().Done():
			return nil, sp.Ctx().Err()
		case base64Data, ok := <-sp.pushDataChan:
			if !ok {
				return nil, io.EOF
			}
			// Base64 解码
			data, err := base64.StdEncoding.DecodeString(base64Data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode base64: %w", err)
			}
			sp.readBufMu.Lock()
			sp.readBuffer = append(sp.readBuffer, data...)
			sp.readBufMu.Unlock()
		}
		sp.readBufMu.Lock()
	}

	// 读取指定长度
	data := make([]byte, length)
	n := copy(data, sp.readBuffer)
	sp.readBuffer = sp.readBuffer[n:]

	if n < length {
		return nil, io.ErrUnexpectedEOF
	}

	return data, nil
}

// GetReader 获取底层 Reader（返回 nil，HTTP 无状态）
func (sp *ServerStreamProcessor) GetReader() io.Reader {
	return nil
}

// GetWriter 获取底层 Writer（返回 nil，HTTP 无状态）
func (sp *ServerStreamProcessor) GetWriter() io.Writer {
	return nil
}

// Close 关闭连接
func (sp *ServerStreamProcessor) Close() {
	sp.Dispose.CloseWithError()
}

// PushData 从 HTTP Push 请求接收数据（由 handleHTTPPush 调用）
func (sp *ServerStreamProcessor) PushData(base64Data string) error {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return io.ErrClosedPipe
	}

	select {
	case <-sp.Ctx().Done():
		return sp.Ctx().Err()
	case sp.pushDataChan <- base64Data:
		return nil
	default:
		return io.ErrShortWrite
	}
}

// PollData 等待数据用于 HTTP Poll 响应（由 handleHTTPPoll 调用）
func (sp *ServerStreamProcessor) PollData(ctx context.Context) (string, error) {
	// 先检查队列中是否有数据（非阻塞）
	if data, ok := sp.pollDataQueue.Pop(); ok {
		base64Data := base64.StdEncoding.EncodeToString(data)
		return base64Data, nil
	}

	// 队列为空，阻塞等待调度器推送数据
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-sp.Ctx().Done():
		return "", sp.Ctx().Err()
	case <-sp.pollWaitChan:
		// 收到信号，立即检查队列
		if data, ok := sp.pollDataQueue.Pop(); ok {
			base64Data := base64.StdEncoding.EncodeToString(data)
			return base64Data, nil
		}
		// 如果队列仍为空，继续等待 pollDataChan
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-sp.Ctx().Done():
			return "", sp.Ctx().Err()
		case data, ok := <-sp.pollDataChan:
			if !ok {
				return "", io.EOF
			}
			base64Data := base64.StdEncoding.EncodeToString(data)
			return base64Data, nil
		}
	case data, ok := <-sp.pollDataChan:
		if !ok {
			return "", io.EOF
		}
		base64Data := base64.StdEncoding.EncodeToString(data)
		return base64Data, nil
	}
}

// pollDataScheduler 优先级队列调度循环
func (sp *ServerStreamProcessor) pollDataScheduler() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sp.Ctx().Done():
			return
		case <-ticker.C:
			// 定期检查队列，如果有数据且 pollDataChan 为空，则推送
			for {
				data, ok := sp.pollDataQueue.Pop()
				if !ok {
					break
				}
				select {
				case <-sp.Ctx().Done():
					sp.pollDataQueue.Push(data)
					return
				case sp.pollDataChan <- data:
					// 通知 PollData 有数据可用
					select {
					case sp.pollWaitChan <- struct{}{}:
					default:
					}
				default:
					// pollDataChan 已满，将数据放回队列
					sp.pollDataQueue.Push(data)
					break
				}
			}
		}
	}
}

// HandlePushRequest 处理 HTTP Push 请求（从 handleHTTPPush 调用）
func (sp *ServerStreamProcessor) HandlePushRequest(pkg *TunnelPackage, pushReq *HTTPPushRequest) (*TunnelPackage, error) {
	// 更新连接信息
	if pkg.ClientID > 0 {
		sp.UpdateClientID(pkg.ClientID)
	}
	if pkg.MappingID != "" {
		sp.SetMappingID(pkg.MappingID)
	}

	// 处理控制包
	var responsePkg *TunnelPackage
	if pkg.Type != "" {
		// 转换为 TransferPacket
		pkt, err := TunnelPackageToTransferPacket(pkg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert tunnel package: %w", err)
		}

		// 这里应该通过 SessionManager 处理包
		// 暂时返回 nil，由上层处理
		responsePkg = nil
		_ = pkt // 避免未使用变量
	}

	// 处理数据流
	if pushReq != nil && pushReq.Data != "" {
		if err := sp.PushData(pushReq.Data); err != nil {
			return nil, fmt.Errorf("failed to push data: %w", err)
		}
	}

	return responsePkg, nil
}

// HandlePollRequest 处理 HTTP Poll 请求（从 handleHTTPPoll 调用）
func (sp *ServerStreamProcessor) HandlePollRequest(ctx context.Context) (string, *TunnelPackage, error) {
	// 从队列获取数据
	base64Data, err := sp.PollData(ctx)
	if err != nil {
		return "", nil, err
	}

	// 检查是否有控制包响应（从队列中获取）
	// 这里简化处理，实际应该从 SessionManager 获取响应
	var responsePkg *TunnelPackage

	return base64Data, responsePkg, nil
}

// HTTPPushRequest HTTP 推送请求结构（用于服务端）
type HTTPPushRequest struct {
	Data      string `json:"data"`
	Seq       uint64 `json:"seq"`
	Timestamp int64  `json:"timestamp"`
}

// 确保 ServerStreamProcessor 实现 stream.PackageStreamer 接口
var _ stream.PackageStreamer = (*ServerStreamProcessor)(nil)

