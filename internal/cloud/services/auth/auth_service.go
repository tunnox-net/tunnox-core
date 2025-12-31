package auth

import (
	"context"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"
)

// JWTTokenInfo JWT令牌信息
type JWTTokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	TokenType    string
	ClientID     int64
}

// Service 认证服务实现
type Service struct {
	*dispose.ServiceBase
	clientRepo  *repos.ClientRepository
	nodeRepo    *repos.NodeRepository
	jwtProvider base.JWTProvider
}

// NewService 创建新的认证服务实现
func NewService(clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository,
	jwtProvider base.JWTProvider, parentCtx context.Context) *Service {
	service := &Service{
		ServiceBase: dispose.NewService("authService", parentCtx),
		clientRepo:  clientRepo,
		nodeRepo:    nodeRepo,
		jwtProvider: jwtProvider,
	}
	return service
}

// Authenticate 认证客户端
func (s *Service) Authenticate(req *models.AuthRequest) (*models.AuthResponse, error) {
	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(req.ClientID))
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 验证认证码
	// 对于匿名客户端，Token可能是SecretKey（因为客户端没有AuthCode）
	isValid := false
	if client.AuthCode == req.AuthCode {
		isValid = true
	} else if client.Type == models.ClientTypeAnonymous && client.SecretKey == req.AuthCode {
		isValid = true
	}

	if !isValid {
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
			corelog.Warnf("Failed to get node %s: %v", req.NodeID, err)
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
		corelog.Warnf("Failed to update client status: %v", err)
	}

	// 生成JWT令牌
	jwtToken, err := s.jwtProvider.GenerateTokenPair(s.Ctx(), client)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Failed to generate token",
		}, nil
	}

	// 更新客户端的JWT信息
	tokenExpiresAt := jwtToken.GetExpiresAt()
	client.JWTToken = jwtToken.GetToken()
	client.TokenExpiresAt = &tokenExpiresAt
	client.RefreshToken = jwtToken.GetRefreshToken()
	client.TokenID = jwtToken.GetTokenID()

	if err := s.clientRepo.UpdateClient(client); err != nil {
		corelog.Warnf("Failed to update client JWT info: %v", err)
	}

	corelog.Infof("Client %d authenticated successfully", req.ClientID)

	return &models.AuthResponse{
		Success:   true,
		Token:     jwtToken.GetToken(),
		Client:    client,
		Node:      node,
		ExpiresAt: jwtToken.GetExpiresAt(),
		Message:   "Authentication successful",
	}, nil
}

// ValidateToken 验证令牌
func (s *Service) ValidateToken(token string) (*models.AuthResponse, error) {
	// 使用JWT管理器验证令牌
	jwtClaims, err := s.jwtProvider.ValidateAccessToken(s.Ctx(), token)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(jwtClaims.GetClientID()))
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

	corelog.Debugf("Token validated successfully for client %d", client.ID)

	return &models.AuthResponse{
		Success:   true,
		Token:     token,
		Client:    client,
		ExpiresAt: *client.TokenExpiresAt,
		Message:   "Token valid",
	}, nil
}

// GenerateJWTToken 生成JWT令牌
func (s *Service) GenerateJWTToken(clientID int64) (*JWTTokenInfo, error) {
	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeClientNotFound, "client not found")
	}

	jwtToken, err := s.jwtProvider.GenerateTokenPair(s.Ctx(), client)
	if err != nil {
		return nil, err
	}

	return &JWTTokenInfo{
		AccessToken:  jwtToken.GetToken(),
		RefreshToken: jwtToken.GetRefreshToken(),
		ExpiresAt:    jwtToken.GetExpiresAt(),
		TokenType:    "Bearer",
		ClientID:     jwtToken.GetClientId(),
	}, nil
}

// RefreshJWTToken 刷新JWT令牌
func (s *Service) RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error) {
	// 验证刷新令牌
	refreshClaims, err := s.jwtProvider.ValidateRefreshToken(s.Ctx(), refreshToken)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidToken, "invalid refresh token")
	}

	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(refreshClaims.GetClientID()))
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeClientNotFound, "client not found")
	}

	jwtToken, err := s.jwtProvider.RefreshAccessToken(s.Ctx(), refreshToken, client)
	if err != nil {
		return nil, err
	}

	return &JWTTokenInfo{
		AccessToken:  jwtToken.GetToken(),
		RefreshToken: jwtToken.GetRefreshToken(),
		ExpiresAt:    jwtToken.GetExpiresAt(),
		TokenType:    "Bearer",
		ClientID:     jwtToken.GetClientId(),
	}, nil
}

// ValidateJWTToken 验证JWT令牌
func (s *Service) ValidateJWTToken(token string) (*JWTTokenInfo, error) {
	// 验证访问令牌
	claims, err := s.jwtProvider.ValidateAccessToken(s.Ctx(), token)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidToken, "invalid token")
	}

	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(claims.GetClientID()))
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeClientNotFound, "client not found")
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
func (s *Service) RevokeJWTToken(token string) error {
	// 验证令牌以获取TokenID
	claims, err := s.jwtProvider.ValidateAccessToken(s.Ctx(), token)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInvalidToken, "invalid token")
	}

	// 从客户端信息中获取TokenID
	client, err := s.clientRepo.GetClient(utils.Int64ToString(claims.GetClientID()))
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeClientNotFound, "client not found")
	}

	return s.jwtProvider.RevokeToken(s.Ctx(), client.TokenID)
}
