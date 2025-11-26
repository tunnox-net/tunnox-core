package bridge

import (
	"context"
	"io"
	"sync"
	"time"
	pb "tunnox-core/api/proto/bridge"
	"tunnox-core/internal/utils"
)

// GRPCBridgeServer 实现 BridgeService gRPC 服务
type GRPCBridgeServer struct {
	pb.UnimplementedBridgeServiceServer
	nodeID        string
	manager       *BridgeManager
	activeBridges map[string]*BridgeContext
	bridgesMu     sync.RWMutex
	startTime     time.Time
}

// BridgeContext 桥接上下文（服务端侧）
type BridgeContext struct {
	streamID     string
	stream       pb.BridgeService_ForwardStreamServer
	recvChan     chan *pb.BridgePacket
	ctx          context.Context
	cancel       context.CancelFunc
	createdAt    time.Time
	lastActiveAt time.Time
	mu           sync.RWMutex
}

// NewGRPCBridgeServer 创建 gRPC 桥接服务器
func NewGRPCBridgeServer(nodeID string, manager *BridgeManager) *GRPCBridgeServer {
	return &GRPCBridgeServer{
		nodeID:        nodeID,
		manager:       manager,
		activeBridges: make(map[string]*BridgeContext),
		startTime:     time.Now(),
	}
}

// ForwardStream 实现双向流式转发
func (s *GRPCBridgeServer) ForwardStream(stream pb.BridgeService_ForwardStreamServer) error {
	ctx := stream.Context()
	utils.Infof("GRPCBridgeServer: new forward stream connection established")

	// 创建桥接上下文
	bridgeCtx, cancel := context.WithCancel(ctx)
	bridge := &BridgeContext{
		stream:       stream,
		recvChan:     make(chan *pb.BridgePacket, 100),
		ctx:          bridgeCtx,
		cancel:       cancel,
		createdAt:    time.Now(),
		lastActiveAt: time.Now(),
	}

	// 启动接收循环
	errChan := make(chan error, 2)
	go s.receiveLoop(bridge, errChan)
	go s.sendLoop(bridge, errChan)

	// 等待任一循环结束
	err := <-errChan
	cancel()

	if err != nil && err != io.EOF {
		utils.Errorf("GRPCBridgeServer: forward stream error: %v", err)
	}

	utils.Infof("GRPCBridgeServer: forward stream connection closed")
	return err
}

// receiveLoop 接收数据包循环
func (s *GRPCBridgeServer) receiveLoop(bridge *BridgeContext, errChan chan error) {
	for {
		packet, err := bridge.stream.Recv()
		if err != nil {
			if err == io.EOF {
				utils.Infof("GRPCBridgeServer: stream closed by client")
			} else {
				utils.Errorf("GRPCBridgeServer: failed to receive packet: %v", err)
			}
			errChan <- err
			return
		}

		bridge.mu.Lock()
		bridge.lastActiveAt = time.Now()
		bridge.mu.Unlock()

		utils.Debugf("GRPCBridgeServer: received packet (stream_id: %s, type: %v)", packet.StreamId, packet.Type)

		// 处理数据包
		switch packet.Type {
		case pb.PacketType_STREAM_OPEN:
			s.handleStreamOpen(bridge, packet)
		case pb.PacketType_STREAM_DATA:
			s.handleStreamData(bridge, packet)
		case pb.PacketType_STREAM_CLOSE:
			s.handleStreamClose(bridge, packet)
		default:
			utils.Warnf("GRPCBridgeServer: unknown packet type: %v", packet.Type)
		}
	}
}

// sendLoop 发送数据包循环
func (s *GRPCBridgeServer) sendLoop(bridge *BridgeContext, errChan chan error) {
	for {
		select {
		case packet := <-bridge.recvChan:
			if err := bridge.stream.Send(packet); err != nil {
				utils.Errorf("GRPCBridgeServer: failed to send packet: %v", err)
				errChan <- err
				return
			}

			bridge.mu.Lock()
			bridge.lastActiveAt = time.Now()
			bridge.mu.Unlock()

			utils.Debugf("GRPCBridgeServer: sent packet (stream_id: %s, type: %v)", packet.StreamId, packet.Type)

		case <-bridge.ctx.Done():
			errChan <- bridge.ctx.Err()
			return
		}
	}
}

// handleStreamOpen 处理打开流请求
func (s *GRPCBridgeServer) handleStreamOpen(bridge *BridgeContext, packet *pb.BridgePacket) {
	utils.Infof("GRPCBridgeServer: handling stream open for %s", packet.StreamId)

	// 存储桥接上下文
	bridge.streamID = packet.StreamId
	s.bridgesMu.Lock()
	s.activeBridges[packet.StreamId] = bridge
	s.bridgesMu.Unlock()

	// 这里应该：
	// 1. 根据 StreamOpenRequest 中的 target_client_id 查找目标客户端
	// 2. 建立到目标客户端的连接
	// 3. 开始双向转发

	// 目前先返回成功响应（实际实现需要与 SessionManager 集成）
	ackPacket := &pb.BridgePacket{
		StreamId:  packet.StreamId,
		Type:      pb.PacketType_STREAM_ACK,
		Timestamp: time.Now().UnixMilli(),
	}

	select {
	case bridge.recvChan <- ackPacket:
		utils.Infof("GRPCBridgeServer: sent ack for stream %s", packet.StreamId)
	case <-time.After(1 * time.Second):
		utils.Errorf("GRPCBridgeServer: timeout sending ack for stream %s", packet.StreamId)
	}
}

// handleStreamData 处理数据传输
func (s *GRPCBridgeServer) handleStreamData(bridge *BridgeContext, packet *pb.BridgePacket) {
	utils.Debugf("GRPCBridgeServer: handling stream data for %s (size: %d bytes)", packet.StreamId, len(packet.Payload))

	// 这里应该将数据转发到目标客户端
	// 目前只是记录日志（实际实现需要与 SessionManager 集成）

	// 此处应实现实际的数据转发逻辑（待与 BridgeManager 集成）
}

// handleStreamClose 处理关闭流请求
func (s *GRPCBridgeServer) handleStreamClose(bridge *BridgeContext, packet *pb.BridgePacket) {
	utils.Infof("GRPCBridgeServer: handling stream close for %s", packet.StreamId)

	s.bridgesMu.Lock()
	delete(s.activeBridges, packet.StreamId)
	s.bridgesMu.Unlock()

	// 这里应该清理相关的转发会话
	// 在此添加资源清理逻辑
}

// Ping 实现健康检查
func (s *GRPCBridgeServer) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	utils.Debugf("GRPCBridgeServer: received ping from node %s", req.NodeId)

	return &pb.PingResponse{
		Ok:              true,
		ServerTimestamp: time.Now().UnixMilli(),
	}, nil
}

// GetNodeInfo 实现获取节点信息
func (s *GRPCBridgeServer) GetNodeInfo(ctx context.Context, req *pb.NodeInfoRequest) (*pb.NodeInfoResponse, error) {
	utils.Debugf("GRPCBridgeServer: received node info request")

	s.bridgesMu.RLock()
	activeBridges := int32(len(s.activeBridges))
	s.bridgesMu.RUnlock()

	// 获取连接池统计
	var totalStreams int32
	if s.manager != nil {
		metrics := s.manager.GetConnectionPool().GetMetrics()
		totalStreams = metrics.GlobalStats.TotalActiveStreams
	}

	return &pb.NodeInfoResponse{
		NodeId:            s.nodeID,
		NodeAddress:       "", // 从配置中获取地址（尚未实现）
		ActiveConnections: activeBridges,
		ActiveStreams:     totalStreams,
		UptimeSeconds:     int64(time.Since(s.startTime).Seconds()),
		Metadata: &pb.NodeMetadata{
			Version:  "2.2.0",
			NodeType: "tunnox-server",
			Region:   "",
			Cluster:  "",
		},
	}, nil
}

// GetActiveStreamsCount 获取活跃流数量
func (s *GRPCBridgeServer) GetActiveStreamsCount() int {
	s.bridgesMu.RLock()
	defer s.bridgesMu.RUnlock()
	return len(s.activeBridges)
}
