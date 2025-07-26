package managers

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/distributed"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
)

// CloudControl 基础云控实现，所有存储操作通过 Storage 接口
// 业务逻辑、资源管理、定时清理等通用逻辑全部在这里实现
// 子类只需注入不同的 Storage 实现

type CloudControl struct {
	*dispose.ResourceBase
	config            *ControlConfig
	storage           storage.Storage
	idManager         *idgen.IDManager
	userRepo          *repos.UserRepository
	clientRepo        *repos.ClientRepository
	mappingRepo       *repos.PortMappingRepo
	nodeRepo          *repos.NodeRepository
	connRepo          *repos.ConnectionRepo
	jwtManager        *JWTManager
	configManager     *ConfigManager
	cleanupManager    *CleanupManager
	statsManager      *StatsManager
	anonymousManager  *AnonymousManager
	nodeManager       *NodeManager
	searchManager     *SearchManager
	connectionManager *ConnectionManager
	lock              distributed.DistributedLock
	cleanupTicker     *time.Ticker
	done              chan bool
}

func NewCloudControl(config *ControlConfig, storage storage.Storage) *CloudControl {
	ctx := context.Background()
	repo := repos.NewRepository(storage)

	// 使用锁工厂创建分布式锁
	lockFactory := distributed.NewLockFactory(storage)
	owner := fmt.Sprintf("cloud_control_%d", time.Now().UnixNano())
	lock := lockFactory.CreateDefaultLock(owner)

	// 创建仓库实例
	userRepo := repos.NewUserRepository(repo)
	clientRepo := repos.NewClientRepository(repo)
	mappingRepo := repos.NewPortMappingRepo(repo)
	nodeRepo := repos.NewNodeRepository(repo)
	connRepo := repos.NewConnectionRepo(repo)

	// 创建ID管理器
	idManager := idgen.NewIDManager(storage, ctx)

	base := &CloudControl{
		ResourceBase:      dispose.NewResourceBase("CloudControl"),
		config:            config,
		storage:           storage,
		idManager:         idManager,
		userRepo:          userRepo,
		clientRepo:        clientRepo,
		mappingRepo:       mappingRepo,
		nodeRepo:          nodeRepo,
		connRepo:          connRepo,
		jwtManager:        NewJWTManager(config, repo),
		configManager:     NewConfigManager(storage, config, ctx),
		cleanupManager:    NewCleanupManager(storage, lock, ctx),
		statsManager:      NewStatsManager(userRepo, clientRepo, mappingRepo, nodeRepo),
		anonymousManager:  NewAnonymousManager(clientRepo, mappingRepo, idManager),
		nodeManager:       NewNodeManager(nodeRepo),
		searchManager:     NewSearchManager(userRepo, clientRepo, mappingRepo),
		connectionManager: NewConnectionManager(connRepo, idManager),
		lock:              lock,
		cleanupTicker:     time.NewTicker(constants.DefaultCleanupInterval),
		done:              make(chan bool),
	}
	base.Initialize(ctx)
	return base
}

// 这里实现 CloudControlAPI 的大部分方法，所有数据操作都用 b.storage
// ...（后续迁移 builtin.go 的通用方法到这里）

// 用户管理
func (b *CloudControl) CreateUser(username, email string) (*models.User, error) {
	userID, _ := b.idManager.GenerateUserID()
	now := time.Now()
	user := &models.User{
		ID:        userID,
		Username:  username,
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := b.userRepo.CreateUser(user); err != nil {
		return nil, err
	}
	return user, nil
}

func (b *CloudControl) GetUser(userID string) (*models.User, error) {
	return b.userRepo.GetUser(userID)
}

func (b *CloudControl) UpdateUser(user *models.User) error {
	user.UpdatedAt = time.Now()
	return b.userRepo.UpdateUser(user)
}

func (b *CloudControl) DeleteUser(userID string) error {
	return b.userRepo.DeleteUser(userID)
}

func (b *CloudControl) ListUsers(userType models.UserType) ([]*models.User, error) {
	return b.userRepo.ListUsers(userType)
}

// 客户端管理
func (b *CloudControl) CreateClient(userID, clientName string) (*models.Client, error) {
	// 生成客户端ID，确保不重复
	var clientID int64
	for attempts := 0; attempts < constants.DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idManager.GenerateClientID()
		if err != nil {
			return nil, fmt.Errorf("generate client ID failed: %w", err)
		}

		// 检查客户端是否已存在
		existingClient, err := b.clientRepo.GetClient(fmt.Sprintf("%d", generatedID))
		if err != nil {
			// 客户端不存在，可以使用这个ID
			clientID = generatedID
			break
		}

		if existingClient != nil {
			// 客户端已存在，释放ID并重试
			_ = b.idManager.ReleaseClientID(generatedID)
			continue
		}

		clientID = generatedID
		break
	}

	if clientID == 0 {
		return nil, fmt.Errorf("failed to generate unique client ID after %d attempts", constants.DefaultMaxAttempts)
	}

	authCode, err := b.idManager.GenerateAuthCode()
	if err != nil {
		// 如果生成认证码失败，释放客户端ID
		_ = b.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("generate auth code failed: %w", err)
	}

	secretKey, err := b.idManager.GenerateSecretKey()
	if err != nil {
		// 如果生成密钥失败，释放客户端ID
		_ = b.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("generate secret key failed: %w", err)
	}

	now := time.Now()
	client := &models.Client{
		ID:        clientID,
		UserID:    userID,
		Name:      clientName,
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    models.ClientStatusOffline,
		Type:      models.ClientTypeRegistered,
		Config: configs.ClientConfig{
			EnableCompression: constants.DefaultEnableCompression,
			BandwidthLimit:    constants.DefaultClientBandwidthLimit,
			MaxConnections:    constants.DefaultClientMaxConnections,
			AllowedPorts:      constants.DefaultAllowedPorts,
			BlockedPorts:      constants.DefaultBlockedPorts,
			AutoReconnect:     constants.DefaultAutoReconnect,
			HeartbeatInterval: constants.DefaultHeartbeatInterval,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := b.clientRepo.CreateClient(client); err != nil {
		// 如果保存失败，释放客户端ID
		_ = b.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("save client failed: %w", err)
	}

	if err := b.clientRepo.AddClientToUser(userID, client); err != nil {
		// 如果添加到用户失败，删除客户端并释放ID
		_ = b.clientRepo.DeleteClient(fmt.Sprintf("%d", clientID))
		_ = b.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("add client to user failed: %w", err)
	}

	return client, nil
}

func (b *CloudControl) TouchClient(clientID int64) {
	client, err := b.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
	if (err == nil) && (client != nil) {
		client.UpdatedAt = time.Now()
		_ = b.clientRepo.UpdateClient(client)
		_ = b.clientRepo.TouchClient(fmt.Sprintf("%d", clientID))
	}
}

func (b *CloudControl) GetClient(clientID int64) (*models.Client, error) {
	return b.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
}

func (b *CloudControl) UpdateClient(client *models.Client) error {
	client.UpdatedAt = time.Now()
	return b.clientRepo.UpdateClient(client)
}

func (b *CloudControl) DeleteClient(clientID int64) error {
	// 获取客户端信息，用于释放ID
	client, err := b.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
	if err == nil && client != nil {
		// 释放客户端ID
		_ = b.idManager.ReleaseClientID(clientID)
	}
	return b.clientRepo.DeleteClient(fmt.Sprintf("%d", clientID))
}

func (b *CloudControl) UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error {
	return b.clientRepo.UpdateClientStatus(fmt.Sprintf("%d", clientID), status, nodeID)
}

func (b *CloudControl) ListClients(userID string, clientType models.ClientType) ([]*models.Client, error) {
	if userID != "" {
		return b.clientRepo.ListUserClients(userID)
	}
	// 简单实现：返回所有客户端
	clients, err := b.clientRepo.ListUserClients("")
	if err != nil {
		return nil, err
	}
	if clientType == "" {
		return clients, nil
	}
	var filtered []*models.Client
	for _, client := range clients {
		if client.Type == clientType {
			filtered = append(filtered, client)
		}
	}
	return filtered, nil
}

func (b *CloudControl) ListUserClients(userID string) ([]*models.Client, error) {
	return b.clientRepo.ListUserClients(userID)
}

func (b *CloudControl) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	return b.mappingRepo.GetClientPortMappings(fmt.Sprintf("%d", clientID))
}

// 端口映射管理
func (b *CloudControl) CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error) {
	// 生成端口映射ID，确保不重复
	var mappingID string
	for attempts := 0; attempts < constants.DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idManager.GeneratePortMappingID()
		if err != nil {
			return nil, fmt.Errorf("generate mapping ID failed: %w", err)
		}

		// 检查端口映射是否已存在
		existingMapping, err := b.mappingRepo.GetPortMapping(generatedID)
		if err != nil {
			// 端口映射不存在，可以使用这个ID
			mappingID = generatedID
			break
		}

		if existingMapping != nil {
			// 端口映射已存在，释放ID并重试
			_ = b.idManager.ReleasePortMappingID(generatedID)
			continue
		}

		mappingID = generatedID
		break
	}

	if mappingID == "" {
		return nil, fmt.Errorf("failed to generate unique mapping ID after %d attempts", constants.DefaultMaxAttempts)
	}

	mapping.ID = mappingID
	mapping.CreatedAt = time.Now()
	mapping.UpdatedAt = time.Now()

	if err := b.mappingRepo.CreatePortMapping(mapping); err != nil {
		// 如果保存失败，释放ID
		_ = b.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("save port mapping failed: %w", err)
	}

	// 添加到用户的端口映射列表
	if err := b.mappingRepo.AddMappingToUser(mapping.UserID, mapping); err != nil {
		// 如果添加到用户失败，删除端口映射并释放ID
		_ = b.mappingRepo.DeletePortMapping(mappingID)
		_ = b.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("add mapping to user failed: %w", err)
	}

	return mapping, nil
}

func (b *CloudControl) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	return b.mappingRepo.GetUserPortMappings(userID)
}

func (b *CloudControl) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	return b.mappingRepo.GetPortMapping(mappingID)
}

func (b *CloudControl) UpdatePortMapping(mapping *models.PortMapping) error {
	mapping.UpdatedAt = time.Now()
	return b.mappingRepo.UpdatePortMapping(mapping)
}

func (b *CloudControl) DeletePortMapping(mappingID string) error {
	// 获取端口映射信息，用于释放ID
	mapping, err := b.mappingRepo.GetPortMapping(mappingID)
	if err == nil && mapping != nil {
		// 释放端口映射ID
		_ = b.idManager.ReleasePortMappingID(mappingID)
	}
	return b.mappingRepo.DeletePortMapping(mappingID)
}

func (b *CloudControl) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	return b.mappingRepo.UpdatePortMappingStatus(mappingID, status)
}

func (b *CloudControl) UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error {
	return b.mappingRepo.UpdatePortMappingStats(mappingID, stats)
}

func (b *CloudControl) ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error) {
	// 简化实现：返回所有映射
	return b.mappingRepo.GetUserPortMappings("")
}

// 匿名用户管理 - 委托给AnonymousManager
func (b *CloudControl) GenerateAnonymousCredentials() (*models.Client, error) {
	return b.anonymousManager.GenerateAnonymousCredentials()
}

func (b *CloudControl) GetAnonymousClient(clientID int64) (*models.Client, error) {
	return b.anonymousManager.GetAnonymousClient(clientID)
}

func (b *CloudControl) ListAnonymousClients() ([]*models.Client, error) {
	return b.anonymousManager.ListAnonymousClients()
}

func (b *CloudControl) DeleteAnonymousClient(clientID int64) error {
	return b.anonymousManager.DeleteAnonymousClient(clientID)
}

func (b *CloudControl) CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) {
	return b.anonymousManager.CreateAnonymousMapping(sourceClientID, targetClientID, protocol, sourcePort, targetPort)
}

func (b *CloudControl) GetAnonymousMappings() ([]*models.PortMapping, error) {
	return b.anonymousManager.GetAnonymousMappings()
}

func (b *CloudControl) CleanupExpiredAnonymous() error {
	return b.anonymousManager.CleanupExpiredAnonymous()
}

// 节点管理 - 委托给NodeManager
func (b *CloudControl) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error) {
	return b.nodeManager.GetNodeServiceInfo(nodeID)
}

func (b *CloudControl) GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error) {
	return b.nodeManager.GetAllNodeServiceInfo()
}

// 统计相关 - 委托给StatsManager
func (b *CloudControl) GetUserStats(userID string) (*stats.UserStats, error) {
	return b.statsManager.GetUserStats(userID)
}

func (b *CloudControl) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	return b.statsManager.GetClientStats(clientID)
}

func (b *CloudControl) GetSystemStats() (*stats.SystemStats, error) {
	return b.statsManager.GetSystemStats()
}

func (b *CloudControl) GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error) {
	return b.statsManager.GetTrafficStats(timeRange)
}

func (b *CloudControl) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) {
	return b.statsManager.GetConnectionStats(timeRange)
}

// 搜索相关 - 委托给SearchManager
func (b *CloudControl) SearchUsers(keyword string) ([]*models.User, error) {
	return b.searchManager.SearchUsers(keyword)
}

func (b *CloudControl) SearchClients(keyword string) ([]*models.Client, error) {
	return b.searchManager.SearchClients(keyword)
}

func (b *CloudControl) SearchPortMappings(keyword string) ([]*models.PortMapping, error) {
	return b.searchManager.SearchPortMappings(keyword)
}

// 连接管理 - 委托给ConnectionManager
func (b *CloudControl) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error {
	return b.connectionManager.RegisterConnection(mappingID, connInfo)
}

func (b *CloudControl) UnregisterConnection(connID string) error {
	return b.connectionManager.UnregisterConnection(connID)
}

func (b *CloudControl) GetConnections(mappingID string) ([]*models.ConnectionInfo, error) {
	return b.connectionManager.GetConnections(mappingID)
}

func (b *CloudControl) GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error) {
	return b.connectionManager.GetClientConnections(clientID)
}

func (b *CloudControl) UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error {
	return b.connectionManager.UpdateConnectionStats(connID, bytesSent, bytesReceived)
}

// JWT管理
func (b *CloudControl) GenerateJWTToken(clientID int64) (*JWTTokenInfo, error) {
	client, err := b.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
	if err != nil {
		return nil, err
	}
	return b.jwtManager.GenerateTokenPair(b.ResourceBase.Dispose.Ctx(), client)
}

func (b *CloudControl) RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error) {
	// 验证刷新令牌
	claims, err := b.jwtManager.ValidateRefreshToken(b.ResourceBase.Dispose.Ctx(), refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// 获取客户端信息
	client, err := b.clientRepo.GetClient(fmt.Sprintf("%d", claims.ClientID))
	if err != nil {
		return nil, err
	}

	// 生成新的令牌对
	return b.jwtManager.GenerateTokenPair(b.ResourceBase.Dispose.Ctx(), client)
}

func (b *CloudControl) ValidateJWTToken(token string) (*JWTTokenInfo, error) {
	claims, err := b.jwtManager.ValidateAccessToken(b.ResourceBase.Dispose.Ctx(), token)
	if err != nil {
		return nil, err
	}

	client, err := b.clientRepo.GetClient(fmt.Sprintf("%d", claims.ClientID))
	if err != nil {
		return nil, err
	}

	return &JWTTokenInfo{
		Token:    token,
		ClientId: client.ID,
		TokenID:  claims.ID,
	}, nil
}

func (b *CloudControl) RevokeJWTToken(token string) error {
	// 验证令牌以获取客户端ID
	claims, err := b.jwtManager.ValidateAccessToken(b.ResourceBase.Dispose.Ctx(), token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// 将令牌加入黑名单
	return b.jwtManager.RevokeToken(b.ResourceBase.Dispose.Ctx(), claims.ID)
}

// 核心节点管理
func (b *CloudControl) NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error) {
	// 生成节点ID，确保不重复
	var nodeID string
	for attempts := 0; attempts < constants.DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idManager.GenerateNodeID()
		if err != nil {
			return nil, fmt.Errorf("generate node ID failed: %w", err)
		}

		// 检查节点是否已存在
		existingNode, err := b.nodeRepo.GetNode(generatedID)
		if err != nil {
			// 节点不存在，可以使用这个ID
			nodeID = generatedID
			break
		}

		if existingNode != nil {
			// 节点已存在，释放ID并重试
			_ = b.idManager.ReleaseNodeID(generatedID)
			continue
		}

		nodeID = generatedID
		break
	}

	if nodeID == "" {
		return nil, fmt.Errorf("failed to generate unique node ID after %d attempts", constants.DefaultMaxAttempts)
	}

	now := time.Now()
	node := &models.Node{
		ID:        nodeID,
		Name:      fmt.Sprintf("Node-%s", nodeID),
		Address:   req.Address,
		Meta:      req.Meta,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := b.nodeRepo.CreateNode(node); err != nil {
		// 如果保存失败，释放节点ID
		_ = b.idManager.ReleaseNodeID(nodeID)
		return nil, fmt.Errorf("save node failed: %w", err)
	}

	return &models.NodeRegisterResponse{
		NodeID:  nodeID,
		Success: true,
		Message: "Node registered successfully",
	}, nil
}

func (b *CloudControl) NodeUnregister(req *models.NodeUnregisterRequest) error {
	// 获取节点信息，用于释放ID
	node, err := b.nodeRepo.GetNode(req.NodeID)
	if err == nil && node != nil {
		// 释放节点ID
		_ = b.idManager.ReleaseNodeID(req.NodeID)
	}
	return b.nodeRepo.DeleteNode(req.NodeID)
}

func (b *CloudControl) NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error) {
	// 更新节点心跳时间
	node, err := b.nodeRepo.GetNode(req.NodeID)
	if err != nil {
		return &models.NodeHeartbeatResponse{
			Success: false,
			Message: "Node not found",
		}, nil
	}

	if node == nil {
		return &models.NodeHeartbeatResponse{
			Success: false,
			Message: "Node not found",
		}, nil
	}

	// 更新节点信息
	node.Address = req.Address
	node.UpdatedAt = time.Now()
	if err := b.nodeRepo.UpdateNode(node); err != nil {
		return &models.NodeHeartbeatResponse{
			Success: false,
			Message: "Failed to update node",
		}, nil
	}

	return &models.NodeHeartbeatResponse{
		Success: true,
		Message: "Heartbeat received",
	}, nil
}

func (b *CloudControl) Authenticate(req *models.AuthRequest) (*models.AuthResponse, error) {
	// 获取客户端信息
	client, err := b.clientRepo.GetClient(fmt.Sprintf("%d", req.ClientID))
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	if client == nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 验证认证码
	if client.AuthCode != req.AuthCode {
		return &models.AuthResponse{
			Success: false,
			Message: "Invalid auth code",
		}, nil
	}

	// 验证密钥（如果提供）
	if req.SecretKey != "" && client.SecretKey != req.SecretKey {
		return &models.AuthResponse{
			Success: false,
			Message: "Invalid secret key",
		}, nil
	}

	// 更新客户端状态
	client.Status = models.ClientStatusOnline
	client.NodeID = req.NodeID
	client.IPAddress = req.IPAddress
	client.Version = req.Version
	now := time.Now()
	client.LastSeen = &now
	client.UpdatedAt = now

	if err := b.clientRepo.UpdateClient(client); err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Failed to update client status",
		}, nil
	}

	// 生成JWT令牌
	tokenInfo, err := b.jwtManager.GenerateTokenPair(b.ResourceBase.Dispose.Ctx(), client)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Failed to generate token",
		}, nil
	}

	// 获取节点信息
	node, _ := b.nodeRepo.GetNode(req.NodeID)

	return &models.AuthResponse{
		Success:   true,
		Token:     tokenInfo.Token,
		Client:    client,
		Node:      node,
		ExpiresAt: tokenInfo.ExpiresAt,
		Message:   "Authentication successful",
	}, nil
}

func (b *CloudControl) ValidateToken(token string) (*models.AuthResponse, error) {
	// 验证JWT令牌
	claims, err := b.jwtManager.ValidateAccessToken(b.ResourceBase.Dispose.Ctx(), token)
	if err != nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	// 获取客户端信息
	client, err := b.clientRepo.GetClient(fmt.Sprintf("%d", claims.ClientID))
	if err != nil {
		return nil, err
	}

	if client == nil {
		return &models.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 获取节点信息
	var node *models.Node
	if client.NodeID != "" {
		node, _ = b.nodeRepo.GetNode(client.NodeID)
	}

	return &models.AuthResponse{
		Success:   true,
		Token:     token,
		Client:    client,
		Node:      node,
		ExpiresAt: claims.ExpiresAt.Time,
		Message:   "Token validated successfully",
	}, nil
}

// Close 实现 CloudControlAPI 接口的 Close 方法
func (b *CloudControl) Close() error {
	// 停止清理定时器
	if b.cleanupTicker != nil {
		b.cleanupTicker.Stop()
	}

	// 关闭 done 通道
	close(b.done)

	// 调用 ResourceBase 的清理逻辑
	result := b.ResourceBase.Dispose.Close()
	if result.HasErrors() {
		return result
	}
	return nil
}
