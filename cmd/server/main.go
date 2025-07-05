package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tunnox-core/internal/cloud"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/utils"
)

// ProtocolConfig 协议配置
type ProtocolConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Host    string `yaml:"host"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host      string                    `yaml:"host"`
	Port      int                       `yaml:"port"`
	Protocols map[string]ProtocolConfig `yaml:"protocols"`
}

// AppConfig 应用配置
type AppConfig struct {
	Server ServerConfig    `yaml:"server"`
	Log    utils.LogConfig `yaml:"log"`
}

// ProtocolFactory 协议工厂
type ProtocolFactory struct {
	session *protocol.ConnectionSession
}

// NewProtocolFactory 创建协议工厂
func NewProtocolFactory(session *protocol.ConnectionSession) *ProtocolFactory {
	return &ProtocolFactory{
		session: session,
	}
}

// CreateAdapter 创建协议适配器
func (pf *ProtocolFactory) CreateAdapter(protocolName string, ctx context.Context) (protocol.Adapter, error) {
	switch protocolName {
	case "tcp":
		return protocol.NewTcpAdapter(ctx, pf.session), nil
	case "websocket":
		return protocol.NewWebSocketAdapter(ctx, pf.session), nil
	case "udp":
		return protocol.NewUdpAdapter(ctx, pf.session), nil
	case "quic":
		return protocol.NewQuicAdapter(ctx, pf.session), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocolName)
	}
}

// Server 服务器结构
type Server struct {
	config       *AppConfig
	cloudControl cloud.CloudControlAPI
	protocolMgr  *protocol.Manager
	utils.Dispose
}

// NewServer 创建新服务器
func NewServer(config *AppConfig, parentCtx context.Context) *Server {
	// 初始化日志
	if err := utils.InitLogger(&config.Log); err != nil {
		utils.Fatalf("Failed to initialize logger: %v", err)
	}

	// 创建云控制器
	cloudControl, err := cloud.NewBuiltinCloudControl(nil)
	if err != nil {
		utils.Fatalf("Failed to create cloud control: %v", err)
	}

	// 创建服务器
	server := &Server{
		config:       config,
		cloudControl: cloudControl,
	}

	server.SetCtx(parentCtx, server.onClose)

	// 创建协议适配器管理器，纳入Dispose树
	protocolMgr := protocol.NewManager(server.Ctx())
	server.protocolMgr = protocolMgr

	return server
}

// setupProtocolAdapters 设置协议适配器
func (s *Server) setupProtocolAdapters() error {
	// 创建 ConnectionSession
	session := &protocol.ConnectionSession{}
	session.SetCtx(s.Ctx(), nil)

	// 创建协议工厂
	factory := NewProtocolFactory(session)

	// 获取启用的协议配置
	enabledProtocols := s.getEnabledProtocols()
	if len(enabledProtocols) == 0 {
		utils.Warn("No protocols enabled in configuration")
		return nil
	}

	// 创建并注册所有启用的协议适配器
	registeredProtocols := make([]string, 0, len(enabledProtocols))

	for protocolName, config := range enabledProtocols {
		// 创建适配器
		adapter, err := factory.CreateAdapter(protocolName, s.Ctx())
		if err != nil {
			return fmt.Errorf("failed to create %s adapter: %v", protocolName, err)
		}

		// 配置监听地址
		addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
		if err := adapter.ListenFrom(addr); err != nil {
			return fmt.Errorf("failed to configure %s adapter on %s: %v", protocolName, addr, err)
		}

		// 注册到管理器
		s.protocolMgr.Register(adapter)
		registeredProtocols = append(registeredProtocols, protocolName)

		utils.Infof("%s adapter configured on %s", capitalize(protocolName), addr)
	}

	utils.Infof("Successfully registered %d protocol adapters: %v", len(registeredProtocols), registeredProtocols)
	return nil
}

// getEnabledProtocols 获取启用的协议配置
func (s *Server) getEnabledProtocols() map[string]ProtocolConfig {
	enabled := make(map[string]ProtocolConfig)

	for name, config := range s.config.Server.Protocols {
		if config.Enabled {
			enabled[name] = config
		}
	}

	return enabled
}

// onClose 资源释放回调
func (s *Server) onClose() {
	// 优雅关闭协议适配器
	if s.protocolMgr != nil {
		s.protocolMgr.CloseAll()
	}

	// 关闭云控制器
	if s.cloudControl != nil {
		_ = s.cloudControl.Close()
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	utils.Info("Starting tunnox-core server...")

	// 设置协议适配器
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 启动所有协议适配器
	if s.protocolMgr != nil {
		utils.Info("Starting all protocol adapters...")
		if err := s.protocolMgr.StartAll(s.Ctx()); err != nil {
			return fmt.Errorf("failed to start protocol adapters: %v", err)
		}
		utils.Info("All protocol adapters started successfully")
	} else {
		utils.Warn("Protocol manager is nil")
	}

	utils.Info("Tunnox-core server started successfully")
	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	utils.Info("Shutting down tunnox-core server...")

	// 关闭协议适配器
	if s.protocolMgr != nil {
		s.protocolMgr.CloseAll()
	}

	utils.Info("Tunnox-core server shutdown completed")
	return nil
}

// WaitForShutdown 等待关闭信号
func (s *Server) WaitForShutdown() {
	// 创建信号通道
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	<-quit
	utils.Info("Received shutdown signal")
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() *AppConfig {
	return &AppConfig{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			Protocols: map[string]ProtocolConfig{
				"tcp": {
					Enabled: true,
					Port:    8080,
					Host:    "0.0.0.0",
				},
				"websocket": {
					Enabled: true,
					Port:    8081,
					Host:    "0.0.0.0",
				},
				"udp": {
					Enabled: true,
					Port:    8082,
					Host:    "0.0.0.0",
				},
				"quic": {
					Enabled: true,
					Port:    8083,
					Host:    "0.0.0.0",
				},
			},
		},
		Log: utils.LogConfig{
			Level:  constants.LogLevelInfo,
			Format: constants.LogFormatText,
			Output: constants.LogOutputStdout,
		},
	}
}

// capitalize 首字母大写
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]&^32) + s[1:]
}

func main() {
	// 获取配置
	config := getDefaultConfig()

	// 创建服务器
	server := NewServer(config, context.Background())

	// 启动服务器
	if err := server.Start(); err != nil {
		utils.Fatalf("Failed to start server: %v", err)
	}

	// 等待关闭信号
	server.WaitForShutdown()

	// 停止服务器
	if err := server.Stop(); err != nil {
		utils.Errorf("Failed to stop server: %v", err)
		os.Exit(1)
	}
}
