package client

import (
	"context"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
)

// Service 客户端服务实现
//
// 职责：
// - 聚合ClientConfig, ClientRuntimeState, ClientToken
// - 提供完整的客户端业务逻辑
// - 管理客户端连接状态
//
// 数据分离：
// - ClientConfig: 持久化配置（数据库+缓存）
// - ClientRuntimeState: 运行时状态（仅缓存，TTL=90秒）
// - ClientToken: JWT Token（仅缓存，自动过期）
type Service struct {
	*dispose.ServiceBase
	baseService *base.Service

	// 新的Repository（分离存储）
	configRepo *repos.ClientConfigRepository
	stateRepo  *repos.ClientStateRepository
	tokenRepo  *repos.ClientTokenRepository

	// 保留的Repository（兼容性）
	clientRepo  *repos.ClientRepository // 旧版，逐步迁移
	mappingRepo *repos.PortMappingRepo

	// 其他依赖
	idManager     *idgen.IDManager
	statsProvider base.StatsProvider
	statsCounter  *stats.StatsCounter

	// 消息代理（可选，用于发布客户端状态事件）
	broker broker.MessageBroker
}

// NewService 创建客户端服务
//
// 参数：
//   - configRepo: 配置Repository
//   - stateRepo: 状态Repository
//   - tokenRepo: TokenRepository
//   - clientRepo: 旧版Repository（兼容性，逐步迁移）
//   - mappingRepo: 映射Repository
//   - idManager: ID管理器
//   - statsProvider: 统计数据提供者（接口，由 managers.StatsManager 实现）
//   - parentCtx: 父上下文
//
// 返回：
//   - *Service: 客户端服务实例
func NewService(
	configRepo *repos.ClientConfigRepository,
	stateRepo *repos.ClientStateRepository,
	tokenRepo *repos.ClientTokenRepository,
	clientRepo *repos.ClientRepository,
	mappingRepo *repos.PortMappingRepo,
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
