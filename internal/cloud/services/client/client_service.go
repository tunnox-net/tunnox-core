package client

import (
	"context"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/security"
)

// WebhookNotifier webhook 通知接口
// 注意：方法签名必须与 services.WebhookNotifier 一致，以便 client.Service
// 能够隐式实现 services.WebhookNotifierAware 接口
type WebhookNotifier interface {
	DispatchClientOnline(clientID int64, userID, ipAddress, nodeID string)
	DispatchClientOffline(clientID int64, userID string)
}

type Service struct {
	*dispose.ServiceBase
	baseService *base.Service

	configRepo repos.IClientConfigRepository
	stateRepo  repos.IClientStateRepository
	tokenRepo  repos.IClientTokenRepository

	clientRepo  repos.IClientRepository
	mappingRepo repos.IPortMappingRepository

	idManager     *idgen.IDManager
	statsProvider base.StatsProvider
	statsCounter  *stats.StatsCounter

	broker          broker.MessageBroker
	webhookNotifier WebhookNotifier
	secretKeyMgr    *security.SecretKeyManager
}

func NewService(
	configRepo repos.IClientConfigRepository,
	stateRepo repos.IClientStateRepository,
	tokenRepo repos.IClientTokenRepository,
	clientRepo repos.IClientRepository,
	mappingRepo repos.IPortMappingRepository,
	idManager *idgen.IDManager,
	statsProvider base.StatsProvider,
	parentCtx context.Context,
) *Service {
	service := &Service{
		ServiceBase:   dispose.NewService("ClientService", parentCtx),
		baseService:   base.NewService(),
		configRepo:    configRepo,
		stateRepo:     stateRepo,
		tokenRepo:     tokenRepo,
		clientRepo:    clientRepo,
		mappingRepo:   mappingRepo,
		idManager:     idManager,
		statsProvider: statsProvider,
		statsCounter:  statsProvider.GetCounter(),
	}
	return service
}

// SetBroker 设置消息代理（用于发布客户端状态事件）
//
// 参数：
//   - b: 消息代理实例
func (s *Service) SetBroker(b broker.MessageBroker) {
	s.broker = b
}

func (s *Service) SetWebhookNotifier(n WebhookNotifier) {
	s.webhookNotifier = n
}

// SetSecretKeyManager 设置 SecretKey 管理器（用于加密存储凭据）
func (s *Service) SetSecretKeyManager(mgr *security.SecretKeyManager) {
	s.secretKeyMgr = mgr
}
