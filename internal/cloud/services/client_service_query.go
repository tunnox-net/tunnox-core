package services

import (
	"fmt"
	"strings"
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
		configs, err = s.configRepo.ListUserConfigs(userID)
		if err != nil {
			return nil, fmt.Errorf("failed to list user configs: %w", err)
		}
	} else {

		// 获取所有客户端配置
		configs, err = s.configRepo.ListConfigs()
		if err != nil {
			return nil, fmt.Errorf("failed to list client configs: %w", err)
		}
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
	// 使用ConfigRepo查询用户配置
	configs, err := s.configRepo.ListUserConfigs(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user configs: %w", err)
	}

	// 并发聚合每个客户端的完整信息
	clients := make([]*models.Client, 0, len(configs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cfg := range configs {
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
	// 获取所有客户端配置
	configs, err := s.configRepo.ListConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to list client configs for search: %w", err)
	}

	// 如果关键词为空，返回空列表
	if keyword == "" {
		return []*models.Client{}, nil
	}

	// 大小写不敏感搜索
	keyword = strings.ToLower(keyword)

	// 并发聚合匹配的客户端
	matchedClients := make([]*models.Client, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cfg := range configs {
		// 检查名称是否匹配（不区分大小写）
		if !strings.Contains(strings.ToLower(cfg.Name), keyword) &&
			!strings.Contains(strings.ToLower(fmt.Sprintf("%d", cfg.ID)), keyword) {
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
			matchedClients = append(matchedClients, client)
			mu.Unlock()
		}(cfg)
	}

	wg.Wait()
	return matchedClients, nil
}

// GetClientStats 获取客户端统计信息
func (s *clientService) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	if s.statsProvider == nil {
		return nil, fmt.Errorf("stats provider not available")
	}

	clientStats, err := s.statsProvider.GetClientStats(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client stats for %d: %w", clientID, err)
	}
	return clientStats, nil
}
