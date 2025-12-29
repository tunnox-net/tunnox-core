package client

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/client/mapping"
	"tunnox-core/internal/client/notify"
	"tunnox-core/internal/client/socks5"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"

	"github.com/google/uuid"
)

// TunnoxClient 隧道客户端
type TunnoxClient struct {
	*dispose.ManagerBase

	config *ClientConfig

	// 记录命令行参数中是否指定了服务器地址和协议（用于判断是否允许保存到配置文件）
	serverAddressFromCLI  bool
	serverProtocolFromCLI bool

	// 标记是否使用了自动连接检测（首次连接成功后应保存服务器配置）
	usedAutoConnection bool

	// 客户端实例标识（进程级别的唯一标识）
	instanceID string

	// 指令连接
	controlConn   net.Conn
	controlStream stream.PackageStreamer

	// 映射管理
	mappingHandlers map[string]MappingHandler
	mu              sync.RWMutex

	// SOCKS5 代理管理器（运行在入口端 ClientA）
	socks5Manager *socks5.Manager

	// 商业化控制：配额缓存
	cachedQuota      *models.UserQuota
	quotaCacheMu     sync.RWMutex
	quotaLastRefresh time.Time

	// 商业化控制：流量累计
	localTrafficStats map[string]*localMappingStats // mappingID -> stats
	trafficStatsMu    sync.RWMutex

	// API客户端（用于CLI调用Management API）
	apiClient *ManagementAPIClient

	// 命令响应管理器（用于指令通道命令）
	commandResponseManager *CommandResponseManager

	// 隧道连接池（用于复用隧道连接，提高并发性能）
	tunnelPool *TunnelPool

	// 配置请求控制（防止重复请求）
	configRequesting atomic.Bool

	// readLoop 控制（防止重复启动）
	readLoopRunning atomic.Bool

	// heartbeatLoop 控制（防止重复启动）
	heartbeatLoopRunning atomic.Bool

	// 重连控制（防止重复重连）
	reconnecting atomic.Bool

	// 重连控制
	kicked     bool // 是否被踢下线
	authFailed bool // 是否认证失败

	// 启动时间（用于计算运行时间）
	startTime time.Time

	// 通知处理
	notificationDispatcher *notify.Dispatcher

	// 目标端隧道管理器（用于接收隧道关闭通知时关闭对应隧道）
	targetTunnelManager *TargetTunnelManager
}

// GetInstanceID 获取客户端实例标识
func (c *TunnoxClient) GetInstanceID() string {
	return c.instanceID
}

// localMappingStats 本地映射流量统计
type localMappingStats struct {
	bytesSent      int64
	bytesReceived  int64
	lastReportTime time.Time
	mu             sync.RWMutex
}

// NewClient 创建客户端
func NewClient(ctx context.Context, config *ClientConfig) *TunnoxClient {
	return NewClientWithCLIFlags(ctx, config, false, false)
}

// NewClientWithCLIFlags 创建客户端（带命令行参数标志）
func NewClientWithCLIFlags(ctx context.Context, config *ClientConfig, serverAddressFromCLI, serverProtocolFromCLI bool) *TunnoxClient {
	// 生成客户端实例标识（进程级别的唯一UUID）
	instanceID := uuid.New().String()

	client := &TunnoxClient{
		ManagerBase:            dispose.NewManager("TunnoxClient", ctx),
		config:                 config,
		serverAddressFromCLI:   serverAddressFromCLI,
		serverProtocolFromCLI:  serverProtocolFromCLI,
		instanceID:             instanceID,
		mappingHandlers:        make(map[string]MappingHandler),
		localTrafficStats:      make(map[string]*localMappingStats),
		commandResponseManager: NewCommandResponseManager(),
		startTime:              time.Now(),
		notificationDispatcher: notify.NewDispatcher(),
		targetTunnelManager:    NewTargetTunnelManager(),
	}

	corelog.Infof("Client: instance ID generated: %s", instanceID)

	// 初始化 SOCKS5 管理器（延迟初始化隧道创建器，等待 clientID 确定）
	tunnelCreator := NewSOCKS5TunnelCreatorImpl(client)
	client.socks5Manager = socks5.NewManager(client.Ctx(), config.ClientID, tunnelCreator)

	// 初始化API客户端（用于CLI）
	// 假设Management API在服务器地址的8080端口
	managementAPIAddr := config.Server.Address
	if managementAPIAddr == "" {
		managementAPIAddr = "localhost:8080"
	}
	client.apiClient = NewManagementAPIClient(managementAPIAddr, config.ClientID, config.SecretKey)

	// 初始化隧道连接池
	client.tunnelPool = NewTunnelPool(client, DefaultTunnelPoolConfig())

	// 注册目标端通知处理器（处理隧道关闭通知）
	client.notificationDispatcher.AddHandler(NewTargetNotificationHandler(client.targetTunnelManager))

	// 添加清理处理器
	client.AddCleanHandler(func() error {
		corelog.Infof("Client: cleaning up client resources")

		// 关闭隧道连接池
		if client.tunnelPool != nil {
			client.tunnelPool.Shutdown()
		}

		// 关闭 SOCKS5 管理器
		if client.socks5Manager != nil {
			client.socks5Manager.Close()
		}

		// 关闭所有映射处理器
		client.mu.RLock()
		handlers := make([]MappingHandler, 0, len(client.mappingHandlers))
		for _, handler := range client.mappingHandlers {
			handlers = append(handlers, handler)
		}
		client.mu.RUnlock()

		for _, handler := range handlers {
			handler.Stop()
		}

		// 关闭控制连接
		if client.controlConn != nil {
			client.controlConn.Close()
		}

		return nil
	})

	return client
}

// Stop 停止客户端
func (c *TunnoxClient) Stop() {
	corelog.Infof("Client: stopping...")
	c.Close()
}

// WasKicked 检查是否因被踢下线而断开连接
// 用于 main.go 判断退出码
func (c *TunnoxClient) WasKicked() bool {
	return c.kicked
}

// GetContext 获取上下文（供映射处理器使用）
func (c *TunnoxClient) GetContext() context.Context {
	return c.Ctx()
}

// GetConfig 获取配置（供映射处理器使用）
func (c *TunnoxClient) GetConfig() *ClientConfig {
	return c.config
}

// GetServerProtocol 获取服务器协议（供映射处理器使用）
func (c *TunnoxClient) GetServerProtocol() string {
	if c.config == nil {
		return "tcp" // 默认协议
	}
	protocol := c.config.Server.Protocol
	if protocol == "" {
		return "tcp" // 默认协议
	}
	return protocol
}

// GetAPIClient 获取Management API客户端（供CLI使用）
func (c *TunnoxClient) GetAPIClient() *ManagementAPIClient {
	return c.apiClient
}

// GetClientID 获取客户端ID
func (c *TunnoxClient) GetClientID() int64 {
	if c.config == nil {
		return 0
	}
	return c.config.ClientID
}

// GetStatusInfo 获取客户端状态信息（供CLI使用）
type StatusInfo struct {
	ActiveMappings     int
	TotalBytesSent     int64
	TotalBytesReceived int64
}

// GetStatusInfo 获取客户端状态信息
func (c *TunnoxClient) GetStatusInfo() *StatusInfo {
	c.mu.RLock()
	activeMappings := len(c.mappingHandlers)
	c.mu.RUnlock()

	// 汇总流量统计
	var totalSent, totalReceived int64
	c.trafficStatsMu.RLock()
	for _, stats := range c.localTrafficStats {
		stats.mu.RLock()
		totalSent += stats.bytesSent
		totalReceived += stats.bytesReceived
		stats.mu.RUnlock()
	}
	c.trafficStatsMu.RUnlock()

	return &StatusInfo{
		ActiveMappings:     activeMappings,
		TotalBytesSent:     totalSent,
		TotalBytesReceived: totalReceived,
	}
}

// Status 客户端状态（供调试 API 使用）
type Status struct {
	Connected    bool
	ClientID     int64
	ServerAddr   string
	Protocol     string
	Uptime       time.Duration
	MappingCount int
}

// GetStatus 获取客户端状态
func (c *TunnoxClient) GetStatus() *Status {
	c.mu.RLock()
	connected := c.controlConn != nil && c.controlStream != nil
	c.mu.RUnlock()

	config := c.GetConfig()
	serverAddr := config.Server.Address
	if serverAddr == "" {
		serverAddr = "not configured"
	}
	protocol := config.Server.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	statusInfo := c.GetStatusInfo()

	return &Status{
		Connected:    connected,
		ClientID:     config.ClientID,
		ServerAddr:   serverAddr,
		Protocol:     protocol,
		Uptime:       time.Since(c.startTime),
		MappingCount: statusInfo.ActiveMappings,
	}
}

// GetTunnelPool 获取隧道连接池
func (c *TunnoxClient) GetTunnelPool() *TunnelPool {
	return c.tunnelPool
}

// GetTunnelPoolStats 获取连接池统计信息
func (c *TunnoxClient) GetTunnelPoolStats() map[string]interface{} {
	if c.tunnelPool == nil {
		return nil
	}
	return c.tunnelPool.Stats()
}

// IsTunnelPoolEnabled 是否启用了连接池
func (c *TunnoxClient) IsTunnelPoolEnabled() bool {
	return c.tunnelPool != nil && c.tunnelPool.config.Enabled
}

// DialTunnelPooled 从连接池获取隧道连接
func (c *TunnoxClient) DialTunnelPooled(mappingID, secretKey string) (mapping.PooledTunnelConnInterface, error) {
	if c.tunnelPool == nil || !c.tunnelPool.config.Enabled {
		return nil, nil
	}
	return c.tunnelPool.Get(mappingID, secretKey)
}

// ReturnTunnelToPool 归还连接到池中
func (c *TunnoxClient) ReturnTunnelToPool(conn mapping.PooledTunnelConnInterface) {
	if c.tunnelPool == nil {
		return
	}
	if pooledConn, ok := conn.(*PooledTunnelConn); ok {
		c.tunnelPool.Put(pooledConn)
	}
}

// CloseTunnelFromPool 关闭连接（不归还到池）
func (c *TunnoxClient) CloseTunnelFromPool(conn mapping.PooledTunnelConnInterface) {
	if c.tunnelPool == nil {
		return
	}
	if pooledConn, ok := conn.(*PooledTunnelConn); ok {
		c.tunnelPool.Close(pooledConn)
	}
}
