package server

import (
	"context"
	"fmt"
	"sync"
	"tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/managers"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/protocol/session"
)

// ============================================================================
// SessionManager 适配器（适配 session.SessionManager 到 httpservice.SessionManagerInterface）
// ============================================================================

// SessionManagerAdapter 会话管理器适配器
type SessionManagerAdapter struct {
	sessionMgr *session.SessionManager
}

// NewSessionManagerAdapter 创建会话管理器适配器
func NewSessionManagerAdapter(sessionMgr *session.SessionManager) *SessionManagerAdapter {
	return &SessionManagerAdapter{sessionMgr: sessionMgr}
}

// GetControlConnectionInterface 获取控制连接
func (a *SessionManagerAdapter) GetControlConnectionInterface(clientID int64) httpservice.ControlConnectionAccessor {
	conn := a.sessionMgr.GetControlConnectionInterface(clientID)
	if conn == nil {
		return nil
	}
	return &ControlConnectionAdapter{conn: conn}
}

// BroadcastConfigPush 广播配置推送
func (a *SessionManagerAdapter) BroadcastConfigPush(clientID int64, configBody string) error {
	return a.sessionMgr.BroadcastConfigPush(clientID, configBody)
}

// GetNodeID 获取当前节点ID
func (a *SessionManagerAdapter) GetNodeID() string {
	return a.sessionMgr.GetNodeID()
}

// SendHTTPProxyRequest 发送 HTTP 代理请求
func (a *SessionManagerAdapter) SendHTTPProxyRequest(clientID int64, request *httpservice.HTTPProxyRequest) (*httpservice.HTTPProxyResponse, error) {
	return a.sessionMgr.SendHTTPProxyRequest(clientID, request)
}

// RequestTunnelForHTTP 请求为 HTTP 代理创建隧道连接
func (a *SessionManagerAdapter) RequestTunnelForHTTP(clientID int64, mappingID string, targetURL string, method string) (httpservice.TunnelConnectionInterface, error) {
	tunnelConn, err := a.sessionMgr.RequestTunnelForHTTP(clientID, mappingID, targetURL, method)
	if err != nil {
		return nil, err
	}
	return &TunnelConnectionAdapter{conn: tunnelConn}, nil
}

// NotifyClientUpdate 通知客户端更新配置
func (a *SessionManagerAdapter) NotifyClientUpdate(clientID int64) {
	a.sessionMgr.NotifyClientUpdate(clientID)
}

// ControlConnectionAdapter 控制连接适配器
type ControlConnectionAdapter struct {
	conn session.ControlConnectionInterface
}

// GetConnID 获取连接ID
func (a *ControlConnectionAdapter) GetConnID() string {
	return a.conn.GetConnID()
}

// GetRemoteAddr 获取远程地址
func (a *ControlConnectionAdapter) GetRemoteAddr() string {
	addr := a.conn.GetRemoteAddr()
	if addr == nil {
		return ""
	}
	return addr.String()
}

// TunnelConnectionAdapter 隧道连接适配器
type TunnelConnectionAdapter struct {
	conn session.TunnelConnectionInterface
}

// GetNetConn 获取底层网络连接
func (a *TunnelConnectionAdapter) GetNetConn() interface{} {
	return a.conn.GetNetConn()
}

// GetStream 获取数据流
func (a *TunnelConnectionAdapter) GetStream() interface{} {
	return a.conn.GetStream()
}

// Read 读取数据
func (a *TunnelConnectionAdapter) Read(p []byte) (int, error) {
	stream := a.conn.GetStream()
	if stream == nil {
		return 0, fmt.Errorf("stream is nil")
	}
	// 使用 stream 的 Read 方法
	reader := stream.GetReader()
	if reader == nil {
		return 0, fmt.Errorf("reader is nil")
	}
	return reader.Read(p)
}

// Write 写入数据
func (a *TunnelConnectionAdapter) Write(p []byte) (int, error) {
	stream := a.conn.GetStream()
	if stream == nil {
		return 0, fmt.Errorf("stream is nil")
	}
	// 使用 stream 的 Write 方法
	writer := stream.GetWriter()
	if writer == nil {
		return 0, fmt.Errorf("writer is nil")
	}
	return writer.Write(p)
}

// Close 关闭连接
func (a *TunnelConnectionAdapter) Close() error {
	return a.conn.Close()
}

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

	// 默认：仅记录日志（写入文件）
	corelog.Infof("Starting service: %s", s.name)
	return nil
}

func (s *BaseService) Stop(ctx context.Context) error {
	corelog.Infof("Stopping service: %s", s.name)

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
		return nil
	})
}

// NewStorageService 创建存储服务
// 注意：Storage 通常不需要 Close，所以传 nil
func NewStorageService(name string, storage storage.Storage) *BaseService {
	_ = storage // 保留参数以备将来使用
	return NewBaseService(name, nil).WithOnStart(func(ctx context.Context) error {
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

// NewHTTPServiceAdapter 创建 HTTP 服务适配器
func NewHTTPServiceAdapter(name string, httpService *httpservice.HTTPService) *BaseService {
	return NewBaseService(name, httpService).WithOnStart(func(ctx context.Context) error {
		return httpService.Start()
	}).WithOnStop(func(ctx context.Context) error {
		return httpService.Stop()
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
// 注意：websocket 和 httppoll 不需要独立适配器，它们通过 HTTP 服务容器提供
func (pf *ProtocolFactory) CreateAdapter(protocolName string, ctx context.Context) (adapter.Adapter, error) {
	switch protocolName {
	case "tcp":
		return adapter.NewTcpAdapter(ctx, pf.session), nil
	case "quic":
		return adapter.NewQuicAdapter(ctx, pf.session), nil
	case "kcp":
		return adapter.NewKcpAdapter(ctx, pf.session), nil
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
}

// UnregisterNode 注销节点
func (r *SimpleNodeRegistry) UnregisterNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeID)
}
