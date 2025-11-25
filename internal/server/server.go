package server

import (
	"context"
	"fmt"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// TunnoxServer Tunnox 服务器核心类（简化版）
// 封装核心服务器逻辑，便于在 main.go 中调用
type TunnoxServer struct {
	*dispose.ManagerBase
	
	config *ServerConfig
	
	// 核心管理器
	sessionManager *session.SessionManager
	tunnelManager  *TunnelManager
	authManager    *AuthManager
}

// NewTunnoxServer 创建 Tunnox 服务器
func NewTunnoxServer(ctx context.Context, config *ServerConfig) (*TunnoxServer, error) {
	if config == nil {
		return nil, fmt.Errorf("server config is required")
	}
	
	if config.CloudControl == nil {
		return nil, fmt.Errorf("cloud control is required")
	}
	
	utils.Infof("TunnoxServer: initializing server, node_id=%s, bind_addr=%s", config.NodeID, config.BindAddr)
	
	server := &TunnoxServer{
		ManagerBase: dispose.NewManager("TunnoxServer", ctx),
		config:      config,
	}
	
	utils.Infof("TunnoxServer: initialization completed successfully")
	return server, nil
}

// GetConfig 获取配置
func (s *TunnoxServer) GetConfig() *ServerConfig {
	return s.config
}

// SetSessionManager 设置 SessionManager
func (s *TunnoxServer) SetSessionManager(sessionMgr *session.SessionManager) {
	s.sessionManager = sessionMgr
}

// SetTunnelManager 设置 TunnelManager
func (s *TunnoxServer) SetTunnelManager(tunnelMgr *TunnelManager) {
	s.tunnelManager = tunnelMgr
}

// SetAuthManager 设置 AuthManager
func (s *TunnoxServer) SetAuthManager(authMgr *AuthManager) {
	s.authManager = authMgr
}

// InitializeHandlers 初始化处理器（连接各个组件）
func (s *TunnoxServer) InitializeHandlers() error {
	if s.sessionManager == nil {
		return fmt.Errorf("session manager not set")
	}
	
	// 设置隧道和认证处理器
	if s.tunnelManager != nil {
		s.sessionManager.SetTunnelHandler(s.tunnelManager)
	}
	
	if s.authManager != nil {
		s.sessionManager.SetAuthHandler(s.authManager)
	}
	
	utils.Infof("TunnoxServer: handlers initialized successfully")
	return nil
}

// GetSessionManager 获取 SessionManager
func (s *TunnoxServer) GetSessionManager() *session.SessionManager {
	return s.sessionManager
}

// GetTunnelManager 获取 TunnelManager
func (s *TunnoxServer) GetTunnelManager() *TunnelManager {
	return s.tunnelManager
}

// GetAuthManager 获取 AuthManager
func (s *TunnoxServer) GetAuthManager() *AuthManager {
	return s.authManager
}

// GetCloudControl 获取 CloudControl
func (s *TunnoxServer) GetCloudControl() *managers.CloudControl {
	return s.config.CloudControl
}

// GetNodeID 获取节点 ID
func (s *TunnoxServer) GetNodeID() string {
	return s.config.NodeID
}

// Stop 停止服务器
func (s *TunnoxServer) Stop() error {
	utils.Infof("TunnoxServer: stopping server")
	
	// 关闭所有组件
	if err := s.Close(); err != nil {
		utils.Errorf("TunnoxServer: error during shutdown: %v", err)
		return err
	}
	
	utils.Infof("TunnoxServer: server stopped successfully")
	return nil
}
