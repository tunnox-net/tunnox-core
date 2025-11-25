package server

import (
	"context"
	"errors"
	"fmt"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// AuthManager 认证管理器（处理指令连接的握手认证）
type AuthManager struct {
	*dispose.ManagerBase
	
	cloudControl   *managers.CloudControl
	sessionManager *session.SessionManager
}

// NewAuthManager 创建认证管理器
func NewAuthManager(ctx context.Context, cloudControl *managers.CloudControl, sessionMgr *session.SessionManager) *AuthManager {
	return &AuthManager{
		ManagerBase:    dispose.NewManager("AuthManager", ctx),
		cloudControl:   cloudControl,
		sessionManager: sessionMgr,
	}
}

// HandleHandshake 处理指令连接的握手认证（支持注册客户端和匿名客户端）
// 注意：此方法只处理指令连接（Control Connection）的认证
// 映射连接（Tunnel Connection）的认证由 TunnelManager.HandleTunnelOpen 处理
func (am *AuthManager) HandleHandshake(conn *session.ClientConnection, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
	// 判断是否为匿名认证（ClientID == 0 或 Token 以 "anonymous:" 开头）
	isAnonymous := req.ClientID == 0 || (len(req.Token) > 10 && req.Token[:10] == "anonymous:")
	
	if isAnonymous {
		return am.handleAnonymousAuth(conn, req)
	}
	
	return am.handleRegisteredAuth(conn, req)
}

// handleAnonymousAuth 处理匿名认证
func (am *AuthManager) handleAnonymousAuth(conn *session.ClientConnection, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
	utils.Infof("AuthManager: handling anonymous handshake, protocol=%s", req.Protocol)
	
	// 1. 调用 CloudControl 生成匿名客户端凭据
	client, err := am.cloudControl.GenerateAnonymousCredentials()
	if err != nil {
		utils.Errorf("AuthManager: failed to generate anonymous credentials, error=%v", err)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "failed to create anonymous client",
		}, fmt.Errorf("failed to generate anonymous credentials: %w", err)
	}
	
	utils.Infof("AuthManager: anonymous client created, client_id=%d", client.ID)
	
	// 2. 更新 SessionManager 中的指令连接认证状态
	if err := am.sessionManager.UpdateControlConnectionAuth(conn.ConnID, client.ID, ""); err != nil {
		utils.Errorf("AuthManager: failed to update control connection auth, error=%v", err)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "failed to update connection auth",
		}, err
	}
	
	// 3. 踢掉旧的指令连接（如果有）
	am.sessionManager.KickOldControlConnection(client.ID, conn.ConnID)
	
	utils.Infof("AuthManager: anonymous control connection authenticated successfully, client_id=%d, conn_id=%s",
		client.ID, conn.ConnID)
	
	// 4. 返回成功响应（包含分配的 ClientID）
	return &packet.HandshakeResponse{
		Success: true,
		Message: fmt.Sprintf("Anonymous client authenticated, client_id=%d", client.ID),
	}, nil
}

// handleRegisteredAuth 处理注册客户端认证
func (am *AuthManager) handleRegisteredAuth(conn *session.ClientConnection, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
	utils.Infof("AuthManager: handling registered handshake, client_id=%d, protocol=%s", req.ClientID, req.Protocol)
	
	// 1. 验证 JWT Token
	authResp, err := am.cloudControl.ValidateToken(req.Token)
	if err != nil || !authResp.Success {
		utils.Warnf("AuthManager: invalid token, client_id=%d, error=%v", req.ClientID, err)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "invalid token",
		}, fmt.Errorf("invalid token: %w", err)
	}
	
	// 2. 验证 Client 信息
	if authResp.Client == nil {
		utils.Warnf("AuthManager: no client info in token response, client_id=%d", req.ClientID)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "no client info in token",
		}, errors.New("no client info in token")
	}
	
	// 3. 验证 ClientID 是否匹配
	if authResp.Client.ID != req.ClientID {
		utils.Warnf("AuthManager: client ID mismatch, token_client=%d, req_client=%d", authResp.Client.ID, req.ClientID)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "client ID mismatch",
		}, errors.New("client ID mismatch")
	}
	
	// 4. 验证 Client 状态
	if authResp.Client.Status != "active" {
		utils.Warnf("AuthManager: client inactive, client_id=%d, status=%s", req.ClientID, authResp.Client.Status)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "client not active",
		}, errors.New("client not active")
	}
	
	// 5. 更新 SessionManager 中的指令连接认证状态
	if err := am.sessionManager.UpdateControlConnectionAuth(conn.ConnID, req.ClientID, authResp.Client.UserID); err != nil {
		utils.Errorf("AuthManager: failed to update control connection auth, error=%v", err)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "failed to update connection auth",
		}, err
	}
	
	// 6. 踢掉旧的指令连接（同一 ClientID 只能有 1 条指令连接）
	am.sessionManager.KickOldControlConnection(req.ClientID, conn.ConnID)
	
	utils.Infof("AuthManager: registered control connection authenticated successfully, client_id=%d, user_id=%s, conn_id=%s",
		req.ClientID, authResp.Client.UserID, conn.ConnID)
	
	// 7. 返回成功响应
	return &packet.HandshakeResponse{
		Success: true,
		Message: "Control connection authenticated successfully",
	}, nil
}

