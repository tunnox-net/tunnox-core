// Package session 提供会话管理功能
package session

import (
	"context"
	"net"
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// CrossNodeTunnelConn 跨节点隧道专用连接
// 绑定客户端连接和跨节点连接的生命周期
type CrossNodeTunnelConn struct {
	TunnelID   string
	RemoteAddr string // 客户端地址 (ip:port)
	Target     string // 目标地址 (host:port)
	NodeID     string // 目标节点 ID

	CrossConn *net.TCPConn // 跨节点专用连接

	ctx          context.Context
	cancel       context.CancelFunc
	lastActivity time.Time
	closeOnce    sync.Once
	closed       bool
	mu           sync.Mutex
}

// TunnelConnectionManager 隧道连接管理器
// 管理跨节点隧道的专用连接，不使用连接池
// 每个隧道使用专用连接，生命周期与隧道绑定
type TunnelConnectionManager struct {
	connections sync.Map // key: tunnelID, value: *CrossNodeTunnelConn

	// 获取节点地址的函数
	getNodeAddr func(nodeID string) (string, error)

	// 配置
	dialTimeout time.Duration
	idleTimeout time.Duration

	// 后台清理
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
}

// TunnelConnectionManagerConfig 管理器配置
type TunnelConnectionManagerConfig struct {
	DialTimeout time.Duration
	IdleTimeout time.Duration
}

// DefaultTunnelConnectionManagerConfig 返回默认配置
func DefaultTunnelConnectionManagerConfig() TunnelConnectionManagerConfig {
	return TunnelConnectionManagerConfig{
		DialTimeout: 5 * time.Second,
		IdleTimeout: 5 * time.Minute,
	}
}

// NewTunnelConnectionManager 创建隧道连接管理器
func NewTunnelConnectionManager(
	getNodeAddr func(nodeID string) (string, error),
	config TunnelConnectionManagerConfig,
) *TunnelConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &TunnelConnectionManager{
		getNodeAddr:   getNodeAddr,
		dialTimeout:   config.DialTimeout,
		idleTimeout:   config.IdleTimeout,
		cleanupCtx:    ctx,
		cleanupCancel: cancel,
	}

	// 启动后台清理
	go m.startCleanupLoop()

	corelog.Infof("TunnelConnectionManager: initialized (dialTimeout=%v, idleTimeout=%v)",
		config.DialTimeout, config.IdleTimeout)

	return m
}

// CreateDedicatedConnection 为隧道创建专用跨节点连接
// 不使用连接池，每个隧道使用独立的 TCP 连接
func (m *TunnelConnectionManager) CreateDedicatedConnection(
	ctx context.Context,
	tunnelID string,
	targetNodeID string,
	remoteAddr string,
	target string,
) (*net.TCPConn, error) {
	// 检查是否已存在
	if existing, ok := m.connections.Load(tunnelID); ok {
		tc := existing.(*CrossNodeTunnelConn)
		tc.mu.Lock()
		closed := tc.closed
		crossConn := tc.CrossConn
		tc.mu.Unlock()

		if !closed {
			corelog.Warnf("TunnelConnectionManager: tunnel %s already has connection, reusing", tunnelID)
			return crossConn, nil
		}
		// 旧连接已关闭，删除并创建新的
		m.connections.Delete(tunnelID)
	}

	// 获取目标节点地址
	nodeAddr, err := m.getNodeAddr(targetNodeID)
	if err != nil {
		corelog.Errorf("TunnelConnectionManager: failed to get node address for %s: %v", targetNodeID, err)
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to get node address")
	}

	corelog.Infof("TunnelConnectionManager[%s]: creating dedicated connection to %s (%s)",
		tunnelID, targetNodeID, nodeAddr)

	// 建立专用 TCP 连接
	dialCtx, dialCancel := context.WithTimeout(ctx, m.dialTimeout)
	defer dialCancel()

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", nodeAddr)
	if err != nil {
		corelog.Errorf("TunnelConnectionManager[%s]: failed to dial %s: %v", tunnelID, nodeAddr, err)
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to dial node")
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		conn.Close()
		return nil, coreerrors.New(coreerrors.CodeNetworkError, "connection is not TCP")
	}

	// 创建隧道连接上下文
	tcCtx, tcCancel := context.WithCancel(context.Background())

	tc := &CrossNodeTunnelConn{
		TunnelID:     tunnelID,
		RemoteAddr:   remoteAddr,
		Target:       target,
		NodeID:       targetNodeID,
		CrossConn:    tcpConn,
		ctx:          tcCtx,
		cancel:       tcCancel,
		lastActivity: time.Now(),
	}

	// 注册到管理器
	m.connections.Store(tunnelID, tc)

	corelog.Infof("TunnelConnectionManager[%s]: dedicated connection created to %s, remoteAddr=%s, target=%s",
		tunnelID, targetNodeID, remoteAddr, target)

	return tcpConn, nil
}

// GetConnection 获取隧道的专用连接
func (m *TunnelConnectionManager) GetConnection(tunnelID string) *CrossNodeTunnelConn {
	if v, ok := m.connections.Load(tunnelID); ok {
		return v.(*CrossNodeTunnelConn)
	}
	return nil
}

// UpdateActivity 更新隧道活动时间
func (m *TunnelConnectionManager) UpdateActivity(tunnelID string) {
	if v, ok := m.connections.Load(tunnelID); ok {
		tc := v.(*CrossNodeTunnelConn)
		tc.mu.Lock()
		tc.lastActivity = time.Now()
		tc.mu.Unlock()
	}
}

// CloseTunnel 关闭隧道的所有连接
// 会关闭跨节点连接并清理映射
func (m *TunnelConnectionManager) CloseTunnel(tunnelID string) {
	v, ok := m.connections.LoadAndDelete(tunnelID)
	if !ok {
		return
	}

	tc := v.(*CrossNodeTunnelConn)
	tc.closeOnce.Do(func() {
		tc.mu.Lock()
		tc.closed = true
		tc.mu.Unlock()

		// 取消上下文
		if tc.cancel != nil {
			tc.cancel()
		}

		// 关闭跨节点连接
		if tc.CrossConn != nil {
			tc.CrossConn.Close()
		}

		corelog.Infof("TunnelConnectionManager[%s]: tunnel closed, nodeID=%s, remoteAddr=%s",
			tunnelID, tc.NodeID, tc.RemoteAddr)
	})
}

// IsClosed 检查隧道是否已关闭
func (m *TunnelConnectionManager) IsClosed(tunnelID string) bool {
	v, ok := m.connections.Load(tunnelID)
	if !ok {
		return true
	}
	tc := v.(*CrossNodeTunnelConn)
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.closed
}

// GetContext 获取隧道的上下文
func (m *TunnelConnectionManager) GetContext(tunnelID string) context.Context {
	if v, ok := m.connections.Load(tunnelID); ok {
		tc := v.(*CrossNodeTunnelConn)
		return tc.ctx
	}
	return context.Background()
}

// startCleanupLoop 启动后台清理循环
func (m *TunnelConnectionManager) startCleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.cleanupCtx.Done():
			return
		case <-ticker.C:
			m.cleanupIdleConnections()
		}
	}
}

// cleanupIdleConnections 清理空闲超时的连接
func (m *TunnelConnectionManager) cleanupIdleConnections() {
	now := time.Now()
	var cleaned int

	m.connections.Range(func(key, value interface{}) bool {
		tc := value.(*CrossNodeTunnelConn)
		tc.mu.Lock()
		lastActivity := tc.lastActivity
		closed := tc.closed
		tc.mu.Unlock()

		if !closed && now.Sub(lastActivity) > m.idleTimeout {
			tunnelID := key.(string)
			corelog.Warnf("TunnelConnectionManager[%s]: connection idle for %v, closing",
				tunnelID, now.Sub(lastActivity))
			m.CloseTunnel(tunnelID)
			cleaned++
		}
		return true
	})

	if cleaned > 0 {
		corelog.Infof("TunnelConnectionManager: cleaned %d idle connections", cleaned)
	}
}

// Stats 返回统计信息
func (m *TunnelConnectionManager) Stats() map[string]interface{} {
	var total, active int
	m.connections.Range(func(key, value interface{}) bool {
		total++
		tc := value.(*CrossNodeTunnelConn)
		tc.mu.Lock()
		if !tc.closed {
			active++
		}
		tc.mu.Unlock()
		return true
	})

	return map[string]interface{}{
		"total_connections":  total,
		"active_connections": active,
	}
}

// Close 关闭管理器
func (m *TunnelConnectionManager) Close() {
	// 停止后台清理
	m.cleanupCancel()

	// 关闭所有连接
	m.connections.Range(func(key, value interface{}) bool {
		tunnelID := key.(string)
		m.CloseTunnel(tunnelID)
		return true
	})

	corelog.Infof("TunnelConnectionManager: closed")
}
