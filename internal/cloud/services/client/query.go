package client

import (
	"fmt"
	"strings"
	"sync"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/utils/random"
)

// ============================================================================
// 兼容性方法（保持接口一致）
// ============================================================================

// ListClients 列出客户端
func (s *Service) ListClients(userID string, clientType models.ClientType) ([]*models.Client, error) {
	var configs []*models.ClientConfig
	var err error

	if userID != "" {
		// 获取用户的客户端配置
		configs, err = s.configRepo.ListUserConfigs(userID)
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to list user configs")
		}
	} else {

		// 获取所有客户端配置
		configs, err = s.configRepo.ListConfigs()
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to list client configs")
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
			// 错误可安全忽略：列表查询场景中单个客户端的状态/token获取失败不应影响整体查询
			// FromConfigAndState 可以正确处理 nil 值
			var state *models.ClientRuntimeState
			var token *models.ClientToken

			state, _ = s.stateRepo.GetState(config.ID)  // 忽略错误：状态可能不存在
			token, _ = s.tokenRepo.GetToken(config.ID)  // 忽略错误：token可能不存在

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
func (s *Service) ListUserClients(userID string) ([]*models.Client, error) {
	// 使用ConfigRepo查询用户配置
	configs, err := s.configRepo.ListUserConfigs(userID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to list user configs")
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
			// 错误可安全忽略：列表查询场景中单个客户端的状态/token获取失败不应影响整体查询
			// FromConfigAndState 可以正确处理 nil 值
			var state *models.ClientRuntimeState
			var token *models.ClientToken

			state, _ = s.stateRepo.GetState(config.ID)  // 忽略错误：状态可能不存在
			token, _ = s.tokenRepo.GetToken(config.ID)  // 忽略错误：token可能不存在

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
func (s *Service) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	mappings, err := s.mappingRepo.GetClientPortMappings(random.Int64ToString(clientID))
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeStorageError, "failed to get client port mappings for %d", clientID)
	}
	return mappings, nil
}

// SearchClients 搜索客户端
func (s *Service) SearchClients(keyword string) ([]*models.Client, error) {
	// 获取所有客户端配置
	configs, err := s.configRepo.ListConfigs()
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to list client configs for search")
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
			// 错误可安全忽略：列表查询场景中单个客户端的状态/token获取失败不应影响整体查询
			// FromConfigAndState 可以正确处理 nil 值
			var state *models.ClientRuntimeState
			var token *models.ClientToken

			state, _ = s.stateRepo.GetState(config.ID)  // 忽略错误：状态可能不存在
			token, _ = s.tokenRepo.GetToken(config.ID)  // 忽略错误：token可能不存在

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
func (s *Service) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	if s.statsProvider == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "stats provider not available")
	}

	clientStats, err := s.statsProvider.GetClientStats(clientID)
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeStorageError, "failed to get client stats for %d", clientID)
	}
	return clientStats, nil
}
