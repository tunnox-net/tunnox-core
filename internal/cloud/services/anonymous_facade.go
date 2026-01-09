package services

import (
	"context"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/anonymous"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/security"
)

// anonymousServiceWrapper 包装 anonymous.Service 以实现 AnonymousService 接口
// 主要用于适配 SetNotifier 方法的接口类型差异
type anonymousServiceWrapper struct {
	*anonymous.Service
}

// SetNotifier 实现 AnonymousService 接口
// 将 services.ClientNotifier 适配到 anonymous.Notifier
func (w *anonymousServiceWrapper) SetNotifier(notifier ClientNotifier) {
	// anonymous.Notifier 和 ClientNotifier 有相同的方法签名
	// 可以直接传递，因为 Go 的接口是隐式实现的
	w.Service.SetNotifier(notifier)
}

// SetSecretKeyManager 实现 AnonymousService 接口
func (w *anonymousServiceWrapper) SetSecretKeyManager(mgr *security.SecretKeyManager) {
	w.Service.SetSecretKeyManager(mgr)
}

// GenerateAnonymousCredentials 委托到底层服务
func (w *anonymousServiceWrapper) GenerateAnonymousCredentials() (*models.Client, error) {
	return w.Service.GenerateAnonymousCredentials()
}

// GetAnonymousClient 委托到底层服务
func (w *anonymousServiceWrapper) GetAnonymousClient(clientID int64) (*models.Client, error) {
	return w.Service.GetAnonymousClient(clientID)
}

// DeleteAnonymousClient 委托到底层服务
func (w *anonymousServiceWrapper) DeleteAnonymousClient(clientID int64) error {
	return w.Service.DeleteAnonymousClient(clientID)
}

// ListAnonymousClients 委托到底层服务
func (w *anonymousServiceWrapper) ListAnonymousClients() ([]*models.Client, error) {
	return w.Service.ListAnonymousClients()
}

// CreateAnonymousMapping 委托到底层服务
func (w *anonymousServiceWrapper) CreateAnonymousMapping(listenClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) {
	return w.Service.CreateAnonymousMapping(listenClientID, targetClientID, protocol, sourcePort, targetPort)
}

// GetAnonymousMappings 委托到底层服务
func (w *anonymousServiceWrapper) GetAnonymousMappings() ([]*models.PortMapping, error) {
	return w.Service.GetAnonymousMappings()
}

// CleanupExpiredAnonymous 委托到底层服务
func (w *anonymousServiceWrapper) CleanupExpiredAnonymous() error {
	return w.Service.CleanupExpiredAnonymous()
}

// NewAnonymousService 创建匿名服务
// 返回包装后的服务以满足 AnonymousService 接口
func NewAnonymousService(clientRepo *repos.ClientRepository, configRepo *repos.ClientConfigRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, parentCtx context.Context) AnonymousService {
	return &anonymousServiceWrapper{
		Service: anonymous.NewService(clientRepo, configRepo, mappingRepo, idManager, parentCtx),
	}
}
