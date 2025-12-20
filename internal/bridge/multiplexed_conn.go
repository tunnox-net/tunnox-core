package bridge

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
	pb "tunnox-core/api/proto/bridge"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"

	"google.golang.org/grpc"
)

// grpcMultiplexedConn 多路复用的 gRPC 连接实现
type grpcMultiplexedConn struct {
	*dispose.ResourceBase
	targetNodeID  string
	grpcConn      *grpc.ClientConn
	stream        pb.BridgeService_ForwardStreamClient
	sessions      map[string]*ForwardSession
	sessionsMu    sync.RWMutex
	createdAt     time.Time
	lastActiveAt  time.Time
	activeStreams int32
	maxStreams    int32
	closed        bool
	closedMu      sync.RWMutex
}

// NewMultiplexedConn 创建新的多路复用连接
func NewMultiplexedConn(parentCtx context.Context, targetNodeID string, grpcConn *grpc.ClientConn, maxStreams int32) (MultiplexedConn, error) {
	client := pb.NewBridgeServiceClient(grpcConn)

	stream, err := client.ForwardStream(parentCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create forward stream: %w", err)
	}

	mc := &grpcMultiplexedConn{
		ResourceBase:  dispose.NewResourceBase(fmt.Sprintf("MultiplexedConn-%s", targetNodeID)),
		targetNodeID:  targetNodeID,
		grpcConn:      grpcConn,
		stream:        stream,
		sessions:      make(map[string]*ForwardSession),
		createdAt:     time.Now(),
		lastActiveAt:  time.Now(),
		activeStreams: 0,
		maxStreams:    maxStreams,
		closed:        false,
	}

	// 初始化资源
	mc.Initialize(parentCtx)

	// 启动接收和发送循环
	go mc.receiveLoop()

	corelog.Infof("MultiplexedConn: created connection to node %s (max_streams: %d)", targetNodeID, maxStreams)
	return mc, nil
}

// RegisterSession 注册会话
func (m *grpcMultiplexedConn) RegisterSession(streamID string, session *ForwardSession) error {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	if m.closed {
		return fmt.Errorf("connection is closed")
	}

	if int32(len(m.sessions)) >= m.maxStreams {
		return fmt.Errorf("max streams reached: %d", m.maxStreams)
	}

	if _, exists := m.sessions[streamID]; exists {
		return fmt.Errorf("session already exists: %s", streamID)
	}

	m.sessions[streamID] = session
	m.activeStreams = int32(len(m.sessions))
	m.lastActiveAt = time.Now()

	// 启动发送协程（为每个会话）
	go m.sendLoopForSession(session)

	corelog.Debugf("MultiplexedConn: registered session %s (active: %d/%d)", streamID, m.activeStreams, m.maxStreams)
	return nil
}

// UnregisterSession 注销会话
func (m *grpcMultiplexedConn) UnregisterSession(streamID string) {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	if _, exists := m.sessions[streamID]; exists {
		delete(m.sessions, streamID)
		m.activeStreams = int32(len(m.sessions))
		corelog.Debugf("MultiplexedConn: unregistered session %s (active: %d/%d)", streamID, m.activeStreams, m.maxStreams)
	}
}

// sendLoopForSession 为指定会话发送数据包
func (m *grpcMultiplexedConn) sendLoopForSession(session *ForwardSession) {
	sendChan := session.getSendChannel()

	for {
		select {
		case packet, ok := <-sendChan:
			if !ok {
				// 通道已关闭
				return
			}

			m.closedMu.RLock()
			if m.closed {
				m.closedMu.RUnlock()
				return
			}
			m.closedMu.RUnlock()

			// 发送到 gRPC 流
			if err := m.stream.Send(packet); err != nil {
				corelog.Errorf("MultiplexedConn: failed to send packet for stream %s: %v", session.streamID, err)
				session.Close()
				return
			}

			m.sessionsMu.Lock()
			m.lastActiveAt = time.Now()
			m.sessionsMu.Unlock()

			corelog.Debugf("MultiplexedConn: sent packet for stream %s (type: %v)", session.streamID, packet.Type)

		case <-session.Ctx().Done():
			return
		case <-m.Ctx().Done():
			return
		}
	}
}

// receiveLoop 接收数据包并分发到对应的会话
func (m *grpcMultiplexedConn) receiveLoop() {
	corelog.Infof("MultiplexedConn: receive loop started for node %s", m.targetNodeID)

	for {
		select {
		case <-m.Ctx().Done():
			corelog.Infof("MultiplexedConn: receive loop stopped for node %s", m.targetNodeID)
			return
		default:
			packet, err := m.stream.Recv()
			if err != nil {
				if err == io.EOF {
					corelog.Infof("MultiplexedConn: stream closed for node %s", m.targetNodeID)
				} else {
					corelog.Errorf("MultiplexedConn: failed to receive packet: %v", err)
				}
				m.Close()
				return
			}

			m.sessionsMu.RLock()
			m.lastActiveAt = time.Now()
			session, exists := m.sessions[packet.StreamId]
			m.sessionsMu.RUnlock()

			if !exists {
				corelog.Warnf("MultiplexedConn: received packet for unknown stream %s", packet.StreamId)
				continue
			}

			// 投递到对应的会话
			if err := session.deliverPacket(packet); err != nil {
				corelog.Warnf("MultiplexedConn: failed to deliver packet to session %s: %v", packet.StreamId, err)
			}

			corelog.Debugf("MultiplexedConn: delivered packet to stream %s (type: %v)", packet.StreamId, packet.Type)
		}
	}
}

// CanAcceptStream 检查是否可以接受新流
func (m *grpcMultiplexedConn) CanAcceptStream() bool {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()
	return !m.closed && int32(len(m.sessions)) < m.maxStreams
}

// GetActiveStreams 获取活跃流数量
func (m *grpcMultiplexedConn) GetActiveStreams() int32 {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()
	return int32(len(m.sessions))
}

// IsIdle 检查连接是否空闲
func (m *grpcMultiplexedConn) IsIdle(maxIdleTime time.Duration) bool {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	// 如果还有活跃会话，不算空闲
	if len(m.sessions) > 0 {
		return false
	}

	// 检查最后活跃时间
	return time.Since(m.lastActiveAt) > maxIdleTime
}

// GetTargetNodeID 获取目标节点ID
func (m *grpcMultiplexedConn) GetTargetNodeID() string {
	return m.targetNodeID
}

// UpdateLastActive 更新最后活跃时间
func (m *grpcMultiplexedConn) UpdateLastActive() {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()
	m.lastActiveAt = time.Now()
}

// GetLastActiveTime 获取最后活跃时间
func (m *grpcMultiplexedConn) GetLastActiveTime() time.Time {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()
	return m.lastActiveAt
}

// Close 关闭连接
func (m *grpcMultiplexedConn) Close() error {
	m.closedMu.Lock()
	if m.closed {
		m.closedMu.Unlock()
		return nil
	}
	m.closed = true
	m.closedMu.Unlock()

	corelog.Infof("MultiplexedConn: closing connection to node %s", m.targetNodeID)

	// 关闭所有会话
	m.sessionsMu.Lock()
	sessions := make([]*ForwardSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessionsMu.Unlock()

	for _, session := range sessions {
		session.Close()
	}

	// 关闭 gRPC 流
	if m.stream != nil {
		if err := m.stream.CloseSend(); err != nil {
			corelog.Warnf("MultiplexedConn: failed to close stream: %v", err)
		}
	}

	// 关闭 gRPC 连接
	if m.grpcConn != nil {
		if err := m.grpcConn.Close(); err != nil {
			corelog.Warnf("MultiplexedConn: failed to close grpc connection: %v", err)
		}
	}

	corelog.Infof("MultiplexedConn: closed connection to node %s", m.targetNodeID)

	// 调用基类 Close
	return m.ResourceBase.Close()
}

// IsClosed 检查连接是否已关闭
func (m *grpcMultiplexedConn) IsClosed() bool {
	m.closedMu.RLock()
	defer m.closedMu.RUnlock()
	return m.closed
}
