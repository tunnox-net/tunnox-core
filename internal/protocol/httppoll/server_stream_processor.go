package httppoll

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	// 控制包响应通道（用于控制包，通过 X-Tunnel-Package header 返回）
	controlPacketChan chan *packet.TransferPacket

	// 等待控制包的 Poll 请求队列（requestID -> pollRequestInfo）
	pendingPollRequests map[string]*pollRequestInfo
	pendingPollMu       sync.Mutex

	// 待分配的控制包队列（等待匹配的 Poll 请求）
	pendingControlPackets []*packet.TransferPacket
	pendingControlMu      sync.Mutex

	// 读取缓冲区（用于从 Push 请求读取数据）
	readBuffer []byte
	readBufMu  sync.Mutex

	// 分片重组器（用于接收端重组分片）
	fragmentReassembler *FragmentReassembler

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
		pollDataChan:          make(chan []byte, 100),                // 增加容量，避免阻塞
		pollWaitChan:          make(chan struct{}, 10),               // 增加容量，避免丢失通知
		controlPacketChan:     make(chan *packet.TransferPacket, 10), // 控制包通道
		pushDataChan:          make(chan string, 1000),               // 增加容量，支持大数据包分片
		pendingPollRequests:   make(map[string]*pollRequestInfo),
		pendingControlPackets: make([]*packet.TransferPacket, 0),
		fragmentReassembler:   NewFragmentReassembler(), // 创建分片重组器
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
	utils.Debugf("ServerStreamProcessor: onClose called, connID=%s", sp.connectionID)
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

// GetFragmentReassembler 获取分片重组器
func (sp *ServerStreamProcessor) GetFragmentReassembler() *FragmentReassembler {
	return sp.fragmentReassembler
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

	utils.Debugf("ServerStreamProcessor: WritePacket - isControlPacket=%v, HandshakeResp=0x%02x, baseType=0x%02x, connID=%s",
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

// 确保 ServerStreamProcessor 实现 stream.PackageStreamer 接口
var _ stream.PackageStreamer = (*ServerStreamProcessor)(nil)
