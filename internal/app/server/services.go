package server

import (
	"context"
	"fmt"
	"sync"
	"tunnox-core/internal/api"
	"tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/server/udp"
	"tunnox-core/internal/utils"
)

// ============================================================================
// 通用服务适配器（消除重复代码）
// ============================================================================

// Closeable 定义可关闭的资源接口
type Closeable interface {
	Close() error
}

// BaseService 通用服务适配器
// 用于包装各种资源（CloudControl, Storage, Broker, Bridge 等）为统一的服务接口
type BaseService struct {
	name      string
	closeable Closeable                       // 可选：需要关闭的资源
	onStart   func(ctx context.Context) error // 可选：自定义启动逻辑
	onStop    func(ctx context.Context) error // 可选：自定义停止逻辑
}

// NewBaseService 创建通用服务
// name: 服务名称
// closeable: 可关闭的资源（可为 nil）
func NewBaseService(name string, closeable Closeable) *BaseService {
	return &BaseService{
		name:      name,
		closeable: closeable,
	}
}

// WithOnStart 设置自定义启动逻辑（链式调用）
func (s *BaseService) WithOnStart(fn func(ctx context.Context) error) *BaseService {
	s.onStart = fn
	return s
}

// WithOnStop 设置自定义停止逻辑（链式调用）
func (s *BaseService) WithOnStop(fn func(ctx context.Context) error) *BaseService {
	s.onStop = fn
	return s
}

func (s *BaseService) Name() string {
	return s.name
}

func (s *BaseService) Start(ctx context.Context) error {
	// 执行自定义启动逻辑
	if s.onStart != nil {
		return s.onStart(ctx)
	}

	// 默认：仅记录日志
	utils.Infof("Starting service: %s", s.name)
	return nil
}

func (s *BaseService) Stop(ctx context.Context) error {
	utils.Infof("Stopping service: %s", s.name)

	// 执行自定义停止逻辑
	if s.onStop != nil {
		if err := s.onStop(ctx); err != nil {
			return err
		}
	}

	// 关闭资源
	if s.closeable != nil {
		if err := s.closeable.Close(); err != nil {
			return fmt.Errorf("failed to close %s: %w", s.name, err)
		}
	}

	return nil
}

// ============================================================================
// 便捷构造函数（提供类型安全的创建方式）
// ============================================================================

// NewCloudService 创建云控制服务
func NewCloudService(name string, cloudControl managers.CloudControlAPI) *BaseService {
	return NewBaseService(name, cloudControl).WithOnStart(func(ctx context.Context) error {
		utils.Debugf("Starting cloud service: %s", name)
		return nil
	})
}

// NewStorageService 创建存储服务
// 注意：Storage 通常不需要 Close，所以传 nil
func NewStorageService(name string, storage storage.Storage) *BaseService {
	_ = storage // 保留参数以备将来使用
	return NewBaseService(name, nil).WithOnStart(func(ctx context.Context) error {
		utils.Infof("Starting storage service: %s", name)
		return nil
	})
}

// NewBrokerService 创建消息代理服务
func NewBrokerService(name string, broker broker.MessageBroker) *BaseService {
	return NewBaseService(name, broker)
}

// NewBridgeService 创建桥接服务
func NewBridgeService(name string, manager *bridge.BridgeManager) *BaseService {
	return NewBaseService(name, manager)
}

// NewManagementAPIService 创建 Management API 服务
func NewManagementAPIService(name string, apiServer *api.ManagementAPIServer) *BaseService {
	return NewBaseService(name, apiServer).WithOnStart(func(ctx context.Context) error {
		return apiServer.Start()
	})
}

// NewUDPIngressService 创建 UDP Ingress 服务
func NewUDPIngressService(name string, mgr *udp.Manager) *BaseService {
	return NewBaseService(name, mgr).WithOnStart(func(ctx context.Context) error {
		return mgr.Start(ctx)
	})
}

// ProtocolFactory 协议工厂
type ProtocolFactory struct {
	session *session.SessionManager
}

// NewProtocolFactory 创建协议工厂
func NewProtocolFactory(session *session.SessionManager) *ProtocolFactory {
	return &ProtocolFactory{
		session: session,
	}
}

// CreateAdapter 创建协议适配器
func (pf *ProtocolFactory) CreateAdapter(protocolName string, ctx context.Context) (adapter.Adapter, error) {
	switch protocolName {
	case "tcp":
		return adapter.NewTcpAdapter(ctx, pf.session), nil
	case "websocket":
		return adapter.NewWebSocketAdapter(ctx, pf.session), nil
	case "udp":
		return adapter.NewUdpAdapter(ctx, pf.session), nil
	case "quic":
		return adapter.NewQuicAdapter(ctx, pf.session), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocolName)
	}
}

// SimpleNodeRegistry 简单的节点注册表实现
type SimpleNodeRegistry struct {
	nodes map[string]string
	mu    sync.RWMutex
}

// NewSimpleNodeRegistry 创建简单节点注册表
func NewSimpleNodeRegistry() *SimpleNodeRegistry {
	return &SimpleNodeRegistry{
		nodes: make(map[string]string),
	}
}

// GetNodeAddress 获取节点地址
func (r *SimpleNodeRegistry) GetNodeAddress(nodeID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	addr, exists := r.nodes[nodeID]
	if !exists {
		return "", fmt.Errorf("node not found: %s", nodeID)
	}
	return addr, nil
}

// ListAllNodes 列出所有节点
func (r *SimpleNodeRegistry) ListAllNodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]string, 0, len(r.nodes))
	for nodeID := range r.nodes {
		nodes = append(nodes, nodeID)
	}
	return nodes
}

// RegisterNode 注册节点
func (r *SimpleNodeRegistry) RegisterNode(nodeID, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[nodeID] = addr
	utils.Infof("SimpleNodeRegistry: registered node %s at %s", nodeID, addr)
}

// UnregisterNode 注销节点
func (r *SimpleNodeRegistry) UnregisterNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeID)
	utils.Infof("SimpleNodeRegistry: unregistered node %s", nodeID)
}
