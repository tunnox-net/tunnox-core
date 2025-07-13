package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/storages"

	"gopkg.in/yaml.v3"

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
	Host         string                    `yaml:"host"`
	Port         int                       `yaml:"port"`
	ReadTimeout  int                       `yaml:"read_timeout"`
	WriteTimeout int                       `yaml:"write_timeout"`
	IdleTimeout  int                       `yaml:"idle_timeout"`
	Protocols    map[string]ProtocolConfig `yaml:"protocols"`
}

// CloudConfig 云控配置
type CloudConfig struct {
	Type     string                 `yaml:"type"`
	BuiltIn  map[string]interface{} `yaml:"built_in"`
	External map[string]interface{} `yaml:"external"`
}

// AppConfig 应用配置
type AppConfig struct {
	Server ServerConfig    `yaml:"server"`
	Log    utils.LogConfig `yaml:"log"`
	Cloud  CloudConfig     `yaml:"cloud"`
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
	config          *AppConfig
	cloudControl    managers.CloudControlAPI
	protocolMgr     *protocol.Manager
	serverId        string
	storage         storages.Storage
	idManager       *generators.IDManager
	session         *protocol.ConnectionSession
	protocolFactory *ProtocolFactory

	utils.Dispose
}

// NewServer 创建新服务器
func NewServer(config *AppConfig, parentCtx context.Context) *Server {
	// 初始化日志
	if err := utils.InitLogger(&config.Log); err != nil {
		utils.Fatalf("Failed to initialize logger: %v", err)
	}

	// 创建云控制器
	cloudControl := managers.NewBuiltinCloudControl(nil)

	// 创建服务器
	server := &Server{
		config:       config,
		cloudControl: cloudControl,
	}

	server.SetCtx(parentCtx, server.onClose)

	// 创建存储和ID管理器
	server.storage = storages.NewMemoryStorage(server.Ctx())
	server.idManager = generators.NewIDManager(server.storage, server.Ctx())

	// 创建 ConnectionSession（使用新的架构）
	server.session = protocol.NewConnectionSession(server.idManager, server.Ctx())

	// 创建协议工厂
	server.protocolFactory = NewProtocolFactory(server.session)

	// 创建协议适配器管理器，纳入Dispose树
	server.protocolMgr = protocol.NewManager(server.Ctx())

	server.serverId, _ = server.idManager.GenerateConnectionID()

	return server
}

// setupProtocolAdapters 设置协议适配器
func (s *Server) setupProtocolAdapters() error {

	// 获取启用的协议配置
	enabledProtocols := s.getEnabledProtocols()
	if len(enabledProtocols) == 0 {
		utils.Warn(constants.MsgNoProtocolsEnabled)
		return nil
	}

	// 创建并注册所有启用的协议适配器
	registeredProtocols := make([]string, 0, len(enabledProtocols))

	for protocolName, config := range enabledProtocols {
		// 创建适配器
		adapter, err := s.protocolFactory.CreateAdapter(protocolName, s.protocolMgr.Ctx())
		if err != nil {
			return fmt.Errorf("failed to create %s adapter: %v", protocolName, err)
		}

		// 配置监听地址
		addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
		adapter.SetAddr(addr)

		// 注册到管理器
		s.protocolMgr.Register(adapter)
		registeredProtocols = append(registeredProtocols, protocolName)

		utils.Infof(constants.MsgAdapterConfigured, capitalize(protocolName), addr)
	}

	utils.Infof(constants.MsgRegisteredAdapters, len(registeredProtocols), registeredProtocols)
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
func (s *Server) onClose() error {
	// 优雅关闭协议适配器
	if s.protocolMgr != nil {
		err := s.protocolMgr.CloseAll()
		if err != nil {
			return err
		}
	}

	// 关闭云控制器
	if s.cloudControl != nil {
		utils.Info(constants.MsgClosingCloudControl)
		if err := s.cloudControl.Close(); err != nil {
			utils.Errorf("Cloud control closed with error: %v", err)
			return err
		} else {
			utils.Info(constants.MsgCloudControlClosed)
		}
	}

	return nil
}

// Start 启动服务器
func (s *Server) Start() error {
	utils.Info(constants.MsgStartingServer)

	// 设置协议适配器
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 启动所有协议适配器
	if s.protocolMgr != nil {
		utils.Info("Starting all protocol adapters...")
		if err := s.protocolMgr.StartAll(); err != nil {
			return fmt.Errorf("failed to start protocol adapters: %v", err)
		}
		utils.Info(constants.MsgAllAdaptersStarted)
	} else {
		utils.Warn("Protocol manager is nil")
	}

	utils.Info(constants.MsgServerStarted)
	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	utils.Info(constants.MsgShuttingDownServer)

	// 关闭协议适配器
	if s.protocolMgr != nil {
		s.protocolMgr.CloseAll()
		utils.Info(constants.MsgAllProtocolManagerClosed)
	}

	// 关闭云控制器
	if s.cloudControl != nil {
		utils.Info(constants.MsgClosingCloudControl)
		if err := s.cloudControl.Close(); err != nil {
			utils.Errorf("Cloud control closed with error: %v", err)
		} else {
			utils.Info(constants.MsgCloudControlClosed)
		}
	}

	s.Ctx().Done()
	utils.Info(constants.MsgServerShutdownCompleted)
	return nil
}

// WaitForShutdown 等待关闭信号
func (s *Server) WaitForShutdown() {
	// 创建信号通道
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	<-quit

	utils.Info(constants.MsgReceivedShutdownSignal)

	// 停止服务器
	if err := s.Stop(); err != nil {
		utils.Errorf("Failed to stop server: %v", err)
		os.Exit(1)
	}

	// 确保所有资源都被清理
	utils.Info(constants.MsgCleaningUpServerResources)
	s.Dispose.Close()

	utils.Info(constants.MsgServerShutdownMainExited)
}

// loadConfig 加载配置文件
func loadConfig(configPath string) (*AppConfig, error) {
	// 如果配置文件不存在，使用默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		utils.Warnf(constants.MsgConfigFileNotFound, configPath)
		return getDefaultConfig(), nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf(constants.MsgFailedToReadConfigFile, configPath, err)
	}

	// 解析YAML
	var config AppConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf(constants.MsgFailedToParseConfigFile, configPath, err)
	}

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf(constants.MsgInvalidConfiguration, err)
	}

	utils.Infof(constants.MsgConfigLoadedFrom, configPath)
	return &config, nil
}

// validateConfig 验证配置
func validateConfig(config *AppConfig) error {
	// 验证服务器配置
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.Port <= 0 {
		config.Server.Port = 8080
	}

	// 验证协议配置
	if config.Server.Protocols == nil {
		config.Server.Protocols = make(map[string]ProtocolConfig)
	}

	// 设置默认协议配置
	defaultProtocols := map[string]ProtocolConfig{
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
	}

	// 合并默认配置
	for name, defaultConfig := range defaultProtocols {
		if _, exists := config.Server.Protocols[name]; !exists {
			config.Server.Protocols[name] = defaultConfig
		}
	}

	// 验证日志配置
	if config.Log.Level == "" {
		config.Log.Level = constants.LogLevelInfo
	}
	if config.Log.Format == "" {
		config.Log.Format = constants.LogFormatText
	}
	if config.Log.Output == "" {
		config.Log.Output = constants.LogOutputStdout
	}

	return nil
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() *AppConfig {
	return &AppConfig{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			ReadTimeout:  30,
			WriteTimeout: 30,
			IdleTimeout:  60,
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
		Cloud: CloudConfig{
			Type: "built_in",
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
	// 解析命令行参数
	var (
		configPath = flag.String("config", "config.yaml", "Path to configuration file")
		showHelp   = flag.Bool("help", false, "Show help information")
	)
	flag.Parse()

	// 显示帮助信息
	if *showHelp {
		utils.Info("Tunnox Core Server")
		utils.Info("Usage: server.exe [options]")
		utils.Info()
		utils.Info("Options:")
		flag.PrintDefaults()
		utils.Info()
		utils.Info("Examples:")
		utils.Info("  server.exe                    # 使用当前目录下的 config.yaml")
		utils.Info("  server.exe -config ./my_config.yaml")
		utils.Info("  server.exe -config /path/to/config.yaml")
		return
	}

	// 获取配置文件绝对路径
	absConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		utils.Fatalf("Failed to resolve config path: %v", err)
	}

	// 加载配置
	config, err := loadConfig(absConfigPath)
	if err != nil {
		utils.Fatalf("Failed to load configuration: %v", err)
	}

	// 创建服务器
	server := NewServer(config, context.Background())

	// 启动服务器
	if err := server.Start(); err != nil {
		utils.Fatalf("Failed to start server: %v", err)
	}

	// 等待关闭信号并处理关闭
	server.WaitForShutdown()
}
