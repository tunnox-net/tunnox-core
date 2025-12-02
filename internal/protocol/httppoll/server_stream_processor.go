package httppoll

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
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

	// 数据队列（用于 Poll 响应 - 仅用于数据流，不用于控制包）
	pollDataQueue *session.PriorityQueue
	pollDataChan  chan []byte
	pollWaitChan  chan struct{}
	pollMu        sync.Mutex

	// 控制包响应通道（用于控制包，通过 X-Tunnel-Package header 返回）
	controlPacketChan chan *packet.TransferPacket
	controlPacketMu   sync.Mutex

	// 等待控制包的 Poll 请求队列（requestID -> pollRequestInfo）
	pendingPollRequests map[string]*pollRequestInfo
	pendingPollMu       sync.Mutex

	// 待分配的控制包队列（等待匹配的 Poll 请求）
	pendingControlPackets []*packet.TransferPacket
	pendingControlMu      sync.Mutex

	// 读取缓冲区（用于从 Push 请求读取数据）
	readBuffer []byte
	readBufMu  sync.Mutex

	// 控制
	closed  bool
	closeMu sync.RWMutex

	// HTTP 请求/响应通道（用于与 HTTP handler 通信）
	pushDataChan chan string // Base64 编码的数据
}

// pollRequestInfo 等待控制包的 Poll 请求信息
type pollRequestInfo struct {
	responseChan chan *TunnelPackage
	tunnelType   string // "control" | "data" | "keepalive"
}

// NewServerStreamProcessor 创建服务端 HTTP 长轮询流处理器
func NewServerStreamProcessor(ctx context.Context, connID string, clientID int64, mappingID string) *ServerStreamProcessor {
	connType := "control"
	if mappingID != "" {
		connType = "data"
	}

	sp := &ServerStreamProcessor{
		ManagerBase:           dispose.NewManager("ServerHTTPStreamProcessor", ctx),
		converter:             NewPacketConverter(),
		connectionID:          connID,
		clientID:              clientID,
		mappingID:             mappingID,
		tunnelType:            connType,
		pollDataQueue:         session.NewPriorityQueue(3),
		pollDataChan:          make(chan []byte, 1),
		pollWaitChan:          make(chan struct{}, 1),
		controlPacketChan:     make(chan *packet.TransferPacket, 10), // 控制包通道
		pushDataChan:          make(chan string, 100),
		pendingPollRequests:   make(map[string]*pollRequestInfo),
		pendingControlPackets: make([]*packet.TransferPacket, 0),
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
	utils.Infof("ServerStreamProcessor: onClose called, connID=%s", sp.connectionID)
	sp.closed = true

	close(sp.pollDataChan)
	close(sp.pushDataChan)
	close(sp.controlPacketChan)
	// 清空队列（通过不断 Pop 直到为空）
	for {
		if _, ok := sp.pollDataQueue.Pop(); !ok {
			break
		}
	}

	// 清理等待队列
	sp.pendingPollMu.Lock()
	for _, info := range sp.pendingPollRequests {
		close(info.responseChan)
	}
	sp.pendingPollRequests = make(map[string]*pollRequestInfo)
	sp.pendingPollMu.Unlock()

	// 清理待分配的控制包队列
	sp.pendingControlMu.Lock()
	sp.pendingControlPackets = nil
	sp.pendingControlMu.Unlock()

	return nil
}

// tryMatchControlPacket 尝试将待分配的控制包匹配给等待的 Poll 请求
// 每次调用处理所有可匹配的控制包，直到没有等待的 Poll 请求或没有待分配的控制包
func (sp *ServerStreamProcessor) tryMatchControlPacket() {
	for {
		sp.pendingControlMu.Lock()
		if len(sp.pendingControlPackets) == 0 {
			sp.pendingControlMu.Unlock()
			return
		}
		// 取出第一个控制包
		controlPkt := sp.pendingControlPackets[0]
		sp.pendingControlPackets = sp.pendingControlPackets[1:]
		pendingCount := len(sp.pendingControlPackets)
		sp.pendingControlMu.Unlock()

		responsePkg := sp.transferPacketToTunnelPackage(controlPkt)

		// [CMD_TRACE] 尝试匹配控制包
		var controlCommandID string
		if controlPkt.CommandPacket != nil {
			controlCommandID = controlPkt.CommandPacket.CommandId
		}
		baseType := byte(controlPkt.PacketType) & 0x3F
		utils.Infof("[CMD_TRACE] [SERVER] [MATCH_START] ConnID=%s, CommandID=%s, PacketType=0x%02x, PendingCount=%d, Time=%s",
			sp.connectionID, controlCommandID, baseType, pendingCount, time.Now().Format("15:04:05.000"))

		// 检查是否有等待的 Poll 请求（优先匹配有 requestID 的，且不是 keepalive 类型）
		sp.pendingPollMu.Lock()
		var targetChan chan *TunnelPackage
		var targetRequestID string
		var availablePollCount int
		var keepaliveCount int
		// 记录所有等待的 Poll 请求信息
		for reqID, info := range sp.pendingPollRequests {
			availablePollCount++
			if info.tunnelType == "keepalive" {
				keepaliveCount++
			}
			if reqID != "" && !strings.HasPrefix(reqID, "legacy-") && info.tunnelType != "keepalive" {
				targetChan = info.responseChan
				targetRequestID = reqID
				responsePkg.RequestID = reqID
				delete(sp.pendingPollRequests, reqID)
				break
			}
		}

		// 如果没有有 requestID 的请求，选择第一个非 keepalive 的请求
		if targetChan == nil {
			for reqID, info := range sp.pendingPollRequests {
				if info.tunnelType != "keepalive" {
					targetChan = info.responseChan
					targetRequestID = reqID
					if reqID != "" {
						responsePkg.RequestID = reqID
					}
					delete(sp.pendingPollRequests, reqID)
					break
				}
			}
		}
		sp.pendingPollMu.Unlock()

		utils.Infof("[CMD_TRACE] [SERVER] [MATCH_CHECK] ConnID=%s, CommandID=%s, AvailablePollCount=%d, KeepaliveCount=%d, MatchedRequestID=%s, Time=%s",
			sp.connectionID, controlCommandID, availablePollCount, keepaliveCount, targetRequestID, time.Now().Format("15:04:05.000"))

		if targetChan != nil {
			// 有等待的请求，直接发送（使用该请求的 requestID）
			select {
			case targetChan <- responsePkg:
				utils.Infof("ServerStreamProcessor: tryMatchControlPacket - control packet matched to waiting Poll request, requestID=%s, connID=%s, remainingPackets=%d",
					targetRequestID, sp.connectionID, pendingCount)
				// 继续循环，尝试匹配下一个控制包
				continue
			default:
				// 通道已关闭或满，重新放回队列
				sp.pendingControlMu.Lock()
				sp.pendingControlPackets = append([]*packet.TransferPacket{controlPkt}, sp.pendingControlPackets...)
				sp.pendingControlMu.Unlock()
				utils.Warnf("ServerStreamProcessor: tryMatchControlPacket - response channel full, requeued, requestID=%s", targetRequestID)
				return // 通道满，停止匹配
			}
		} else {
			// 没有等待的请求，重新放回队列（而不是放入 controlPacketChan，避免 FIFO 匹配错误）
			sp.pendingControlMu.Lock()
			sp.pendingControlPackets = append([]*packet.TransferPacket{controlPkt}, sp.pendingControlPackets...)
			pendingCount = len(sp.pendingControlPackets)
			sp.pendingControlMu.Unlock()
			utils.Infof("[CMD_TRACE] [SERVER] [MATCH_FAILED] ConnID=%s, CommandID=%s, Reason=no_waiting_poll_request, AvailablePollCount=%d, KeepaliveCount=%d, Action=requeued_to_pendingControlPackets, PendingCount=%d, Time=%s",
				sp.connectionID, controlCommandID, availablePollCount, keepaliveCount, pendingCount, time.Now().Format("15:04:05.000"))
			utils.Debugf("ServerStreamProcessor: tryMatchControlPacket - control packet requeued (no waiting requests), connID=%s, remainingPackets=%d", sp.connectionID, pendingCount)
			return // 没有等待的请求，停止匹配
		}
	}
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
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		utils.Errorf("ServerStreamProcessor: WritePacket called but connection is closed, connID=%s", connID)
		return 0, io.ErrClosedPipe
	}

	// [CMD_TRACE] 服务端发送响应开始
	writeStartTime := time.Now()
	baseType := byte(pkt.PacketType) & 0x3F
	var commandID string
	if pkt.CommandPacket != nil {
		commandID = pkt.CommandPacket.CommandId
	}
	utils.Infof("[CMD_TRACE] [SERVER] [SEND_START] ConnID=%s, CommandID=%s, PacketType=0x%02x, PayloadLen=%d, Time=%s",
		connID, commandID, baseType, len(pkt.Payload), writeStartTime.Format("15:04:05.000"))

	// 检查是否是控制包（应该通过 X-Tunnel-Package header 返回）
	isControlPacket := baseType == byte(packet.HandshakeResp) ||
		baseType == byte(packet.TunnelOpenAck) ||
		pkt.PacketType.IsCommandResp() ||
		pkt.PacketType.IsJsonCommand()

	utils.Infof("ServerStreamProcessor: WritePacket - isControlPacket=%v, HandshakeResp=0x%02x, baseType=0x%02x, connID=%s",
		isControlPacket, byte(packet.HandshakeResp), baseType, sp.connectionID)

	if isControlPacket {
		// 控制包：放入待分配队列，等待匹配的 Poll 请求
		// 每个 requestId 只能用一次，如果有多个控制包需要推送，需要等待多个 Poll 请求
		utils.Infof("[CMD_TRACE] [SERVER] [SEND_QUEUE] ConnID=%s, CommandID=%s, PacketType=0x%02x, Action=queued_for_poll, Time=%s",
			connID, commandID, baseType, time.Now().Format("15:04:05.000"))

		sp.pendingControlMu.Lock()
		sp.pendingControlPackets = append(sp.pendingControlPackets, pkt)
		pendingCount := len(sp.pendingControlPackets)
		sp.pendingControlMu.Unlock()

		utils.Infof("[CMD_TRACE] [SERVER] [SEND_QUEUE] ConnID=%s, CommandID=%s, PendingCount=%d, Time=%s",
			connID, commandID, pendingCount, time.Now().Format("15:04:05.000"))

		// 尝试立即匹配等待的 Poll 请求
		sp.tryMatchControlPacket()

		// 通知等待的 Poll 请求
		select {
		case sp.pollWaitChan <- struct{}{}:
			utils.Infof("[CMD_TRACE] [SERVER] [SEND_NOTIFY] ConnID=%s, CommandID=%s, Action=notified_poll_waiters, Time=%s",
				connID, commandID, time.Now().Format("15:04:05.000"))
		default:
			utils.Warnf("[CMD_TRACE] [SERVER] [SEND_NOTIFY_FAILED] ConnID=%s, CommandID=%s, Reason=pollWaitChan_full, Time=%s",
				connID, commandID, time.Now().Format("15:04:05.000"))
		}
		utils.Infof("[CMD_TRACE] [SERVER] [SEND_COMPLETE] ConnID=%s, CommandID=%s, Duration=%v, Time=%s",
			connID, commandID, time.Since(writeStartTime), time.Now().Format("15:04:05.000"))
		return len(pkt.Payload), nil
	}

	// 数据流包：序列化后放入数据队列，通过 body 返回
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

	// 推送到优先级队列（数据流）
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
// requestID 是客户端请求中的 RequestId，用于在响应中携带
// tunnelType 是请求的 TunnelType（"control" | "data" | "keepalive"），用于区分请求类型
func (sp *ServerStreamProcessor) HandlePollRequest(ctx context.Context, requestID string, tunnelType string) (string, *TunnelPackage, error) {
	// 如果 requestID 为空，生成一个临时 ID（用于兼容旧代码）
	actualRequestID := requestID
	if actualRequestID == "" {
		actualRequestID = "legacy-" + fmt.Sprintf("%d", time.Now().UnixNano())
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - generated legacy requestID=%s, connID=%s", actualRequestID, sp.connectionID)
	}

	// keepalive 请求不应该注册到等待队列，因为它们不应该接收控制包
	if tunnelType == "keepalive" {
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - keepalive request, not registering for control packets, requestID=%s, connID=%s", actualRequestID, sp.connectionID)
		// keepalive 请求只等待数据流，不等待控制包
		// 直接返回超时（因为没有数据流）
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-sp.Ctx().Done():
			return "", nil, sp.Ctx().Err()
		case <-time.After(28 * time.Second):
			return "", nil, context.DeadlineExceeded
		}
	}

	// 创建响应通道
	responseChan := make(chan *TunnelPackage, 1)

	// 注册等待请求（只注册非 keepalive 请求）
	sp.pendingPollMu.Lock()
	sp.pendingPollRequests[actualRequestID] = &pollRequestInfo{
		responseChan: responseChan,
		tunnelType:   tunnelType,
	}
	sp.pendingPollMu.Unlock()

	// 清理函数：如果请求超时或取消，从等待队列中移除
	defer func() {
		sp.pendingPollMu.Lock()
		if info, exists := sp.pendingPollRequests[actualRequestID]; exists {
			delete(sp.pendingPollRequests, actualRequestID)
			close(info.responseChan)
		}
		sp.pendingPollMu.Unlock()
	}()

	// [CMD_TRACE] 服务端 Poll 请求开始
	pollStartTime := time.Now()
	utils.Infof("[CMD_TRACE] [SERVER] [POLL_START] ConnID=%s, RequestID=%s, TunnelType=%s, Time=%s",
		sp.connectionID, actualRequestID, tunnelType, pollStartTime.Format("15:04:05.000"))

	// 尝试匹配待分配的控制包（从 pendingControlPackets）
	// 由于 Poll 请求已注册，tryMatchControlPacket 应该能够匹配到
	sp.tryMatchControlPacket()

	// 先检查响应通道（可能已经匹配到控制包）
	select {
	case responsePkg := <-responseChan:
		// 从等待队列收到控制包（已匹配 requestID）
		pollDuration := time.Since(pollStartTime)
		var responseType string
		var responseCommandID string
		if responsePkg != nil {
			responseType = responsePkg.Type
			if responsePkg.Data != nil {
				if cmdPkg, ok := responsePkg.Data.(*packet.CommandPacket); ok {
					responseCommandID = cmdPkg.CommandId
				}
			}
		}
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_MATCHED_IMMEDIATE] ConnID=%s, RequestID=%s, ResponseType=%s, CommandID=%s, PollDuration=%v, Time=%s",
			sp.connectionID, actualRequestID, responseType, responseCommandID, pollDuration, time.Now().Format("15:04:05.000"))
		utils.Infof("ServerStreamProcessor: HandlePollRequest - control packet received immediately from waiting queue (type=%s), connID=%s, requestID=%s",
			responsePkg.Type, sp.connectionID, actualRequestID)
		return "", responsePkg, nil
	default:
		// 没有控制包，继续等待
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_WAIT] ConnID=%s, RequestID=%s, Reason=no_immediate_control_packet, Time=%s",
			sp.connectionID, actualRequestID, time.Now().Format("15:04:05.000"))
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - no control packet immediately, waiting, connID=%s, requestID=%s", sp.connectionID, actualRequestID)
	}

	// 从队列获取数据流（非阻塞检查）
	var base64Data string
	if data, ok := sp.pollDataQueue.Pop(); ok {
		base64Data = base64.StdEncoding.EncodeToString(data)
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received, len=%d, connID=%s", len(base64Data), sp.connectionID)
		return base64Data, nil, nil
	}

	// 队列为空，阻塞等待（控制包或数据流）
	waitStartTime := time.Now()
	utils.Infof("[CMD_TRACE] [SERVER] [POLL_WAIT_START] ConnID=%s, RequestID=%s, Time=%s",
		sp.connectionID, actualRequestID, waitStartTime.Format("15:04:05.000"))
	select {
	case <-ctx.Done():
		waitDuration := time.Since(waitStartTime)
		pollDuration := time.Since(pollStartTime)
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_TIMEOUT] ConnID=%s, RequestID=%s, WaitDuration=%v, PollDuration=%v, Reason=context_done, Time=%s",
			sp.connectionID, actualRequestID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
		return "", nil, ctx.Err()
	case <-sp.Ctx().Done():
		waitDuration := time.Since(waitStartTime)
		pollDuration := time.Since(pollStartTime)
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_TIMEOUT] ConnID=%s, RequestID=%s, WaitDuration=%v, PollDuration=%v, Reason=connection_closed, Time=%s",
			sp.connectionID, actualRequestID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
		return "", nil, sp.Ctx().Err()
	case responsePkg := <-responseChan:
		// 从等待队列收到控制包（已匹配 requestID）
		waitDuration := time.Since(waitStartTime)
		pollDuration := time.Since(pollStartTime)
		var responseType string
		var responseCommandID string
		if responsePkg != nil {
			responseType = responsePkg.Type
			if responsePkg.Data != nil {
				if cmdPkg, ok := responsePkg.Data.(*packet.CommandPacket); ok {
					responseCommandID = cmdPkg.CommandId
				}
			}
		}
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_MATCHED] ConnID=%s, RequestID=%s, ResponseType=%s, CommandID=%s, WaitDuration=%v, PollDuration=%v, Time=%s",
			sp.connectionID, actualRequestID, responseType, responseCommandID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
		utils.Infof("ServerStreamProcessor: HandlePollRequest - control packet received from waiting queue (type=%s), connID=%s, requestID=%s",
			responsePkg.Type, sp.connectionID, actualRequestID)
		return "", responsePkg, nil
	// 注意：不再使用 controlPacketChan，所有控制包都通过 pendingControlPackets 和 tryMatchControlPacket 匹配
	case <-sp.pollWaitChan:
		// 收到通知，检查队列
		if data, ok := sp.pollDataQueue.Pop(); ok {
			base64Data = base64.StdEncoding.EncodeToString(data)
			utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received after wait, len=%d, connID=%s", len(base64Data), sp.connectionID)
			return base64Data, nil, nil
		}
		// 如果队列仍为空，尝试匹配控制包（可能新的控制包已到达）
		sp.tryMatchControlPacket()
		// 检查响应通道（可能已匹配到控制包）
		select {
		case responsePkg := <-responseChan:
			// 从等待队列收到控制包（已匹配 requestID）
			waitDuration := time.Since(waitStartTime)
			pollDuration := time.Since(pollStartTime)
			var responseType string
			var responseCommandID string
			if responsePkg != nil {
				responseType = responsePkg.Type
				if responsePkg.Data != nil {
					if cmdPkg, ok := responsePkg.Data.(*packet.CommandPacket); ok {
						responseCommandID = cmdPkg.CommandId
					}
				}
			}
			utils.Infof("[CMD_TRACE] [SERVER] [POLL_MATCHED] ConnID=%s, RequestID=%s, ResponseType=%s, CommandID=%s, WaitDuration=%v, PollDuration=%v, Time=%s",
				sp.connectionID, actualRequestID, responseType, responseCommandID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
			utils.Infof("ServerStreamProcessor: HandlePollRequest - control packet received from waiting queue (type=%s), connID=%s, requestID=%s",
				responsePkg.Type, sp.connectionID, actualRequestID)
			return "", responsePkg, nil
		case data, ok := <-sp.pollDataChan:
			if !ok {
				return "", nil, io.EOF
			}
			base64Data = base64.StdEncoding.EncodeToString(data)
			utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received from chan, len=%d, connID=%s", len(base64Data), sp.connectionID)
			return base64Data, nil, nil
		default:
			// 继续等待（回到外层 select 循环）
		}
		// 继续等待（回到外层 select，通过循环）
		// 使用 for 循环重新进入等待
		for {
			select {
			case <-ctx.Done():
				waitDuration := time.Since(waitStartTime)
				pollDuration := time.Since(pollStartTime)
				utils.Infof("[CMD_TRACE] [SERVER] [POLL_TIMEOUT] ConnID=%s, RequestID=%s, WaitDuration=%v, PollDuration=%v, Reason=context_done, Time=%s",
					sp.connectionID, actualRequestID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
				return "", nil, ctx.Err()
			case <-sp.Ctx().Done():
				waitDuration := time.Since(waitStartTime)
				pollDuration := time.Since(pollStartTime)
				utils.Infof("[CMD_TRACE] [SERVER] [POLL_TIMEOUT] ConnID=%s, RequestID=%s, WaitDuration=%v, PollDuration=%v, Reason=connection_closed, Time=%s",
					sp.connectionID, actualRequestID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
				return "", nil, sp.Ctx().Err()
			case responsePkg := <-responseChan:
				// 从等待队列收到控制包（已匹配 requestID）
				waitDuration := time.Since(waitStartTime)
				pollDuration := time.Since(pollStartTime)
				var responseType string
				var responseCommandID string
				if responsePkg != nil {
					responseType = responsePkg.Type
					if responsePkg.Data != nil {
						if cmdPkg, ok := responsePkg.Data.(*packet.CommandPacket); ok {
							responseCommandID = cmdPkg.CommandId
						}
					}
				}
				utils.Infof("[CMD_TRACE] [SERVER] [POLL_MATCHED] ConnID=%s, RequestID=%s, ResponseType=%s, CommandID=%s, WaitDuration=%v, PollDuration=%v, Time=%s",
					sp.connectionID, actualRequestID, responseType, responseCommandID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
				utils.Infof("ServerStreamProcessor: HandlePollRequest - control packet received from waiting queue (type=%s), connID=%s, requestID=%s",
					responsePkg.Type, sp.connectionID, actualRequestID)
				return "", responsePkg, nil
			case <-sp.pollWaitChan:
				// 收到通知，检查队列
				if data, ok := sp.pollDataQueue.Pop(); ok {
					base64Data = base64.StdEncoding.EncodeToString(data)
					utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received after wait, len=%d, connID=%s", len(base64Data), sp.connectionID)
					return base64Data, nil, nil
				}
				// 尝试匹配控制包
				sp.tryMatchControlPacket()
			case data, ok := <-sp.pollDataChan:
				if !ok {
					return "", nil, io.EOF
				}
				base64Data = base64.StdEncoding.EncodeToString(data)
				utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received from chan, len=%d, connID=%s", len(base64Data), sp.connectionID)
				return base64Data, nil, nil
			case <-time.After(100 * time.Millisecond):
				// 定期尝试匹配控制包
				sp.tryMatchControlPacket()
			}
		}
	case data, ok := <-sp.pollDataChan:
		if !ok {
			return "", nil, io.EOF
		}
		base64Data = base64.StdEncoding.EncodeToString(data)
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received from chan, len=%d, connID=%s", len(base64Data), sp.connectionID)
		return base64Data, nil, nil
	}
}

// transferPacketToTunnelPackage 将 TransferPacket 转换为 TunnelPackage
func (sp *ServerStreamProcessor) transferPacketToTunnelPackage(pkt *packet.TransferPacket) *TunnelPackage {
	baseType := byte(pkt.PacketType) & 0x3F

	responsePkg := &TunnelPackage{
		ConnectionID: sp.connectionID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   sp.tunnelType, // 保持为 "control" 或 "data"，不是 "keepalive"
		// 注意：即使是通过 keepalive 请求返回的，响应包本身也是指令包，TunnelType 应该是 "control" 或 "data"
	}

	// 根据包类型设置 Type 和 Data
	if baseType == byte(packet.HandshakeResp) {
		responsePkg.Type = "HandshakeResponse"
		var handshakeResp packet.HandshakeResponse
		if err := json.Unmarshal(pkt.Payload, &handshakeResp); err == nil {
			responsePkg.Data = &handshakeResp
		}
	} else if baseType == byte(packet.TunnelOpenAck) {
		responsePkg.Type = "TunnelOpenAck"
		var tunnelOpenAck packet.TunnelOpenAckResponse
		if err := json.Unmarshal(pkt.Payload, &tunnelOpenAck); err == nil {
			responsePkg.Data = &tunnelOpenAck
		}
	} else if pkt.PacketType.IsCommandResp() {
		responsePkg.Type = "CommandResponse"
		if pkt.CommandPacket != nil {
			responsePkg.Data = pkt.CommandPacket
		}
	} else if pkt.PacketType.IsJsonCommand() {
		responsePkg.Type = "JsonCommand"
		if pkt.CommandPacket != nil {
			responsePkg.Data = pkt.CommandPacket
		}
	}

	return responsePkg
}

// HTTPPushRequest HTTP 推送请求结构（用于服务端）
type HTTPPushRequest struct {
	Data      string `json:"data"`
	Seq       uint64 `json:"seq"`
	Timestamp int64  `json:"timestamp"`
}

// 确保 ServerStreamProcessor 实现 stream.PackageStreamer 接口
var _ stream.PackageStreamer = (*ServerStreamProcessor)(nil)
