package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tunnox-core/internal/cloud"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/utils"

	"github.com/gin-gonic/gin"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string `json:"host" yaml:"host"`
	Port         int    `json:"port" yaml:"port"`
	ReadTimeout  int    `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout int    `json:"write_timeout" yaml:"write_timeout"`
	IdleTimeout  int    `json:"idle_timeout" yaml:"idle_timeout"`
}

// AppConfig 应用配置
type AppConfig struct {
	Server ServerConfig    `json:"server" yaml:"server"`
	Log    utils.LogConfig `json:"log" yaml:"log"`
}

// Server 服务器结构
type Server struct {
	config       *AppConfig
	router       *gin.Engine
	cloudControl cloud.CloudControlAPI
	httpServer   *http.Server
	protocolMgr  *protocol.ProtocolManager
	utils.Dispose
}

// NewServer 创建新服务器
func NewServer(config *AppConfig, parentCtx context.Context) *Server {
	// 初始化日志
	if err := utils.InitLogger(&config.Log); err != nil {
		utils.Fatalf("Failed to initialize logger: %v", err)
	}

	// 设置Gin模式
	if config.Log.Level == constants.LogLevelDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建路由器
	router := gin.New()

	// 创建云控制器
	cloudControl, err := cloud.NewBuiltinCloudControl(nil)
	if err != nil {
		utils.Fatalf("Failed to create cloud control: %v", err)
	}

	// 创建服务器
	server := &Server{
		config:       config,
		router:       router,
		cloudControl: cloudControl,
	}

	server.SetCtx(parentCtx, server.onClose)
	// 创建协议适配器管理器，纳入Dispose树
	protocolMgr := protocol.NewProtocolManager(server.Ctx())
	// 注册所有已支持协议（可扩展）
	tcpAdapter := protocol.NewTcpAdapter(":9000", parentCtx)
	protocolMgr.Register(tcpAdapter)
	// 后续可自动注册更多协议适配器

	server.protocolMgr = protocolMgr

	// 设置中间件
	server.setupMiddleware()

	// 设置路由
	server.setupRoutes()

	// 创建HTTP服务器
	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port),
		Handler:      router,
		ReadTimeout:  time.Duration(config.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(config.Server.IdleTimeout) * time.Second,
	}

	return server
}

// onClose 资源释放回调
func (s *Server) onClose() {
	// 优雅关闭协议适配器
	if s.protocolMgr != nil {
		s.protocolMgr.CloseAll()
	}
	// 优雅关闭HTTP服务器
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.httpServer.Shutdown(ctx); err != nil {
			utils.Errorf("Server forced to shutdown: %v", err)
		}
	}

	// 关闭云控制器
	if s.cloudControl != nil {
		_ = s.cloudControl.Close()
	}
}

// setupMiddleware 设置中间件
func (s *Server) setupMiddleware() {
	// 恢复中间件
	s.router.Use(utils.RecoveryMiddleware())

	// 请求ID中间件
	s.router.Use(utils.RequestIDMiddleware())

	// 日志中间件
	s.router.Use(utils.LoggingMiddleware())

	// CORS中间件
	s.router.Use(utils.CORSMiddleware())

	// 安全头部中间件
	s.router.Use(utils.SecurityHeadersMiddleware())

	// 指标中间件
	s.router.Use(utils.MetricsMiddleware())

	// 大小限制中间件
	s.router.Use(utils.SizeLimitMiddleware(10 * 1024 * 1024)) // 10MB

	// 超时中间件
	s.router.Use(utils.TimeoutMiddleware(30 * time.Second))
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// 创建REST处理器
	restHandler := cloud.NewRESTHandler(s.cloudControl, s.Ctx())

	// 注册路由
	restHandler.RegisterRoutes(s.router)

	// 根路径重定向到健康检查
	s.router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, constants.APIPathHealth)
	})

	// 404处理
	s.router.NoRoute(func(c *gin.Context) {
		utils.SendNotFound(c, "API endpoint not found", nil)
	})
}

// Start 启动服务器
func (s *Server) Start() error {
	addr := s.httpServer.Addr
	utils.Infof(constants.LogMsgServerStarting, addr)

	// 向云控注册节点
	if err := s.registerNodeToCloud(); err != nil {
		utils.Errorf("Failed to register node to cloud control: %v", err)
		return err
	}

	// 启动所有协议适配器
	if s.protocolMgr != nil {
		if err := s.protocolMgr.StartAll(s.Ctx()); err != nil {
			return err
		}
	}

	// 启动服务器
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			utils.Errorf("Failed to start server: %v", err)
			os.Exit(1)
		}
	}()

	utils.Info(constants.LogMsgServerStarted)
	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	utils.Info(constants.LogMsgServerShuttingDown)

	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 优雅关闭服务器
	if err := s.httpServer.Shutdown(ctx); err != nil {
		utils.Errorf("Server forced to shutdown: %v", err)
		return err
	}

	utils.Info(constants.LogMsgServerShutdown)
	return nil
}

// registerNodeToCloud 向云控注册节点
func (s *Server) registerNodeToCloud() error {
	ctx := context.Background()

	// 构建节点注册请求
	req := &cloud.NodeRegisterRequest{
		NodeID:  "", // 让云控自动分配节点ID
		Address: fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port),
		Version: "1.0.0", // 可以从配置或编译时信息获取
		Meta: map[string]string{
			"startup_time": time.Now().Format(time.RFC3339),
			"server_type":  "tunnox-core",
		},
	}

	// 调用云控注册节点
	resp, err := s.cloudControl.NodeRegister(ctx, req)
	if err != nil {
		return fmt.Errorf("cloud control node registration failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("cloud control node registration failed: %s", resp.Message)
	}

	utils.Infof("服务器节点已成功注册到云控 - 节点ID: %s", resp.NodeID)
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
			Host:         "0.0.0.0",
			Port:         8080,
			ReadTimeout:  30,
			WriteTimeout: 30,
			IdleTimeout:  60,
		},
		Log: utils.LogConfig{
			Level:  constants.LogLevelInfo,
			Format: constants.LogFormatText,
			Output: constants.LogOutputStdout,
		},
	}
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
