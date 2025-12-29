package managers

import (
	"context"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/dispose"
)

// SearchManager 搜索管理器
// 通过 Service 接口访问数据，遵循 Manager -> Service -> Repository 架构
type SearchManager struct {
	*dispose.ManagerBase
	userService       services.UserService
	clientService     services.ClientService
	portMappingServic services.PortMappingService
}

// NewSearchManager 创建新的搜索管理器
func NewSearchManager(userService services.UserService, clientService services.ClientService, portMappingService services.PortMappingService, parentCtx context.Context) *SearchManager {
	manager := &SearchManager{
		ManagerBase:       dispose.NewManager("SearchManager", parentCtx),
		userService:       userService,
		clientService:     clientService,
		portMappingServic: portMappingService,
	}
	return manager
}

// SearchUsers 搜索用户
func (sm *SearchManager) SearchUsers(keyword string) ([]*models.User, error) {
	return sm.userService.SearchUsers(keyword)
}

// SearchClients 搜索客户端
func (sm *SearchManager) SearchClients(keyword string) ([]*models.Client, error) {
	return sm.clientService.SearchClients(keyword)
}

// SearchPortMappings 搜索端口映射
func (sm *SearchManager) SearchPortMappings(keyword string) ([]*models.PortMapping, error) {
	return sm.portMappingServic.SearchPortMappings(keyword)
}
