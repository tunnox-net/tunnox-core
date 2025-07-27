package managers

import (
	"context"
	"fmt"
	"strings"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
)

// SearchManager 搜索管理器
type SearchManager struct {
	*dispose.ManagerBase
	userRepo    *repos.UserRepository
	clientRepo  *repos.ClientRepository
	mappingRepo *repos.PortMappingRepo
}

// NewSearchManager 创建新的搜索管理器
func NewSearchManager(userRepo *repos.UserRepository, clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, parentCtx context.Context) *SearchManager {
	manager := &SearchManager{
		ManagerBase: dispose.NewManager("SearchManager", parentCtx),
		userRepo:    userRepo,
		clientRepo:  clientRepo,
		mappingRepo: mappingRepo,
	}
	return manager
}

// SearchUsers 搜索用户
func (sm *SearchManager) SearchUsers(keyword string) ([]*models.User, error) {
	users, err := sm.userRepo.ListUsers("")
	if err != nil {
		return nil, err
	}

	var results []*models.User
	for _, user := range users {
		if strings.Contains(strings.ToLower(user.Username), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(user.Email), strings.ToLower(keyword)) {
			results = append(results, user)
		}
	}

	return results, nil
}

// SearchClients 搜索客户端
func (sm *SearchManager) SearchClients(keyword string) ([]*models.Client, error) {
	clients, err := sm.clientRepo.ListUserClients("")
	if err != nil {
		return nil, err
	}

	var results []*models.Client
	for _, client := range clients {
		if strings.Contains(strings.ToLower(client.Name), strings.ToLower(keyword)) ||
			strings.Contains(client.AuthCode, keyword) ||
			strings.Contains(fmt.Sprintf("%d", client.ID), keyword) {
			results = append(results, client)
		}
	}

	return results, nil
}

// SearchPortMappings 搜索端口映射
func (sm *SearchManager) SearchPortMappings(keyword string) ([]*models.PortMapping, error) {
	mappings, err := sm.mappingRepo.GetUserPortMappings("")
	if err != nil {
		return nil, err
	}

	var results []*models.PortMapping
	for _, mapping := range mappings {
		if strings.Contains(mapping.ID, keyword) ||
			strings.Contains(fmt.Sprintf("%d", mapping.SourceClientID), keyword) ||
			strings.Contains(fmt.Sprintf("%d", mapping.TargetClientID), keyword) ||
			strings.Contains(string(mapping.Protocol), strings.ToLower(keyword)) {
			results = append(results, mapping)
		}
	}

	return results, nil
}
