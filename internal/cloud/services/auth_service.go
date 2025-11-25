package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// authService 认证服务实现
type authService struct {
	*dispose.ServiceBase
	clientRepo *repos.ClientRepository
	nodeRepo   *repos.NodeRepository
	jwtManager *managers.JWTManager
}

// NewauthService 创建新的认证服务实现
func NewauthService(clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository,
	jwtManager *managers.JWTManager, parentCtx context.Context) *authService {
	service := &authService{
		ServiceBase: dispose.NewService("authService", parentCtx),
		clientRepo:  clientRepo,
		nodeRepo:    nodeRepo,
		jwtManager:  jwtManager,
	}
	return service
}

// Authenticate 认证客户端
func (s *authService) Authenticate(req *models.AuthRequest) (*models.AuthResponse, error) {
	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(req.ClientID))
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 验证认证码
	if client.AuthCode != req.AuthCode {
		return &models.AuthResponse{
			Success: false,
			Message: "Invalid auth code",
		}, nil
	}

	// 验证密钥（如果提供）
	if req.SecretKey != "" && client.SecretKey != req.SecretKey {
		return &models.AuthResponse{
			Success: false,
			Message: "Invalid secret key",
		}, nil
	}

	// 获取节点信息
	var node *models.Node
	if req.NodeID != "" {
		node, err = s.nodeRepo.GetNode(req.NodeID)
		if err != nil {
			utils.Warnf("Failed to get node %s: %v", req.NodeID, err)
		}
	}

	// 更新客户端状态
	now := time.Now()
	client.Status = models.ClientStatusOnline
	client.LastSeen = &now
	client.NodeID = req.NodeID
	client.IPAddress = req.IPAddress
	client.Version = req.Version
	client.UpdatedAt = now

	if err := s.clientRepo.UpdateClient(client); err != nil {
		utils.Warnf("Failed to update client status: %v", err)
	}

	// 生成JWT令牌
	jwtToken, err := s.jwtManager.GenerateTokenPair(s.Ctx(), client)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Failed to generate token",
		}, nil
	}

	// 更新客户端的JWT信息
	client.JWTToken = jwtToken.Token
	client.TokenExpiresAt = &jwtToken.ExpiresAt
	client.RefreshToken = jwtToken.RefreshToken
	client.TokenID = jwtToken.TokenID

	if err := s.clientRepo.UpdateClient(client); err != nil {
		utils.Warnf("Failed to update client JWT info: %v", err)
	}

	utils.Infof("Client %d authenticated successfully", req.ClientID)

	return &models.AuthResponse{
		Success:   true,
		Token:     jwtToken.Token,
		Client:    client,
		Node:      node,
		ExpiresAt: jwtToken.ExpiresAt,
		Message:   "Authentication successful",
	}, nil
}

// ValidateToken 验证令牌
func (s *authService) ValidateToken(token string) (*models.AuthResponse, error) {
	// 使用JWT管理器验证令牌
	jwtClaims, err := s.jwtManager.ValidateAccessToken(s.Ctx(), token)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(jwtClaims.ClientID))
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 检查令牌是否匹配
	if client.JWTToken != token {
		return &models.AuthResponse{
			Success: false,
			Message: "Token mismatch",
		}, nil
	}

	// 检查令牌是否过期
	if client.TokenExpiresAt != nil && time.Now().After(*client.TokenExpiresAt) {
		return &models.AuthResponse{
			Success: false,
			Message: "Token expired",
		}, nil
	}

	utils.Debugf("Token validated successfully for client %d", client.ID)

	return &models.AuthResponse{
		Success:   true,
		Token:     token,
		Client:    client,
		ExpiresAt: *client.TokenExpiresAt,
		Message:   "Token valid",
	}, nil
}

// GenerateJWTToken 生成JWT令牌
func (s *authService) GenerateJWTToken(clientID int64) (*JWTTokenInfo, error) {
	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	jwtToken, err := s.jwtManager.GenerateTokenPair(s.Ctx(), client)
	if err != nil {
		return nil, err
	}

	return &JWTTokenInfo{
		AccessToken:  jwtToken.Token,
		RefreshToken: jwtToken.RefreshToken,
		ExpiresAt:    jwtToken.ExpiresAt,
		TokenType:    "Bearer",
		ClientID:     jwtToken.ClientId,
	}, nil
}

// RefreshJWTToken 刷新JWT令牌
func (s *authService) RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error) {
	// 验证刷新令牌
	refreshClaims, err := s.jwtManager.ValidateRefreshToken(s.Ctx(), refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(refreshClaims.ClientID))
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	jwtToken, err := s.jwtManager.RefreshAccessToken(s.Ctx(), refreshToken, client)
	if err != nil {
		return nil, err
	}

	return &JWTTokenInfo{
		AccessToken:  jwtToken.Token,
		RefreshToken: jwtToken.RefreshToken,
		ExpiresAt:    jwtToken.ExpiresAt,
		TokenType:    "Bearer",
		ClientID:     jwtToken.ClientId,
	}, nil
}

// ValidateJWTToken 验证JWT令牌
func (s *authService) ValidateJWTToken(token string) (*JWTTokenInfo, error) {
	// 验证访问令牌
	claims, err := s.jwtManager.ValidateAccessToken(s.Ctx(), token)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(claims.ClientID))
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	return &JWTTokenInfo{
		AccessToken:  token,
		RefreshToken: client.RefreshToken,
		ExpiresAt:    *client.TokenExpiresAt,
		TokenType:    "Bearer",
		ClientID:     client.ID,
	}, nil
}

// RevokeJWTToken 撤销JWT令牌
func (s *authService) RevokeJWTToken(token string) error {
	// 验证令牌以获取TokenID
	claims, err := s.jwtManager.ValidateAccessToken(s.Ctx(), token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// 从客户端信息中获取TokenID
	client, err := s.clientRepo.GetClient(utils.Int64ToString(claims.ClientID))
	if err != nil {
		return fmt.Errorf("client not found: %w", err)
	}

	return s.jwtManager.RevokeToken(s.Ctx(), client.TokenID)
}
