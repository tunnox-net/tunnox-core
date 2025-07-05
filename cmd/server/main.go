package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"tunnox-core/internal/cloud"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/utils"
)

// AppConfig 应用配置
type AppConfig struct {
	Log utils.LogConfig `json:"log" yaml:"log"`
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
	utils.Info("Starting protocol adapters...")

	// 启动所有协议适配器
	if s.protocolMgr != nil {
		if err := s.protocolMgr.StartAll(s.Ctx()); err != nil {
			return err
		}
	}

	utils.Info("Protocol adapters started successfully")
	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	utils.Info("Shutting down protocol adapters...")

	// 关闭协议适配器
	if s.protocolMgr != nil {
		s.protocolMgr.CloseAll()
	}

	utils.Info("Protocol adapters shutdown completed")
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
