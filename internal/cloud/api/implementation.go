package api

import (
	"context"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
)

// CloudControlAPIImpl API实现
type CloudControlAPIImpl struct {
	userService        services.UserService
	clientService      services.ClientService
	portMappingService services.PortMappingService
	nodeService        services.NodeService
	authService        services.AuthService
	connectionService  services.ConnectionService
	statsService       services.StatsService
	anonymousService   services.AnonymousService
}

// NewCloudControlAPI 创建新的API实例
func NewCloudControlAPI(
	userService services.UserService,
	clientService services.ClientService,
	portMappingService services.PortMappingService,
	nodeService services.NodeService,
	authService services.AuthService,
	connectionService services.ConnectionService,
	statsService services.StatsService,
	anonymousService services.AnonymousService,
) *CloudControlAPIImpl {
	return &CloudControlAPIImpl{
		userService:        userService,
		clientService:      clientService,
		portMappingService: portMappingService,
		nodeService:        nodeService,
		authService:        authService,
		connectionService:  connectionService,
		statsService:       statsService,
		anonymousService:   anonymousService,
	}
}

// 用户管理实现
func (api *CloudControlAPIImpl) CreateUser(ctx context.Context, user *models.User) error {
	_, err := api.userService.CreateUser(user.Username, user.Email)
	return err
}

func (api *CloudControlAPIImpl) GetUser(ctx context.Context, userID string) (*models.User, error) {
	return api.userService.GetUser(userID)
}

func (api *CloudControlAPIImpl) UpdateUser(ctx context.Context, user *models.User) error {
	return api.userService.UpdateUser(user)
}

func (api *CloudControlAPIImpl) DeleteUser(ctx context.Context, userID string) error {
	return api.userService.DeleteUser(userID)
}

func (api *CloudControlAPIImpl) ListUsers(ctx context.Context) ([]*models.User, error) {
	return api.userService.ListUsers(models.UserTypeRegistered)
}

// 客户端管理实现
func (api *CloudControlAPIImpl) CreateClient(ctx context.Context, client *models.Client) error {
	_, err := api.clientService.CreateClient(client.UserID, client.Name)
	return err
}

func (api *CloudControlAPIImpl) GetClient(ctx context.Context, clientID int64) (*models.Client, error) {
	return api.clientService.GetClient(clientID)
}

func (api *CloudControlAPIImpl) UpdateClient(ctx context.Context, client *models.Client) error {
	return api.clientService.UpdateClient(client)
}

func (api *CloudControlAPIImpl) DeleteClient(ctx context.Context, clientID int64) error {
	return api.clientService.DeleteClient(clientID)
}

func (api *CloudControlAPIImpl) ListClients(ctx context.Context) ([]*models.Client, error) {
	return api.clientService.ListClients("", models.ClientTypeRegistered)
}

// 端口映射管理实现
func (api *CloudControlAPIImpl) CreatePortMapping(ctx context.Context, mapping *models.PortMapping) error {
	_, err := api.portMappingService.CreatePortMapping(mapping)
	return err
}

func (api *CloudControlAPIImpl) GetPortMapping(ctx context.Context, mappingID string) (*models.PortMapping, error) {
	return api.portMappingService.GetPortMapping(mappingID)
}

func (api *CloudControlAPIImpl) UpdatePortMapping(ctx context.Context, mapping *models.PortMapping) error {
	return api.portMappingService.UpdatePortMapping(mapping)
}

func (api *CloudControlAPIImpl) DeletePortMapping(ctx context.Context, mappingID string) error {
	return api.portMappingService.DeletePortMapping(mappingID)
}

func (api *CloudControlAPIImpl) ListPortMappings(ctx context.Context) ([]*models.PortMapping, error) {
	return api.portMappingService.ListPortMappings(models.MappingTypeRegistered)
}

// 节点管理实现
func (api *CloudControlAPIImpl) RegisterNode(ctx context.Context, node *models.Node) error {
	req := &models.NodeRegisterRequest{
		NodeID:  node.ID,
		Address: node.Address,
		Version: "1.0.0",
		Meta:    node.Meta,
	}
	_, err := api.nodeService.NodeRegister(req)
	return err
}

func (api *CloudControlAPIImpl) GetNode(ctx context.Context, nodeID string) (*models.Node, error) {
	info, err := api.nodeService.GetNodeServiceInfo(nodeID)
	if err != nil {
		return nil, err
	}
	return &models.Node{
		ID:      info.NodeID,
		Address: info.Address,
	}, nil
}

func (api *CloudControlAPIImpl) UpdateNode(ctx context.Context, node *models.Node) error {
	// 节点更新通过心跳实现
	return nil
}

func (api *CloudControlAPIImpl) UnregisterNode(ctx context.Context, nodeID string) error {
	req := &models.NodeUnregisterRequest{NodeID: nodeID}
	return api.nodeService.NodeUnregister(req)
}

func (api *CloudControlAPIImpl) ListNodes(ctx context.Context) ([]*models.Node, error) {
	infos, err := api.nodeService.GetAllNodeServiceInfo()
	if err != nil {
		return nil, err
	}
	nodes := make([]*models.Node, len(infos))
	for i, info := range infos {
		nodes[i] = &models.Node{
			ID:      info.NodeID,
			Address: info.Address,
		}
	}
	return nodes, nil
}

// 认证管理实现
func (api *CloudControlAPIImpl) AuthenticateClient(ctx context.Context, req *models.AuthRequest) (*models.AuthResponse, error) {
	return api.authService.Authenticate(req)
}

func (api *CloudControlAPIImpl) ValidateToken(ctx context.Context, token string) (bool, error) {
	_, err := api.authService.ValidateToken(token)
	return err == nil, err
}

func (api *CloudControlAPIImpl) RevokeToken(ctx context.Context, token string) error {
	return api.authService.RevokeJWTToken(token)
}

// 连接管理实现
func (api *CloudControlAPIImpl) RegisterConnection(ctx context.Context, conn *models.ConnectionInfo) error {
	return api.connectionService.RegisterConnection(conn.MappingID, conn)
}

func (api *CloudControlAPIImpl) GetConnection(ctx context.Context, connID string) (*models.ConnectionInfo, error) {
	// 这个接口在服务层没有直接对应的方法，需要扩展
	return nil, nil
}

func (api *CloudControlAPIImpl) UpdateConnection(ctx context.Context, conn *models.ConnectionInfo) error {
	// 这个接口在服务层没有直接对应的方法，需要扩展
	return nil
}

func (api *CloudControlAPIImpl) UnregisterConnection(ctx context.Context, connID string) error {
	return api.connectionService.UnregisterConnection(connID)
}

func (api *CloudControlAPIImpl) ListConnections(ctx context.Context) ([]*models.ConnectionInfo, error) {
	// 这个接口在服务层没有直接对应的方法，需要扩展
	return nil, nil
}

// 统计信息实现
func (api *CloudControlAPIImpl) GetSystemStats(ctx context.Context) (interface{}, error) {
	return api.statsService.GetSystemStats()
}

func (api *CloudControlAPIImpl) GetClientStats(ctx context.Context, clientID int64) (interface{}, error) {
	return api.clientService.GetClientStats(clientID)
}

func (api *CloudControlAPIImpl) GetNodeStats(ctx context.Context, nodeID string) (interface{}, error) {
	// 这个接口在服务层没有直接对应的方法，需要扩展
	return nil, nil
}

// 资源清理
func (api *CloudControlAPIImpl) Close() error {
	// 这里可以添加资源清理逻辑
	return nil
}
