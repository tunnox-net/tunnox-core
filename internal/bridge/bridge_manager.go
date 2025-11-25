package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	pb "tunnox-core/api/proto/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// BridgeManager 桥接管理器（整合连接池和消息代理）
type BridgeManager struct {
	*dispose.ManagerBase
	nodeID          string
	connectionPool  *BridgeConnectionPool
	messageBroker   broker.MessageBroker
	nodeRegistry    NodeRegistry // 节点注册表（用于查找节点地址）
	pendingRequests map[string]*BridgeRequest
	requestsMu      sync.RWMutex
}

// NodeRegistry 节点注册表接口
type NodeRegistry interface {
	// GetNodeAddress 获取节点地址
	GetNodeAddress(nodeID string) (string, error)

	// ListAllNodes 列出所有节点
	ListAllNodes() []string
}

// BridgeRequest 桥接请求
type BridgeRequest struct {
	RequestID      string
	SourceNodeID   string
	TargetNodeID   string
	SourceClientID int64
	TargetClientID int64
	TargetHost     string
	TargetPort     int
	Session        *ForwardSession
	ResponseChan   chan *BridgeResponse
	CreatedAt      time.Time
	Timeout        time.Duration
}

// BridgeResponse 桥接响应
type BridgeResponse struct {
	RequestID string
	Success   bool
	Error     string
	StreamID  string
	Session   *ForwardSession
}

// BridgeManagerConfig 桥接管理器配置
type BridgeManagerConfig struct {
	NodeID         string
	PoolConfig     *PoolConfig
	MessageBroker  broker.MessageBroker
	NodeRegistry   NodeRegistry
	RequestTimeout time.Duration
}

// NewBridgeManager 创建桥接管理器
func NewBridgeManager(ctx context.Context, config *BridgeManagerConfig) (*BridgeManager, error) {
	if config == nil {
		return nil, fmt.Errorf("bridge manager config is required")
	}

	if config.MessageBroker == nil {
		return nil, fmt.Errorf("message broker is required")
	}

	if config.NodeRegistry == nil {
		return nil, fmt.Errorf("node registry is required")
	}

	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}

	manager := &BridgeManager{
		ManagerBase:     dispose.NewManager("BridgeManager", ctx),
		nodeID:          config.NodeID,
		connectionPool:  NewBridgeConnectionPool(ctx, config.PoolConfig),
		messageBroker:   config.MessageBroker,
		nodeRegistry:    config.NodeRegistry,
		pendingRequests: make(map[string]*BridgeRequest),
	}

	// 订阅桥接请求和响应主题
	if err := manager.subscribeToTopics(); err != nil {
		return nil, fmt.Errorf("failed to subscribe to topics: %w", err)
	}

	utils.Infof("BridgeManager: initialized for node %s", config.NodeID)
	return manager, nil
}

// subscribeToTopics 订阅相关主题
func (m *BridgeManager) subscribeToTopics() error {
	// 订阅桥接请求
	requestChan, err := m.messageBroker.Subscribe(m.Ctx(), broker.TopicBridgeRequest)
	if err != nil {
		return fmt.Errorf("failed to subscribe to bridge requests: %w", err)
	}

	// 订阅桥接响应
	responseChan, err := m.messageBroker.Subscribe(m.Ctx(), broker.TopicBridgeResponse)
	if err != nil {
		return fmt.Errorf("failed to subscribe to bridge responses: %w", err)
	}

	// 启动处理循环
	go m.handleBridgeRequests(requestChan)
	go m.handleBridgeResponses(responseChan)

	return nil
}

// handleBridgeRequests 处理桥接请求
func (m *BridgeManager) handleBridgeRequests(requestChan <-chan *broker.Message) {
	utils.Infof("BridgeManager: started bridge request handler")

	for {
		select {
		case msg := <-requestChan:
			if msg == nil {
				return
			}

			var req broker.BridgeRequestMessage
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				utils.Errorf("BridgeManager: failed to unmarshal bridge request: %v", err)
				continue
			}

			// 只处理发给本节点的请求
			if req.TargetNodeID != m.nodeID {
				continue
			}

			utils.Infof("BridgeManager: received bridge request %s from node %s", req.RequestID, req.SourceNodeID)
			go m.processBridgeRequest(&req)

		case <-m.Ctx().Done():
			utils.Infof("BridgeManager: request handling loop stopped")
			return
		}
	}
}

// processBridgeRequest 处理桥接请求
func (m *BridgeManager) processBridgeRequest(req *broker.BridgeRequestMessage) {
	// 创建转发会话元数据（强类型）
	metadata := &SessionMetadata{
		SourceClientID: req.SourceClientID,
		TargetClientID: req.TargetClientID,
		TargetHost:     req.TargetHost,
		TargetPort:     req.TargetPort,
		SourceNodeID:   req.SourceNodeID,
		TargetNodeID:   req.TargetNodeID,
		RequestID:      req.RequestID,
	}

	// 获取源节点地址
	sourceNodeAddr, err := m.nodeRegistry.GetNodeAddress(req.SourceNodeID)
	if err != nil {
		m.sendBridgeResponse(req.RequestID, false, fmt.Sprintf("failed to get source node address: %v", err), "")
		return
	}

	// 创建到源节点的会话
	session, err := m.connectionPool.CreateSession(m.Ctx(), req.SourceNodeID, sourceNodeAddr, metadata)
	if err != nil {
		m.sendBridgeResponse(req.RequestID, false, fmt.Sprintf("failed to create session: %v", err), "")
		return
	}

	// 发送打开流请求
	openReq := &pb.StreamOpenRequest{
		SourceClientId: fmt.Sprintf("%d", req.SourceClientID),
		TargetClientId: fmt.Sprintf("%d", req.TargetClientID),
		TargetHost:     req.TargetHost,
		TargetPort:     int32(req.TargetPort),
		Protocol:       "tcp",
	}

	openReqData, err := json.Marshal(openReq)
	if err != nil {
		session.Close()
		m.sendBridgeResponse(req.RequestID, false, fmt.Sprintf("failed to marshal open request: %v", err), "")
		return
	}

	if err := session.SendPacket(pb.PacketType_STREAM_OPEN, openReqData); err != nil {
		session.Close()
		m.sendBridgeResponse(req.RequestID, false, fmt.Sprintf("failed to send open request: %v", err), "")
		return
	}

	// 等待响应
	packet, err := session.ReceivePacket()
	if err != nil {
		session.Close()
		m.sendBridgeResponse(req.RequestID, false, fmt.Sprintf("failed to receive open response: %v", err), "")
		return
	}

	if packet.Type != pb.PacketType_STREAM_ACK {
		session.Close()
		m.sendBridgeResponse(req.RequestID, false, "unexpected response type", "")
		return
	}

	// 发送成功响应
	m.sendBridgeResponse(req.RequestID, true, "", session.StreamID())
	utils.Infof("BridgeManager: successfully processed bridge request %s (stream: %s)", req.RequestID, session.StreamID())
}

// sendBridgeResponse 发送桥接响应
func (m *BridgeManager) sendBridgeResponse(requestID string, success bool, errorMsg, streamID string) {
	resp := broker.BridgeResponseMessage{
		RequestID: requestID,
		Success:   success,
		Error:     errorMsg,
		StreamID:  streamID,
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		utils.Errorf("BridgeManager: failed to marshal bridge response: %v", err)
		return
	}

	if err := m.messageBroker.Publish(m.Ctx(), broker.TopicBridgeResponse, respData); err != nil {
		utils.Errorf("BridgeManager: failed to publish bridge response: %v", err)
	}
}

// handleBridgeResponses 处理桥接响应
func (m *BridgeManager) handleBridgeResponses(responseChan <-chan *broker.Message) {
	utils.Infof("BridgeManager: started bridge response handler")

	for {
		select {
		case msg := <-responseChan:
			if msg == nil {
				return
			}

			var resp broker.BridgeResponseMessage
			if err := json.Unmarshal(msg.Payload, &resp); err != nil {
				utils.Errorf("BridgeManager: failed to unmarshal bridge response: %v", err)
				continue
			}

			// 查找挂起的请求
			m.requestsMu.Lock()
			req, exists := m.pendingRequests[resp.RequestID]
			if exists {
				delete(m.pendingRequests, resp.RequestID)
			}
			m.requestsMu.Unlock()

			if exists {
				// 发送响应到等待的通道
				bridgeResp := &BridgeResponse{
					RequestID: resp.RequestID,
					Success:   resp.Success,
					Error:     resp.Error,
					StreamID:  resp.StreamID,
					Session:   req.Session,
				}

				select {
				case req.ResponseChan <- bridgeResp:
					utils.Debugf("BridgeManager: delivered bridge response for request %s", resp.RequestID)
				case <-time.After(1 * time.Second):
					utils.Warnf("BridgeManager: timeout delivering bridge response for request %s", resp.RequestID)
				}
			}

		case <-m.Ctx().Done():
			utils.Infof("BridgeManager: response handling loop stopped")
			return
		}
	}
}

// RequestBridge 请求桥接到目标节点
func (m *BridgeManager) RequestBridge(ctx context.Context, targetNodeID string, sourceClientID, targetClientID int64, targetHost string, targetPort int) (*BridgeResponse, error) {
	requestID := fmt.Sprintf("br-%d-%s", time.Now().UnixNano(), m.nodeID)

	request := &BridgeRequest{
		RequestID:      requestID,
		SourceNodeID:   m.nodeID,
		TargetNodeID:   targetNodeID,
		SourceClientID: sourceClientID,
		TargetClientID: targetClientID,
		TargetHost:     targetHost,
		TargetPort:     targetPort,
		ResponseChan:   make(chan *BridgeResponse, 1),
		CreatedAt:      time.Now(),
		Timeout:        30 * time.Second,
	}

	// 添加到挂起请求
	m.requestsMu.Lock()
	m.pendingRequests[requestID] = request
	m.requestsMu.Unlock()

	// 发布桥接请求消息
	reqMsg := broker.BridgeRequestMessage{
		RequestID:      requestID,
		SourceNodeID:   m.nodeID,
		TargetNodeID:   targetNodeID,
		SourceClientID: sourceClientID,
		TargetClientID: targetClientID,
		TargetHost:     targetHost,
		TargetPort:     targetPort,
	}

	reqData, err := json.Marshal(reqMsg)
	if err != nil {
		m.requestsMu.Lock()
		delete(m.pendingRequests, requestID)
		m.requestsMu.Unlock()
		return nil, fmt.Errorf("failed to marshal bridge request: %w", err)
	}

	if err := m.messageBroker.Publish(ctx, broker.TopicBridgeRequest, reqData); err != nil {
		m.requestsMu.Lock()
		delete(m.pendingRequests, requestID)
		m.requestsMu.Unlock()
		return nil, fmt.Errorf("failed to publish bridge request: %w", err)
	}

	utils.Infof("BridgeManager: published bridge request %s to node %s", requestID, targetNodeID)

	// 等待响应
	select {
	case resp := <-request.ResponseChan:
		return resp, nil
	case <-time.After(request.Timeout):
		m.requestsMu.Lock()
		delete(m.pendingRequests, requestID)
		m.requestsMu.Unlock()
		return nil, fmt.Errorf("bridge request timeout")
	case <-ctx.Done():
		m.requestsMu.Lock()
		delete(m.pendingRequests, requestID)
		m.requestsMu.Unlock()
		return nil, ctx.Err()
	}
}

// GetConnectionPool 获取连接池
func (m *BridgeManager) GetConnectionPool() *BridgeConnectionPool {
	return m.connectionPool
}

// GetNodeID 获取当前节点ID
func (m *BridgeManager) GetNodeID() string {
	return m.nodeID
}

// Close 关闭桥接管理器
func (m *BridgeManager) Close() error {
	// 关闭连接池
	if err := m.connectionPool.Close(); err != nil {
		utils.Errorf("BridgeManager: failed to close connection pool: %v", err)
	}

	// 清理挂起的请求
	m.requestsMu.Lock()
	for requestID, req := range m.pendingRequests {
		close(req.ResponseChan)
		delete(m.pendingRequests, requestID)
	}
	m.requestsMu.Unlock()

	utils.Infof("BridgeManager: closed for node %s", m.nodeID)

	// 调用基类 Close
	return m.ManagerBase.Close()
}
