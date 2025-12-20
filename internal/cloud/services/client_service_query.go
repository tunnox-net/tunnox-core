package services

import (
corelog "tunnox-core/internal/core/log"
	"fmt"
	"sync"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/utils"
)

// ============================================================================
// 兼容性方法（保持接口一致）
// ============================================================================

// ListClients 列出客户端
func (s *clientService) ListClients(userID string, clientType models.ClientType) ([]*models.Client, error) {
	var configs []*models.ClientConfig
	var err error

	if userID != "" {
		// 获取用户的客户端配置
		// TODO: 需要实现ConfigRepo的ListUserConfigs方法
		// 暂时fallback到旧逻辑
		return s.ListUserClients(userID)
	}

	// 获取所有客户端配置
	configs, err = s.configRepo.ListConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to list client configs: %w", err)
	}

	// 并发聚合每个客户端的完整信息
	clients := make([]*models.Client, 0, len(configs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cfg := range configs {
		// 类型过滤
		if clientType != "" && cfg.Type != clientType {
			continue
		}

		wg.Add(1)
		go func(config *models.ClientConfig) {
			defer wg.Done()

			// 获取状态和Token
			var state *models.ClientRuntimeState
			var token *models.ClientToken

			state, _ = s.stateRepo.GetState(config.ID)
			token, _ = s.tokenRepo.GetToken(config.ID)

			// 聚合
			client := models.FromConfigAndState(config, state, token)

			mu.Lock()
			clients = append(clients, client)
			mu.Unlock()
		}(cfg)
	}

	wg.Wait()
	return clients, nil
}

// ListUserClients 列出用户的所有客户端
func (s *clientService) ListUserClients(userID string) ([]*models.Client, error) {
	// TODO: 实现基于ConfigRepo的查询
	// 暂时使用旧Repository
	clients, err := s.clientRepo.ListUserClients(userID)
	if err != nil {
		return nil, err
	}
	return clients, nil
}

// GetClientPortMappings 获取客户端的端口映射
func (s *clientService) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	mappings, err := s.mappingRepo.GetClientPortMappings(utils.Int64ToString(clientID))
	if err != nil {
		return nil, fmt.Errorf("failed to get client port mappings for %d: %w", clientID, err)
	}
	return mappings, nil
}

// SearchClients 搜索客户端
func (s *clientService) SearchClients(keyword string) ([]*models.Client, error) {
	// 暂时返回空列表
	corelog.Warnf("SearchClients not implemented yet")
	return []*models.Client{}, nil
}

// GetClientStats 获取客户端统计信息
func (s *clientService) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	if s.statsMgr == nil {
		return nil, fmt.Errorf("stats manager not available")
	}

	clientStats, err := s.statsMgr.GetClientStats(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client stats for %d: %w", clientID, err)
	}
	return clientStats, nil
}

