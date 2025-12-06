package services

import (
	"context"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
)

// clientService 客户端服务实现
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
type clientService struct {
	*dispose.ServiceBase
	baseService *BaseService

	// 新的Repository（分离存储）
	configRepo *repos.ClientConfigRepository
	stateRepo  *repos.ClientStateRepository
	tokenRepo  *repos.ClientTokenRepository

	// 保留的Repository（兼容性）
	clientRepo  *repos.ClientRepository // 旧版，逐步迁移
	mappingRepo *repos.PortMappingRepo

	// 其他依赖
	idManager    *idgen.IDManager
	statsMgr     *managers.StatsManager
	statsCounter *stats.StatsCounter
}

// NewClientService 创建客户端服务
//
// 参数：
//   - configRepo: 配置Repository
//   - stateRepo: 状态Repository
//   - tokenRepo: TokenRepository
//   - clientRepo: 旧版Repository（兼容性，逐步迁移）
//   - mappingRepo: 映射Repository
//   - idManager: ID管理器
//   - statsMgr: 统计管理器
//   - parentCtx: 父上下文
//
// 返回：
//   - ClientService: 客户端服务接口
func NewClientService(
	configRepo *repos.ClientConfigRepository,
	stateRepo *repos.ClientStateRepository,
	tokenRepo *repos.ClientTokenRepository,
	clientRepo *repos.ClientRepository,
	mappingRepo *repos.PortMappingRepo,
	idManager *idgen.IDManager,
	statsMgr *managers.StatsManager,
	parentCtx context.Context,
) ClientService {
	service := &clientService{
		ServiceBase:  dispose.NewService("ClientService", parentCtx),
		baseService:  NewBaseService(),
		configRepo:   configRepo,
		stateRepo:    stateRepo,
		tokenRepo:    tokenRepo,
		clientRepo:   clientRepo,
		mappingRepo:  mappingRepo,
		idManager:    idManager,
		statsMgr:     statsMgr,
		statsCounter: statsMgr.GetCounter(),
	}
	return service
}
