package cloud

import (
	"strings"
	"tunnox-core/internal/utils"
)

// SearchManager 搜索管理服务
type SearchManager struct {
	userRepo    *UserRepository
	clientRepo  *ClientRepository
	mappingRepo *PortMappingRepo
	utils.Dispose
}

// NewSearchManager 创建搜索管理服务
func NewSearchManager(userRepo *UserRepository, clientRepo *ClientRepository, mappingRepo *PortMappingRepo) *SearchManager {
	manager := &SearchManager{
		userRepo:    userRepo,
		clientRepo:  clientRepo,
		mappingRepo: mappingRepo,
	}
	manager.SetCtx(nil, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (sm *SearchManager) onClose() {
	utils.Infof("Search manager resources cleaned up")
}

// SearchUsers 搜索用户
func (sm *SearchManager) SearchUsers(keyword string) ([]*User, error) {
	users, err := sm.userRepo.ListUsers("")
	if err != nil {
		return nil, err
	}

	var results []*User
	for _, user := range users {
		if strings.Contains(strings.ToLower(user.Username), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(user.Email), strings.ToLower(keyword)) {
			results = append(results, user)
		}
	}

	return results, nil
}

// SearchClients 搜索客户端
func (sm *SearchManager) SearchClients(keyword string) ([]*Client, error) {
	clients, err := sm.clientRepo.ListUserClients("")
	if err != nil {
		return nil, err
	}

	var results []*Client
	for _, client := range clients {
		if strings.Contains(strings.ToLower(client.Name), strings.ToLower(keyword)) ||
			strings.Contains(client.AuthCode, keyword) ||
			strings.Contains(utils.Int64ToString(client.ID), keyword) {
			results = append(results, client)
		}
	}

	return results, nil
}

// SearchPortMappings 搜索端口映射
func (sm *SearchManager) SearchPortMappings(keyword string) ([]*PortMapping, error) {
	mappings, err := sm.mappingRepo.GetUserPortMappings("")
	if err != nil {
		return nil, err
	}

	var results []*PortMapping
	for _, mapping := range mappings {
		if strings.Contains(mapping.ID, keyword) ||
			strings.Contains(utils.Int64ToString(mapping.SourceClientID), keyword) ||
			strings.Contains(utils.Int64ToString(mapping.TargetClientID), keyword) ||
			strings.Contains(string(mapping.Protocol), strings.ToLower(keyword)) {
			results = append(results, mapping)
		}
	}

	return results, nil
}
