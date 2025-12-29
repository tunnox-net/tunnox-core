package managers

import (
	"context"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/dispose"
)

// AnonymousManager 匿名用户管理器
// 通过 AnonymousService 接口访问数据，遵循 Manager -> Service -> Repository 架构
type AnonymousManager struct {
	*dispose.ManagerBase
	anonymousService services.AnonymousService
}

// NewAnonymousManager 创建匿名用户管理器
func NewAnonymousManager(anonymousService services.AnonymousService, parentCtx context.Context) *AnonymousManager {
	manager := &AnonymousManager{
		ManagerBase:      dispose.NewManager("AnonymousManager", parentCtx),
		anonymousService: anonymousService,
	}
	return manager
}

// GenerateAnonymousCredentials 生成匿名客户端凭据
func (am *AnonymousManager) GenerateAnonymousCredentials() (*models.Client, error) {
	return am.anonymousService.GenerateAnonymousCredentials()
}

// GetAnonymousClient 获取匿名客户端
func (am *AnonymousManager) GetAnonymousClient(clientID int64) (*models.Client, error) {
	return am.anonymousService.GetAnonymousClient(clientID)
}

// ListAnonymousClients 列出所有匿名客户端
func (am *AnonymousManager) ListAnonymousClients() ([]*models.Client, error) {
	return am.anonymousService.ListAnonymousClients()
}

// DeleteAnonymousClient 删除匿名客户端
func (am *AnonymousManager) DeleteAnonymousClient(clientID int64) error {
	return am.anonymousService.DeleteAnonymousClient(clientID)
}

// CreateAnonymousMapping 创建匿名端口映射
func (am *AnonymousManager) CreateAnonymousMapping(listenClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) {
	return am.anonymousService.CreateAnonymousMapping(listenClientID, targetClientID, protocol, sourcePort, targetPort)
}

// GetAnonymousMappings 获取所有匿名端口映射
func (am *AnonymousManager) GetAnonymousMappings() ([]*models.PortMapping, error) {
	return am.anonymousService.GetAnonymousMappings()
}

// CleanupExpiredAnonymous 清理过期的匿名数据
func (am *AnonymousManager) CleanupExpiredAnonymous() error {
	return am.anonymousService.CleanupExpiredAnonymous()
}

// SetNotifier 设置通知器
func (am *AnonymousManager) SetNotifier(notifier interface{}) {
	am.anonymousService.SetNotifier(notifier)
}
