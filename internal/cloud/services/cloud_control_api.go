package services

import (
	"context"
	"fmt"
	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/utils"
)

// CloudControlAPI 重构后的云控API，使用依赖注入容器
type CloudControlAPI struct {
	container *container.Container

	// 各个业务服务
	userService       UserService
	clientService     ClientService
	mappingService    PortMappingService
	nodeService       NodeService
	authService       AuthService
	anonymousService  AnonymousService
	connectionService ConnectionService
	statsService      StatsService

	utils.Dispose
}

// NewCloudControlAPI 创建新的云控API
func NewCloudControlAPI(config *managers.ControlConfig, storage interface{}, parentCtx context.Context) (*CloudControlAPI, error) {
	// 创建依赖注入容器
	container := container.NewContainer(parentCtx)

	// 注册基础设施服务
	if err := registerInfrastructureServices(container, config, storage, parentCtx); err != nil {
		return nil, fmt.Errorf("failed to register infrastructure services: %w", err)
	}

	// 注册业务服务
	if err := registerBusinessServices(container, parentCtx); err != nil {
		return nil, fmt.Errorf("failed to register business services: %w", err)
	}

	// 创建API实例
	api := &CloudControlAPI{
		container: container,
	}

	// 解析各个服务
	if err := api.resolveServices(); err != nil {
		return nil, fmt.Errorf("failed to resolve services: %w", err)
	}

	api.SetCtx(parentCtx, api.onClose)
	return api, nil
}

// onClose 资源清理回调
func (api *CloudControlAPI) onClose() error {
	utils.Infof("Cleaning up CloudControlAPI resources...")

	// 关闭容器（会自动关闭所有注册的服务）
	if err := api.container.Close(); err != nil {
		utils.Errorf("Failed to close container: %v", err)
	}

	utils.Infof("CloudControlAPI resources cleanup completed")
	return nil
}

// resolveServices 解析各个服务
func (api *CloudControlAPI) resolveServices() error {
	// 解析用户服务
	if err := api.container.ResolveTyped("user_service", &api.userService); err != nil {
		return fmt.Errorf("failed to resolve user service: %w", err)
	}

	// 解析客户端服务
	if err := api.container.ResolveTyped("client_service", &api.clientService); err != nil {
		return fmt.Errorf("failed to resolve client service: %w", err)
	}

	// 解析端口映射服务
	if err := api.container.ResolveTyped("mapping_service", &api.mappingService); err != nil {
		return fmt.Errorf("failed to resolve mapping service: %w", err)
	}

	// 解析节点服务
	if err := api.container.ResolveTyped("node_service", &api.nodeService); err != nil {
		return fmt.Errorf("failed to resolve node service: %w", err)
	}

	// 解析认证服务
	if err := api.container.ResolveTyped("auth_service", &api.authService); err != nil {
		return fmt.Errorf("failed to resolve auth service: %w", err)
	}

	// 解析匿名服务
	if err := api.container.ResolveTyped("anonymous_service", &api.anonymousService); err != nil {
		return fmt.Errorf("failed to resolve anonymous service: %w", err)
	}

	// 解析连接服务
	if err := api.container.ResolveTyped("connection_service", &api.connectionService); err != nil {
		return fmt.Errorf("failed to resolve connection service: %w", err)
	}

	// 解析统计服务
	if err := api.container.ResolveTyped("stats_service", &api.statsService); err != nil {
		return fmt.Errorf("failed to resolve stats service: %w", err)
	}

	utils.Infof("All services resolved successfully")
	return nil
}

// 用户管理接口
func (api *CloudControlAPI) CreateUser(username, email string) (*models.User, error) {
	return api.userService.CreateUser(username, email)
}

func (api *CloudControlAPI) GetUser(userID string) (*models.User, error) {
	return api.userService.GetUser(userID)
}

func (api *CloudControlAPI) UpdateUser(user *models.User) error {
	return api.userService.UpdateUser(user)
}

func (api *CloudControlAPI) DeleteUser(userID string) error {
	return api.userService.DeleteUser(userID)
}

func (api *CloudControlAPI) ListUsers(userType models.UserType) ([]*models.User, error) {
	return api.userService.ListUsers(userType)
}

func (api *CloudControlAPI) SearchUsers(keyword string) ([]*models.User, error) {
	return api.userService.SearchUsers(keyword)
}

func (api *CloudControlAPI) GetUserStats(userID string) (*stats.UserStats, error) {
	return api.userService.GetUserStats(userID)
}

// 客户端管理接口
func (api *CloudControlAPI) CreateClient(userID, clientName string) (*models.Client, error) {
	return api.clientService.CreateClient(userID, clientName)
}

func (api *CloudControlAPI) GetClient(clientID int64) (*models.Client, error) {
	return api.clientService.GetClient(clientID)
}

func (api *CloudControlAPI) TouchClient(clientID int64) {
	api.clientService.TouchClient(clientID)
}

func (api *CloudControlAPI) UpdateClient(client *models.Client) error {
	return api.clientService.UpdateClient(client)
}

func (api *CloudControlAPI) DeleteClient(clientID int64) error {
	return api.clientService.DeleteClient(clientID)
}

func (api *CloudControlAPI) UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error {
	return api.clientService.UpdateClientStatus(clientID, status, nodeID)
}

func (api *CloudControlAPI) ListClients(userID string, clientType models.ClientType) ([]*models.Client, error) {
	return api.clientService.ListClients(userID, clientType)
}

func (api *CloudControlAPI) ListUserClients(userID string) ([]*models.Client, error) {
	return api.clientService.ListUserClients(userID)
}

func (api *CloudControlAPI) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	return api.clientService.GetClientPortMappings(clientID)
}

func (api *CloudControlAPI) SearchClients(keyword string) ([]*models.Client, error) {
	return api.clientService.SearchClients(keyword)
}

func (api *CloudControlAPI) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	return api.clientService.GetClientStats(clientID)
}

// 端口映射管理接口
func (api *CloudControlAPI) CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error) {
	return api.mappingService.CreatePortMapping(mapping)
}

func (api *CloudControlAPI) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	return api.mappingService.GetPortMapping(mappingID)
}

func (api *CloudControlAPI) UpdatePortMapping(mapping *models.PortMapping) error {
	return api.mappingService.UpdatePortMapping(mapping)
}

func (api *CloudControlAPI) DeletePortMapping(mappingID string) error {
	return api.mappingService.DeletePortMapping(mappingID)
}

func (api *CloudControlAPI) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	return api.mappingService.UpdatePortMappingStatus(mappingID, status)
}

func (api *CloudControlAPI) UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error {
	return api.mappingService.UpdatePortMappingStats(mappingID, stats)
}

func (api *CloudControlAPI) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	return api.mappingService.GetUserPortMappings(userID)
}

func (api *CloudControlAPI) ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error) {
	return api.mappingService.ListPortMappings(mappingType)
}

func (api *CloudControlAPI) SearchPortMappings(keyword string) ([]*models.PortMapping, error) {
	return api.mappingService.SearchPortMappings(keyword)
}

// 节点管理接口
func (api *CloudControlAPI) NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error) {
	return api.nodeService.NodeRegister(req)
}

func (api *CloudControlAPI) NodeUnregister(req *models.NodeUnregisterRequest) error {
	return api.nodeService.NodeUnregister(req)
}

func (api *CloudControlAPI) NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error) {
	return api.nodeService.NodeHeartbeat(req)
}

func (api *CloudControlAPI) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error) {
	return api.nodeService.GetNodeServiceInfo(nodeID)
}

func (api *CloudControlAPI) GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error) {
	return api.nodeService.GetAllNodeServiceInfo()
}

// 认证接口
func (api *CloudControlAPI) Authenticate(req *models.AuthRequest) (*models.AuthResponse, error) {
	return api.authService.Authenticate(req)
}

func (api *CloudControlAPI) ValidateToken(token string) (*models.AuthResponse, error) {
	return api.authService.ValidateToken(token)
}

func (api *CloudControlAPI) GenerateJWTToken(clientID int64) (*JWTTokenInfo, error) {
	return api.authService.GenerateJWTToken(clientID)
}

func (api *CloudControlAPI) RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error) {
	return api.authService.RefreshJWTToken(refreshToken)
}

func (api *CloudControlAPI) ValidateJWTToken(token string) (*JWTTokenInfo, error) {
	return api.authService.ValidateJWTToken(token)
}

func (api *CloudControlAPI) RevokeJWTToken(token string) error {
	return api.authService.RevokeJWTToken(token)
}

// 匿名用户管理接口
func (api *CloudControlAPI) GenerateAnonymousCredentials() (*models.Client, error) {
	return api.anonymousService.GenerateAnonymousCredentials()
}

func (api *CloudControlAPI) GetAnonymousClient(clientID int64) (*models.Client, error) {
	return api.anonymousService.GetAnonymousClient(clientID)
}

func (api *CloudControlAPI) DeleteAnonymousClient(clientID int64) error {
	return api.anonymousService.DeleteAnonymousClient(clientID)
}

func (api *CloudControlAPI) ListAnonymousClients() ([]*models.Client, error) {
	return api.anonymousService.ListAnonymousClients()
}

func (api *CloudControlAPI) CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) {
	return api.anonymousService.CreateAnonymousMapping(sourceClientID, targetClientID, protocol, sourcePort, targetPort)
}

func (api *CloudControlAPI) GetAnonymousMappings() ([]*models.PortMapping, error) {
	return api.anonymousService.GetAnonymousMappings()
}

func (api *CloudControlAPI) CleanupExpiredAnonymous() error {
	return api.anonymousService.CleanupExpiredAnonymous()
}

// 连接管理接口
func (api *CloudControlAPI) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error {
	return api.connectionService.RegisterConnection(mappingID, connInfo)
}

func (api *CloudControlAPI) UnregisterConnection(connID string) error {
	return api.connectionService.UnregisterConnection(connID)
}

func (api *CloudControlAPI) GetConnections(mappingID string) ([]*models.ConnectionInfo, error) {
	return api.connectionService.GetConnections(mappingID)
}

func (api *CloudControlAPI) GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error) {
	return api.connectionService.GetClientConnections(clientID)
}

func (api *CloudControlAPI) UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error {
	return api.connectionService.UpdateConnectionStats(connID, bytesSent, bytesReceived)
}

// 统计接口
func (api *CloudControlAPI) GetSystemStats() (*stats.SystemStats, error) {
	return api.statsService.GetSystemStats()
}

func (api *CloudControlAPI) GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error) {
	return api.statsService.GetTrafficStats(timeRange)
}

func (api *CloudControlAPI) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) {
	return api.statsService.GetConnectionStats(timeRange)
}

// Close 关闭API
func (api *CloudControlAPI) Close() error {
	result := api.Dispose.Close()
	if result.HasErrors() {
		return fmt.Errorf("cloud control API cleanup failed: %s", result.Error())
	}
	return nil
}
