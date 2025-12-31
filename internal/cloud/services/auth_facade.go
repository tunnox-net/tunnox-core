package services

import (
	"context"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/auth"
)

// authService 认证服务实现
// 向后兼容：别名到 auth.Service
type authService = auth.Service

// JWTTokenInfo JWT令牌信息
// 向后兼容：别名到 auth.JWTTokenInfo
type JWTTokenInfo = auth.JWTTokenInfo

// NewauthService 创建认证服务
// 向后兼容：委托到 auth.NewService
func NewauthService(clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository,
	jwtProvider JWTProvider, parentCtx context.Context) AuthService {
	return auth.NewService(clientRepo, nodeRepo, jwtProvider, parentCtx)
}
