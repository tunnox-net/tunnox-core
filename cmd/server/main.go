package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"gopkg.in/yaml.v3"
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
	session *session.SessionManager
}

// NewProtocolFactory 创建协议工厂
func NewProtocolFactory(session *session.SessionManager) *ProtocolFactory {
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

// CloudService 云控制服务适配器
type CloudService struct {
	cloudControl managers.CloudControlAPI
	name         string
}

// NewCloudService 创建云控制服务
func NewCloudService(name string, cloudControl managers.CloudControlAPI) *CloudService {
	return &CloudService{
		cloudControl: cloudControl,
		name:         name,
	}
}

func (cs *CloudService) Name() string {
	return cs.name
}

func (cs *CloudService) Start(ctx context.Context) error {
	utils.Infof("Starting cloud service: %s", cs.name)
	// 云控制器通常不需要显式启动
	return nil
}

func (cs *CloudService) Stop(ctx context.Context) error {
	utils.Infof("Stopping cloud service: %s", cs.name)
	if cs.cloudControl != nil {
		return cs.cloudControl.Close()
	}
	return nil
}

// StorageService 存储服务适配器
type StorageService struct {
	storage storages.Storage
	name    string
}

// NewStorageService 创建存储服务
func NewStorageService(name string, storage storages.Storage) *StorageService {
	return &StorageService{
		storage: storage,
		name:    name,
	}
}

func (ss *StorageService) Name() string {
	return ss.name
}

func (ss *StorageService) Start(ctx context.Context) error {
	utils.Infof("Starting storage service: %s", ss.name)
	// 存储服务通常不需要显式启动
	return nil
}

func (ss *StorageService) Stop(ctx context.Context) error {
	utils.Infof("Stopping storage service: %s", ss.name)
	// 存储服务通常不需要显式停止
	return nil
}

// Server 服务器结构（使用ServiceManager）
type Server struct {
	config          *AppConfig
	serviceManager  *utils.ServiceManager
	protocolMgr     *protocol.Manager
	serverId        string
	storage         storages.Storage
	idManager       *idgen.IDManager
	session         *session.SessionManager
	protocolFactory *ProtocolFactory
	cloudControl    managers.CloudControlAPI
}

// NewServer 创建新服务器
func NewServer(config *AppConfig, parentCtx context.Context) *Server {
	// 初始化日志
	if err := utils.InitLogger(&config.Log); err != nil {
		utils.Fatalf("Failed to initialize logger: %v", err)
	}

	// 创建服务管理器
	serviceConfig := utils.DefaultServiceConfig()
	serviceConfig.EnableSignalHandling = true
	serviceConfig.GracefulShutdownTimeout = 30 * time.Second
	serviceConfig.ResourceDisposeTimeout = 10 * time.Second

	serviceManager := utils.NewServiceManager(serviceConfig)

	// 创建云控制器
	cloudControl := managers.NewBuiltinCloudControl(nil)

	// 创建服务器
	server := &Server{
		config:         config,
		serviceManager: serviceManager,
		cloudControl:   cloudControl,
	}

	// 创建存储和ID管理器
	server.storage = storages.NewMemoryStorage(parentCtx)
	server.idManager = idgen.NewIDManager(server.storage, parentCtx)

	// 创建 SessionManager
	server.session = session.NewSessionManager(server.idManager, parentCtx)

	// 创建协议工厂
	server.protocolFactory = NewProtocolFactory(server.session)

	// 创建协议适配器管理器
	server.protocolMgr = protocol.NewManager(parentCtx)

	server.serverId, _ = server.idManager.GenerateConnectionID()

	// 注册服务到服务管理器
	server.registerServices()

	return server
}

// registerServices 注册所有服务到服务管理器
func (s *Server) registerServices() {
	// 注册云控制服务
	cloudService := NewCloudService("Cloud-Control", s.cloudControl)
	s.serviceManager.RegisterService(cloudService)

	// 注册存储服务
	storageService := NewStorageService("Storage", s.storage)
	s.serviceManager.RegisterService(storageService)

	// 注册协议服务
	protocolService := protocol.NewProtocolService("Protocol-Manager", s.protocolMgr)
	s.serviceManager.RegisterService(protocolService)

	// 注册流服务
	streamFactory := stream.NewDefaultStreamFactory(s.serviceManager.GetContext())
	streamManager := stream.NewStreamManager(streamFactory, s.serviceManager.GetContext())
	streamService := stream.NewStreamService("Stream-Manager", streamManager)
	s.serviceManager.RegisterService(streamService)

	// 注册实现了Disposable接口的资源
	// 注意：只有实现了Disposable接口的组件才能被注册为资源

	// 注册协议管理器（已实现Disposable接口）
	if err := s.serviceManager.RegisterResource("protocol-manager", s.protocolMgr); err != nil {
		utils.Errorf("Failed to register protocol manager: %v", err)
	}

	// 注册流管理器（已实现Disposable接口）
	if err := s.serviceManager.RegisterResource("stream-manager", streamManager); err != nil {
		utils.Errorf("Failed to register stream manager: %v", err)
	}

	// 注册存储组件（如果实现了Disposable接口）
	if disposable, ok := s.storage.(utils.Disposable); ok {
		if err := s.serviceManager.RegisterResource("storage", disposable); err != nil {
			utils.Errorf("Failed to register storage: %v", err)
		}
	}

	// 注册云控制器（如果实现了Disposable接口）
	if disposable, ok := s.cloudControl.(utils.Disposable); ok {
		if err := s.serviceManager.RegisterResource("cloud-control", disposable); err != nil {
			utils.Errorf("Failed to register cloud control: %v", err)
		}
	}

	utils.Infof("Registered %d services and %d resources",
		s.serviceManager.GetServiceCount(),
		s.serviceManager.GetResourceCount())
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
		adapter, err := s.protocolFactory.CreateAdapter(protocolName, s.serviceManager.GetContext())
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

// Start 启动服务器
func (s *Server) Start() error {
	utils.Info(constants.MsgStartingServer)

	// 设置协议适配器
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器启动所有服务
	if err := s.serviceManager.StartAllServices(); err != nil {
		return fmt.Errorf("failed to start services: %v", err)
	}

	utils.Info(constants.MsgServerStarted)
	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	utils.Info(constants.MsgShuttingDownServer)

	// 使用服务管理器停止所有服务
	if err := s.serviceManager.StopAllServices(); err != nil {
		utils.Errorf("Failed to stop services: %v", err)
	}

	utils.Info(constants.MsgServerShutdownCompleted)
	return nil
}

// Run 运行服务器（使用ServiceManager的优雅关闭）
func (s *Server) Run() error {
	utils.Info("Starting Tunnox Core with ServiceManager...")

	// 设置协议适配器（但不启动服务）
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器运行（包含信号处理和优雅关闭）
	return s.serviceManager.Run()
}

// RunWithContext 使用指定上下文运行服务器
func (s *Server) RunWithContext(ctx context.Context) error {
	utils.Info("Starting Tunnox Core with ServiceManager...")

	// 设置协议适配器（但不启动服务）
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器运行（包含信号处理和优雅关闭）
	return s.serviceManager.RunWithContext(ctx)
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

	// 使用ServiceManager运行服务器（包含信号处理和优雅关闭）
	if err := server.Run(); err != nil {
		utils.Fatalf("Failed to run server: %v", err)
	}

	utils.Info("Tunnox Core server exited gracefully")
}
