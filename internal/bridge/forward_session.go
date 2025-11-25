package bridge

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
	pb "tunnox-core/api/proto/bridge"
	"tunnox-core/internal/utils"

	"github.com/google/uuid"
)

// SessionMetadata 会话元数据（强类型定义）
type SessionMetadata struct {
	SourceClientID int64  // 源客户端ID
	TargetClientID int64  // 目标客户端ID
	TargetHost     string // 目标主机
	TargetPort     int    // 目标端口
	SourceNodeID   string // 源节点ID
	TargetNodeID   string // 目标节点ID
	RequestID      string // 请求ID（用于关联请求/响应）
}

// ForwardSession 表示一个逻辑转发会话（在一个物理 gRPC 连接上多路复用）
type ForwardSession struct {
	streamID     string
	conn         MultiplexedConn
	sendChan     chan *pb.BridgePacket
	recvChan     chan *pb.BridgePacket
	ctx          context.Context
	cancel       context.CancelFunc
	closedOnce   sync.Once
	metadata     *SessionMetadata
	createdAt    time.Time
	lastActiveAt time.Time
	mu           sync.RWMutex
}

// NewForwardSession 创建新的转发会话
func NewForwardSession(parentCtx context.Context, conn MultiplexedConn, metadata *SessionMetadata) *ForwardSession {
	streamID := uuid.New().String()
	ctx, cancel := context.WithCancel(parentCtx)

	session := &ForwardSession{
		streamID:     streamID,
		conn:         conn,
		sendChan:     make(chan *pb.BridgePacket, 100),
		recvChan:     make(chan *pb.BridgePacket, 100),
		ctx:          ctx,
		cancel:       cancel,
		metadata:     metadata,
		createdAt:    time.Now(),
		lastActiveAt: time.Now(),
	}

	// 注册到多路复用连接
	if err := conn.RegisterSession(streamID, session); err != nil {
		utils.Errorf("ForwardSession: failed to register session %s: %v", streamID, err)
		cancel()
		return nil
	}

	utils.Infof("ForwardSession: created session %s", streamID)
	return session
}

// StreamID 获取流ID
func (s *ForwardSession) StreamID() string {
	return s.streamID
}

// toPacketMetadata 将 SessionMetadata 转换为 PacketMetadata
func (s *ForwardSession) toPacketMetadata() *pb.PacketMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.metadata == nil {
		return nil
	}

	return &pb.PacketMetadata{
		SourceNodeId:   s.metadata.SourceNodeID,
		TargetNodeId:   s.metadata.TargetNodeID,
		SourceClientId: s.metadata.SourceClientID,
		TargetClientId: s.metadata.TargetClientID,
		RequestId:      s.metadata.RequestID,
	}
}

// Send 发送数据包
func (s *ForwardSession) Send(data []byte) error {
	s.mu.Lock()
	s.lastActiveAt = time.Now()
	s.mu.Unlock()

	packet := &pb.BridgePacket{
		StreamId:  s.streamID,
		Type:      pb.PacketType_STREAM_DATA,
		Payload:   data,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  s.toPacketMetadata(),
	}

	select {
	case s.sendChan <- packet:
		return nil
	case <-s.ctx.Done():
		return fmt.Errorf("session closed")
	case <-time.After(5 * time.Second):
		return fmt.Errorf("send timeout")
	}
}

// Receive 接收数据包
func (s *ForwardSession) Receive() ([]byte, error) {
	select {
	case packet := <-s.recvChan:
		if packet == nil {
			return nil, io.EOF
		}
		s.mu.Lock()
		s.lastActiveAt = time.Now()
		s.mu.Unlock()
		return packet.Payload, nil
	case <-s.ctx.Done():
		return nil, fmt.Errorf("session closed")
	}
}

// SendPacket 发送特定类型的数据包
func (s *ForwardSession) SendPacket(packetType pb.PacketType, payload []byte) error {
	s.mu.Lock()
	s.lastActiveAt = time.Now()
	s.mu.Unlock()

	packet := &pb.BridgePacket{
		StreamId:  s.streamID,
		Type:      packetType,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  s.toPacketMetadata(),
	}

	select {
	case s.sendChan <- packet:
		return nil
	case <-s.ctx.Done():
		return fmt.Errorf("session closed")
	case <-time.After(5 * time.Second):
		return fmt.Errorf("send packet timeout")
	}
}

// ReceivePacket 接收数据包（带类型）
func (s *ForwardSession) ReceivePacket() (*pb.BridgePacket, error) {
	select {
	case packet := <-s.recvChan:
		if packet == nil {
			return nil, io.EOF
		}
		s.mu.Lock()
		s.lastActiveAt = time.Now()
		s.mu.Unlock()
		return packet, nil
	case <-s.ctx.Done():
		return nil, fmt.Errorf("session closed")
	}
}

// deliverPacket 投递接收到的数据包（由 MultiplexedConn 调用）
func (s *ForwardSession) deliverPacket(packet *pb.BridgePacket) error {
	select {
	case s.recvChan <- packet:
		return nil
	case <-s.ctx.Done():
		return fmt.Errorf("session closed")
	default:
		// 接收通道满，丢弃数据包
		utils.Warnf("ForwardSession: receive channel full for session %s, dropping packet", s.streamID)
		return fmt.Errorf("receive channel full")
	}
}

// getSendChannel 获取发送通道（供 MultiplexedConn 使用）
func (s *ForwardSession) getSendChannel() <-chan *pb.BridgePacket {
	return s.sendChan
}

// Close 关闭会话
func (s *ForwardSession) Close() error {
	var err error
	s.closedOnce.Do(func() {
		// 发送关闭数据包
		closePacket := &pb.BridgePacket{
			StreamId:  s.streamID,
			Type:      pb.PacketType_STREAM_CLOSE,
			Timestamp: time.Now().UnixMilli(),
		}

		select {
		case s.sendChan <- closePacket:
			// Close packet sent
		case <-time.After(1 * time.Second):
			utils.Warnf("ForwardSession: timeout sending close packet for session %s", s.streamID)
		}

		// 取消上下文
		s.cancel()

		// 从连接中注销
		if s.conn != nil {
			s.conn.UnregisterSession(s.streamID)
		}

		// 关闭通道
		close(s.sendChan)
		close(s.recvChan)

		utils.Infof("ForwardSession: closed session %s", s.streamID)
	})
	return err
}

// IsActive 检查会话是否活跃
func (s *ForwardSession) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.lastActiveAt) < 5*time.Minute
}

// GetMetadata 获取元数据
func (s *ForwardSession) GetMetadata() *SessionMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 返回副本避免并发修改
	if s.metadata == nil {
		return nil
	}
	metadata := &SessionMetadata{
		SourceClientID: s.metadata.SourceClientID,
		TargetClientID: s.metadata.TargetClientID,
		TargetHost:     s.metadata.TargetHost,
		TargetPort:     s.metadata.TargetPort,
		SourceNodeID:   s.metadata.SourceNodeID,
		TargetNodeID:   s.metadata.TargetNodeID,
		RequestID:      s.metadata.RequestID,
	}
	return metadata
}

// UpdateLastActive 更新最后活跃时间
func (s *ForwardSession) UpdateLastActive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActiveAt = time.Now()
}
