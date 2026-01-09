package server

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/broker"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/httpservice/modules/domainproxy"
	"tunnox-core/internal/httpservice/modules/management"
	"tunnox-core/internal/httpservice/modules/websocket"
	"tunnox-core/internal/protocol"
)

// ============================================================================
// ProtocolComponent - 协议组件
// ============================================================================

// ProtocolComponent 协议组件
type ProtocolComponent struct {
	*BaseComponent
}

func (c *ProtocolComponent) Name() string {
	return "Protocol"
}

func (c *ProtocolComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if deps.SessionMgr == nil {
		return fmt.Errorf("SessionManager is required")
	}

	deps.ProtocolFactory = NewProtocolFactory(deps.SessionMgr)
	deps.ProtocolMgr = protocol.NewProtocolManager(ctx)

	corelog.Infof("Protocol components initialized")
	return nil
}

func (c *ProtocolComponent) Start() error {
	return nil
}

func (c *ProtocolComponent) Stop() error {
	return nil
}

// ============================================================================
// MessageBrokerComponent - 消息代理组件
// ============================================================================

// MessageBrokerComponent 消息代理组件
type MessageBrokerComponent struct {
	*BaseComponent
}

func (c *MessageBrokerComponent) Name() string {
	return "MessageBroker"
}

func (c *MessageBrokerComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	// 根据配置决定使用哪种消息代理
	brokerType := "memory"
	var redisConfig *broker.RedisBrokerConfig

	if deps.Config.Redis.Enabled {
		brokerType = "redis"
		redisConfig = &broker.RedisBrokerConfig{
			Addrs:       []string{deps.Config.Redis.Addr},
			Password:    deps.Config.Redis.Password,
			DB:          deps.Config.Redis.DB,
			ClusterMode: false,
			PoolSize:    10,
		}
	}

	brokerConfig := &broker.BrokerConfig{
		Type:   broker.BrokerType(brokerType),
		NodeID: deps.NodeID,
		Redis:  redisConfig,
	}

	mb, err := broker.NewMessageBroker(ctx, brokerConfig)
	if err != nil {
		return fmt.Errorf("failed to create message broker: %w", err)
	}

	deps.MessageBroker = mb

	// 注入 BridgeAdapter 到 SessionManager
	if deps.SessionMgr != nil && mb != nil {
		bridgeAdapter := NewBridgeAdapter(ctx, mb, deps.NodeID)
		deps.SessionMgr.SetBridgeManager(bridgeAdapter)
		deps.BridgeAdapter = bridgeAdapter // 保存到 deps 供后续组件使用
	}

	// 注入 MessageBroker 到 CloudControl（用于客户端状态事件发布）
	if deps.CloudBuiltin != nil && mb != nil {
		deps.CloudBuiltin.SetBroker(mb)
	}

	corelog.Infof("MessageBroker initialized: type=%s", brokerType)
	return nil
}

func (c *MessageBrokerComponent) Start() error {
	return nil
}

func (c *MessageBrokerComponent) Stop() error {
	return nil
}

// ============================================================================
// HTTPServiceComponent - HTTP 服务组件（替代旧的 ManagementAPIComponent）
// ============================================================================

// HTTPServiceComponent HTTP 服务组件
type HTTPServiceComponent struct {
	*BaseComponent
}

func (c *HTTPServiceComponent) Name() string {
	return "HTTPService"
}

func (c *HTTPServiceComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	// HTTP 服务始终启用

	// 检查协议是否在 server.protocols 中启用
	websocketEnabled := false
	if wsConfig, exists := deps.Config.Server.Protocols["websocket"]; exists {
		websocketEnabled = wsConfig.Enabled
	}

	// 构建 HTTP 服务配置
	httpConfig := &httpservice.HTTPServiceConfig{
		Enabled:    true,
		ListenAddr: deps.Config.Management.Listen,
		CORS: httpservice.CORSConfig{
			Enabled:        false,
			AllowedOrigins: []string{},
		},
		RateLimit: httpservice.RateLimitConfig{
			Enabled: false,
		},
		Modules: httpservice.ModulesConfig{
			ManagementAPI: httpservice.ManagementAPIModuleConfig{
				Enabled: true,
				Auth: httpservice.AuthConfig{
					Type:   deps.Config.Management.Auth.Type,
					Secret: deps.Config.Management.Auth.Token,
				},
				PProf: httpservice.PProfConfig{
					Enabled:     deps.Config.Management.PProf.Enabled,
					DataDir:     deps.Config.Management.PProf.DataDir,
					Retention:   deps.Config.Management.PProf.Retention,
					AutoCapture: deps.Config.Management.PProf.AutoCapture,
				},
			},
			WebSocket: httpservice.WebSocketModuleConfig{
				Enabled: websocketEnabled,
			},
			DomainProxy: httpservice.DomainProxyModuleConfig{
				Enabled:              true,
				BaseDomains:          []string{},
				DefaultScheme:        "http",
				CommandModeThreshold: 1048576, // 1MB
				RequestTimeout:       30 * time.Second,
			},
		},
	}

	// 创建 HTTP 服务
	httpSvc := httpservice.NewHTTPService(ctx, httpConfig, deps.CloudControl, deps.Storage, deps.HealthManager)

	// 创建并注册 Management API 模块
	mgmtConfig := &httpConfig.Modules.ManagementAPI
	mgmtModule := management.NewManagementModule(ctx, mgmtConfig, deps.CloudControl, deps.ConnCodeService, deps.HealthManager)
	httpSvc.RegisterModule(mgmtModule)

	// 创建并注册 WebSocket 模块（如果启用）
	if websocketEnabled {
		wsConfig := &httpConfig.Modules.WebSocket
		wsModule := websocket.NewWebSocketModule(ctx, wsConfig)
		if deps.SessionMgr != nil {
			wsModule.SetSession(deps.SessionMgr)
		}
		httpSvc.RegisterModule(wsModule)
	}

	// 创建并注册 Domain Proxy 模块
	domainProxyConfig := &httpConfig.Modules.DomainProxy
	domainProxyModule := domainproxy.NewDomainProxyModule(ctx, domainProxyConfig)
	httpSvc.RegisterModule(domainProxyModule)

	// 设置 HTTP 域名映射仓库（必须在 RegisterModule 之后设置，确保延迟绑定生效）
	if deps.HTTPDomainRepo != nil {
		httpSvc.SetHTTPDomainMappingRepo(deps.HTTPDomainRepo)
	}

	// 设置会话管理器（如果可用）
	if deps.SessionMgr != nil {
		httpSvc.SetSessionManager(NewSessionManagerAdapter(deps.SessionMgr))
	}

	if deps.WebhookManager != nil {
		httpSvc.SetWebhookManager(deps.WebhookManager)
	}

	deps.HTTPService = httpSvc

	corelog.Infof("HTTPService initialized: addr=%s", httpConfig.ListenAddr)
	return nil
}

func (c *HTTPServiceComponent) Start() error {
	return nil
}

func (c *HTTPServiceComponent) Stop() error {
	return nil
}
