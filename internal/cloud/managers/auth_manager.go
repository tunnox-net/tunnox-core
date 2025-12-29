package managers

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
)

// GenerateJWTToken 生成JWT令牌
func (c *CloudControl) GenerateJWTToken(clientID int64) (*JWTTokenInfo, error) {
	client, err := c.clientService.GetClient(clientID)
	if err != nil {
		return nil, err
	}
	return c.jwtManager.GenerateTokenPair(c.Ctx(), client)
}

// RefreshJWTToken 刷新JWT令牌
func (c *CloudControl) RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error) {
	// 验证刷新令牌
	claims, err := c.jwtManager.ValidateRefreshToken(c.Ctx(), refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// 获取客户端信息
	client, err := c.clientService.GetClient(claims.ClientID)
	if err != nil {
		return nil, err
	}

	// 生成新的令牌对
	return c.jwtManager.GenerateTokenPair(c.Ctx(), client)
}

// ValidateJWTToken 验证JWT令牌
func (c *CloudControl) ValidateJWTToken(token string) (*JWTTokenInfo, error) {
	claims, err := c.jwtManager.ValidateAccessToken(c.Ctx(), token)
	if err != nil {
		return nil, err
	}

	client, err := c.clientService.GetClient(claims.ClientID)
	if err != nil {
		return nil, err
	}

	return &JWTTokenInfo{
		Token:    token,
		ClientId: client.ID,
		TokenID:  claims.ID,
	}, nil
}

// RevokeJWTToken 撤销JWT令牌
func (c *CloudControl) RevokeJWTToken(token string) error {
	// 验证令牌以获取客户端ID
	claims, err := c.jwtManager.ValidateAccessToken(c.Ctx(), token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// 将令牌加入黑名单
	return c.jwtManager.RevokeToken(c.Ctx(), claims.ID)
}

// Authenticate 认证客户端
func (c *CloudControl) Authenticate(req *models.AuthRequest) (*models.AuthResponse, error) {
	// 获取客户端信息
	client, err := c.clientService.GetClient(req.ClientID)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	if client == nil {
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

	// 更新客户端状态
	client.Status = models.ClientStatusOnline
	client.NodeID = req.NodeID
	client.IPAddress = req.IPAddress
	client.Version = req.Version
	now := time.Now()
	client.LastSeen = &now
	client.UpdatedAt = now

	if err := c.clientService.UpdateClient(client); err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Failed to update client status",
		}, nil
	}

	// 生成JWT令牌
	tokenInfo, err := c.jwtManager.GenerateTokenPair(c.Ctx(), client)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Failed to generate token",
		}, nil
	}

	// 获取节点信息
	nodeInfo, _ := c.nodeService.GetNodeServiceInfo(req.NodeID)
	var node *models.Node
	if nodeInfo != nil {
		node = &models.Node{
			ID:      nodeInfo.NodeID,
			Address: nodeInfo.Address,
		}
	}

	return &models.AuthResponse{
		Success:   true,
		Token:     tokenInfo.Token,
		Client:    client,
		Node:      node,
		ExpiresAt: tokenInfo.ExpiresAt,
		Message:   "Authentication successful",
	}, nil
}

// ValidateToken 验证令牌
func (c *CloudControl) ValidateToken(token string) (*models.AuthResponse, error) {
	// 验证JWT令牌
	claims, err := c.jwtManager.ValidateAccessToken(c.Ctx(), token)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	// 获取客户端信息
	client, err := c.clientService.GetClient(claims.ClientID)
	if err != nil {
		return nil, err
	}

	if client == nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 获取节点信息
	var node *models.Node
	if client.NodeID != "" {
		nodeInfo, _ := c.nodeService.GetNodeServiceInfo(client.NodeID)
		if nodeInfo != nil {
			node = &models.Node{
				ID:      nodeInfo.NodeID,
				Address: nodeInfo.Address,
			}
		}
	}

	return &models.AuthResponse{
		Success:   true,
		Token:     token,
		Client:    client,
		Node:      node,
		ExpiresAt: claims.ExpiresAt.Time,
		Message:   "Token validated successfully",
	}, nil
}
