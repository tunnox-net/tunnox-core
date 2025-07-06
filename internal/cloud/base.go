package cloud

import (
	"context"
	"fmt"
	"strings"
	"time"
	"tunnox-core/internal/utils"
)

// BaseCloudControl 基础云控实现，所有存储操作通过 Storage 接口
// 业务逻辑、资源管理、定时清理等通用逻辑全部在这里实现
// 子类只需注入不同的 Storage 实现

type BaseCloudControl struct {
	config         *CloudControlConfig
	storage        Storage
	idGen          *DistributedIDGenerator
	userRepo       *UserRepository
	clientRepo     *ClientRepository
	mappingRepo    *PortMappingRepository
	nodeRepo       *NodeRepository
	connRepo       *ConnectionRepository
	jwtManager     *JWTManager
	configManager  *ConfigManager
	cleanupManager *CleanupManager
	lock           DistributedLock
	cleanupTicker  *time.Ticker
	done           chan bool
	utils.Dispose
}

func NewBaseCloudControl(config *CloudControlConfig, storage Storage) *BaseCloudControl {
	ctx := context.Background()
	repo := NewRepository(storage)
	lock := NewMemoryLock() // 可替换为分布式锁
	base := &BaseCloudControl{
		config:         config,
		storage:        storage,
		idGen:          NewDistributedIDGenerator(storage, lock),
		userRepo:       NewUserRepository(repo),
		clientRepo:     NewClientRepository(repo),
		mappingRepo:    NewPortMappingRepository(repo),
		nodeRepo:       NewNodeRepository(repo),
		connRepo:       NewConnectionRepository(repo),
		jwtManager:     NewJWTManager(config, repo),
		configManager:  NewConfigManager(storage, config, ctx),
		cleanupManager: NewCleanupManager(storage, lock, ctx),
		lock:           lock,
		cleanupTicker:  time.NewTicker(DefaultCleanupInterval),
		done:           make(chan bool),
	}
	base.SetCtx(ctx, base.onClose)
	return base
}

// onClose 资源清理回调
func (b *BaseCloudControl) onClose() {
	utils.Infof("Cleaning up cloud control resources...")
	time.Sleep(100 * time.Millisecond)
	if b.jwtManager != nil {
		utils.Infof("JWT manager resources cleaned up")
	}
	if b.cleanupManager != nil {
		utils.Infof("Cleanup manager resources cleaned up")
	}
	if b.lock != nil {
		utils.Infof("Distributed lock resources cleaned up")
	}
	utils.Infof("Cloud control resources cleanup completed")
}

// 这里实现 CloudControlAPI 的大部分方法，所有数据操作都用 b.storage
// ...（后续迁移 builtin.go 的通用方法到这里）

// 用户管理
func (b *BaseCloudControl) CreateUser(username, email string) (*User, error) {
	userID, _ := b.idGen.GenerateUserID(b.Ctx())
	now := time.Now()
	user := &User{
		ID:        userID,
		Username:  username,
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := b.userRepo.CreateUser(b.Ctx(), user); err != nil {
		return nil, err
	}
	return user, nil
}

func (b *BaseCloudControl) GetUser(userID string) (*User, error) {
	return b.userRepo.GetUser(b.Ctx(), userID)
}

func (b *BaseCloudControl) UpdateUser(user *User) error {
	user.UpdatedAt = time.Now()
	return b.userRepo.UpdateUser(b.Ctx(), user)
}

func (b *BaseCloudControl) DeleteUser(userID string) error {
	return b.userRepo.DeleteUser(b.Ctx(), userID)
}

func (b *BaseCloudControl) ListUsers(userType UserType) ([]*User, error) {
	return b.userRepo.ListUsers(b.Ctx(), userType)
}

// 客户端管理
func (b *BaseCloudControl) CreateClient(userID, clientName string) (*Client, error) {
	// 生成客户端ID，确保不重复
	var clientID int64
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateClientID(b.Ctx())
		if err != nil {
			return nil, fmt.Errorf("generate client ID failed: %w", err)
		}

		// 检查客户端是否已存在
		existingClient, err := b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", generatedID))
		if err != nil {
			// 客户端不存在，可以使用这个ID
			clientID = generatedID
			break
		}

		if existingClient != nil {
			// 客户端已存在，释放ID并重试
			_ = b.idGen.ReleaseClientID(b.Ctx(), generatedID)
			continue
		}

		clientID = generatedID
		break
	}

	if clientID == 0 {
		return nil, fmt.Errorf("failed to generate unique client ID after %d attempts", DefaultMaxAttempts)
	}

	authCode, err := b.idGen.GenerateAuthCode()
	if err != nil {
		// 如果生成认证码失败，释放客户端ID
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
		return nil, fmt.Errorf("generate auth code failed: %w", err)
	}

	secretKey, err := b.idGen.GenerateSecretKey()
	if err != nil {
		// 如果生成密钥失败，释放客户端ID
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
		return nil, fmt.Errorf("generate secret key failed: %w", err)
	}

	now := time.Now()
	client := &Client{
		ID:        clientID,
		UserID:    userID,
		Name:      clientName,
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    ClientStatusOffline,
		Type:      ClientTypeRegistered,
		Config: ClientConfig{
			EnableCompression: DefaultEnableCompression,
			BandwidthLimit:    DefaultClientBandwidthLimit,
			MaxConnections:    DefaultClientMaxConnections,
			AllowedPorts:      DefaultAllowedPorts,
			BlockedPorts:      DefaultBlockedPorts,
			AutoReconnect:     DefaultAutoReconnect,
			HeartbeatInterval: DefaultHeartbeatInterval,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := b.clientRepo.CreateClient(b.Ctx(), client); err != nil {
		// 如果保存失败，释放客户端ID
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
		return nil, fmt.Errorf("save client failed: %w", err)
	}

	// 强制添加到用户列表（即使 userID 为空也加到匿名列表）
	if err := b.clientRepo.AddClientToUser(b.Ctx(), userID, client); err != nil {
		// 如果添加到用户失败，删除客户端并释放ID
		_ = b.clientRepo.DeleteClient(b.Ctx(), fmt.Sprintf("%d", clientID))
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
		return nil, fmt.Errorf("add client to user failed: %w", err)
	}

	return client, nil
}

func (b *BaseCloudControl) TouchClient(clientID int64) {
	client, err := b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", clientID))
	if (err == nil) && (client != nil) {
		client.UpdatedAt = time.Now()
		_ = b.clientRepo.UpdateClient(b.Ctx(), client)
		_ = b.clientRepo.TouchClient(b.Ctx(), fmt.Sprintf("%d", clientID))
	}
}

func (b *BaseCloudControl) GetClient(clientID int64) (*Client, error) {
	return b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", clientID))
}

func (b *BaseCloudControl) UpdateClient(client *Client) error {
	client.UpdatedAt = time.Now()
	return b.clientRepo.UpdateClient(b.Ctx(), client)
}

func (b *BaseCloudControl) DeleteClient(clientID int64) error {
	// 获取客户端信息，用于释放ID
	client, err := b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", clientID))
	if err == nil && client != nil {
		// 释放客户端ID
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
	}
	return b.clientRepo.DeleteClient(b.Ctx(), fmt.Sprintf("%d", clientID))
}

func (b *BaseCloudControl) UpdateClientStatus(clientID int64, status ClientStatus, nodeID string) error {
	return b.clientRepo.UpdateClientStatus(b.Ctx(), fmt.Sprintf("%d", clientID), status, nodeID)
}

func (b *BaseCloudControl) ListClients(userID string, clientType ClientType) ([]*Client, error) {
	if userID != "" {
		return b.clientRepo.ListUserClients(b.Ctx(), userID)
	}
	// 简单实现：返回所有客户端
	clients, err := b.clientRepo.ListUserClients(b.Ctx(), "")
	if err != nil {
		return nil, err
	}
	if clientType == "" {
		return clients, nil
	}
	var filtered []*Client
	for _, client := range clients {
		if client.Type == clientType {
			filtered = append(filtered, client)
		}
	}
	return filtered, nil
}

func (b *BaseCloudControl) GetUserClients(userID string) ([]*Client, error) {
	return b.clientRepo.ListUserClients(b.Ctx(), userID)
}

func (b *BaseCloudControl) GetClientPortMappings(clientID int64) ([]*PortMapping, error) {
	return b.mappingRepo.ListClientMappings(b.Ctx(), fmt.Sprintf("%d", clientID))
}

// 端口映射管理
func (b *BaseCloudControl) CreatePortMapping(mapping *PortMapping) (*PortMapping, error) {
	// 生成端口映射ID，确保不重复
	var mappingID string
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateMappingID(b.Ctx())
		if err != nil {
			return nil, fmt.Errorf("generate mapping ID failed: %w", err)
		}

		// 检查端口映射是否已存在
		existingMapping, err := b.mappingRepo.GetPortMapping(b.Ctx(), generatedID)
		if err != nil {
			// 端口映射不存在，可以使用这个ID
			mappingID = generatedID
			break
		}

		if existingMapping != nil {
			// 端口映射已存在，释放ID并重试
			_ = b.idGen.ReleaseMappingID(b.Ctx(), generatedID)
			continue
		}

		mappingID = generatedID
		break
	}

	if mappingID == "" {
		return nil, fmt.Errorf("failed to generate unique mapping ID after %d attempts", DefaultMaxAttempts)
	}

	mapping.ID = mappingID
	mapping.Status = MappingStatusActive
	mapping.CreatedAt = time.Now()
	mapping.UpdatedAt = time.Now()

	if err := b.mappingRepo.CreatePortMapping(b.Ctx(), mapping); err != nil {
		// 如果保存失败，释放ID
		_ = b.idGen.ReleaseMappingID(b.Ctx(), mappingID)
		return nil, fmt.Errorf("save port mapping failed: %w", err)
	}

	if mapping.UserID != "" {
		if err := b.mappingRepo.AddMappingToUser(b.Ctx(), mapping.UserID, mapping); err != nil {
			// 如果添加到用户失败，删除端口映射并释放ID
			_ = b.mappingRepo.DeletePortMapping(b.Ctx(), mappingID)
			_ = b.idGen.ReleaseMappingID(b.Ctx(), mappingID)
			return nil, fmt.Errorf("add mapping to user failed: %w", err)
		}
	}

	return mapping, nil
}

func (b *BaseCloudControl) GetPortMappings(userID string) ([]*PortMapping, error) {
	return b.mappingRepo.ListUserMappings(b.Ctx(), userID)
}

func (b *BaseCloudControl) GetPortMapping(mappingID string) (*PortMapping, error) {
	return b.mappingRepo.GetPortMapping(b.Ctx(), mappingID)
}

func (b *BaseCloudControl) UpdatePortMapping(mapping *PortMapping) error {
	mapping.UpdatedAt = time.Now()
	return b.mappingRepo.UpdatePortMapping(b.Ctx(), mapping)
}

func (b *BaseCloudControl) DeletePortMapping(mappingID string) error {
	// 获取端口映射信息，用于释放ID
	mapping, err := b.mappingRepo.GetPortMapping(b.Ctx(), mappingID)
	if err == nil && mapping != nil {
		// 释放端口映射ID
		_ = b.idGen.ReleaseMappingID(b.Ctx(), mappingID)
	}
	return b.mappingRepo.DeletePortMapping(b.Ctx(), mappingID)
}

func (b *BaseCloudControl) UpdatePortMappingStatus(mappingID string, status MappingStatus) error {
	return b.mappingRepo.UpdatePortMappingStatus(b.Ctx(), mappingID, status)
}

func (b *BaseCloudControl) UpdatePortMappingStats(mappingID string, stats *TrafficStats) error {
	return b.mappingRepo.UpdatePortMappingStats(b.Ctx(), mappingID, stats)
}

func (b *BaseCloudControl) ListPortMappings(mappingType MappingType) ([]*PortMapping, error) {
	// 简化实现：返回所有映射
	return b.mappingRepo.ListUserMappings(b.Ctx(), "")
}

// 匿名用户管理
func (b *BaseCloudControl) GenerateAnonymousCredentials() (*Client, error) {
	// 生成客户端ID，确保不重复
	var clientID int64
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateClientID(b.Ctx())
		if err != nil {
			return nil, fmt.Errorf("generate client ID failed: %w", err)
		}
		// 检查客户端是否已存在
		existingClient, err := b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", generatedID))
		if err != nil {
			clientID = generatedID
			break
		}
		if existingClient != nil {
			_ = b.idGen.ReleaseClientID(b.Ctx(), generatedID)
			continue
		}
		clientID = generatedID
		break
	}
	if clientID == 0 {
		return nil, fmt.Errorf("failed to generate unique client ID after %d attempts", DefaultMaxAttempts)
	}

	authCode, err := b.idGen.GenerateAuthCode()
	if err != nil {
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
		return nil, fmt.Errorf("generate auth code failed: %w", err)
	}
	secretKey, err := b.idGen.GenerateSecretKey()
	if err != nil {
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
		return nil, fmt.Errorf("generate secret key failed: %w", err)
	}
	now := time.Now()
	client := &Client{
		ID:        clientID,
		UserID:    "",
		Name:      fmt.Sprintf("Anonymous-%s", authCode),
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    ClientStatusOffline,
		Type:      ClientTypeAnonymous,
		Config: ClientConfig{
			EnableCompression: DefaultEnableCompression,
			BandwidthLimit:    DefaultAnonymousBandwidthLimit,
			MaxConnections:    DefaultAnonymousMaxConnections,
			AllowedPorts:      DefaultAllowedPorts,
			BlockedPorts:      DefaultBlockedPorts,
			AutoReconnect:     DefaultAutoReconnect,
			HeartbeatInterval: DefaultHeartbeatInterval,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := b.clientRepo.CreateClient(b.Ctx(), client); err != nil {
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
		return nil, fmt.Errorf("save anonymous client failed: %w", err)
	}
	if err := b.clientRepo.AddClientToUser(b.Ctx(), "", client); err != nil {
		_ = b.clientRepo.DeleteClient(b.Ctx(), fmt.Sprintf("%d", clientID))
		_ = b.idGen.ReleaseClientID(b.Ctx(), clientID)
		return nil, fmt.Errorf("add anonymous client to list failed: %w", err)
	}
	return client, nil
}

func (b *BaseCloudControl) GetAnonymousClient(clientID int64) (*Client, error) {
	client, err := b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", clientID))
	if err != nil {
		return nil, err
	}
	if client.Type != ClientTypeAnonymous {
		return nil, fmt.Errorf("client is not anonymous")
	}
	return client, nil
}

func (b *BaseCloudControl) ListAnonymousClients() ([]*Client, error) {
	return b.clientRepo.ListUserClients(b.Ctx(), "")
}

func (b *BaseCloudControl) DeleteAnonymousClient(clientID int64) error {
	return b.DeleteClient(clientID)
}

func (b *BaseCloudControl) CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol Protocol, sourcePort, targetPort int) (*PortMapping, error) {
	// 生成端口映射ID，确保不重复
	var mappingID string
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateMappingID(b.Ctx())
		if err != nil {
			return nil, fmt.Errorf("generate mapping ID failed: %w", err)
		}

		// 检查端口映射是否已存在
		existingMapping, err := b.mappingRepo.GetPortMapping(b.Ctx(), generatedID)
		if err != nil {
			// 端口映射不存在，可以使用这个ID
			mappingID = generatedID
			break
		}

		if existingMapping != nil {
			// 端口映射已存在，释放ID并重试
			_ = b.idGen.ReleaseMappingID(b.Ctx(), generatedID)
			continue
		}

		mappingID = generatedID
		break
	}

	if mappingID == "" {
		return nil, fmt.Errorf("failed to generate unique mapping ID after %d attempts", DefaultMaxAttempts)
	}

	now := time.Now()
	mapping := &PortMapping{
		ID:             mappingID,
		UserID:         "",
		SourceClientID: sourceClientID,
		TargetClientID: targetClientID,
		Protocol:       protocol,
		SourcePort:     sourcePort,
		TargetPort:     targetPort,
		Status:         MappingStatusActive,
		Type:           MappingTypeAnonymous,
		CreatedAt:      now,
		UpdatedAt:      now,
		TrafficStats:   TrafficStats{},
	}

	if err := b.mappingRepo.CreatePortMapping(b.Ctx(), mapping); err != nil {
		// 如果保存失败，释放ID
		_ = b.idGen.ReleaseMappingID(b.Ctx(), mappingID)
		return nil, fmt.Errorf("save anonymous mapping failed: %w", err)
	}

	if err := b.mappingRepo.AddMappingToUser(b.Ctx(), "", mapping); err != nil {
		// 如果添加到匿名列表失败，删除映射并释放ID
		_ = b.mappingRepo.DeletePortMapping(b.Ctx(), mappingID)
		_ = b.idGen.ReleaseMappingID(b.Ctx(), mappingID)
		return nil, fmt.Errorf("add anonymous mapping to list failed: %w", err)
	}

	return mapping, nil
}

func (b *BaseCloudControl) GetAnonymousMappings() ([]*PortMapping, error) {
	return b.mappingRepo.ListUserMappings(b.Ctx(), "")
}

func (b *BaseCloudControl) CleanupExpiredAnonymous() error {
	// 这里可以实现清理逻辑
	return nil
}

// 节点管理
func (b *BaseCloudControl) GetNodeServiceInfo(nodeID string) (*NodeServiceInfo, error) {
	node, err := b.nodeRepo.GetNode(b.Ctx(), nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("node not found")
	}

	return &NodeServiceInfo{
		NodeID:  node.ID,
		Address: node.Address,
	}, nil
}

func (b *BaseCloudControl) GetAllNodeServiceInfo() ([]*NodeServiceInfo, error) {
	nodes, err := b.nodeRepo.ListNodes(b.Ctx())
	if err != nil {
		return nil, err
	}

	var nodeInfos []*NodeServiceInfo
	for _, node := range nodes {
		nodeInfo := &NodeServiceInfo{
			NodeID:  node.ID,
			Address: node.Address,
		}
		nodeInfos = append(nodeInfos, nodeInfo)
	}

	return nodeInfos, nil
}

// 统计相关
func (b *BaseCloudControl) GetUserStats(userID string) (*UserStats, error) {
	// 获取用户的客户端
	clients, err := b.clientRepo.ListUserClients(b.Ctx(), userID)
	if err != nil {
		return nil, err
	}

	// 获取用户的端口映射
	mappings, err := b.mappingRepo.ListUserMappings(b.Ctx(), userID)
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	totalClients := len(clients)
	onlineClients := 0
	totalMappings := len(mappings)
	activeMappings := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	var lastActive time.Time

	for _, client := range clients {
		if client.Status == ClientStatusOnline {
			onlineClients++
		}
		if client.LastSeen != nil && client.LastSeen.After(lastActive) {
			lastActive = *client.LastSeen
		}
	}

	for _, mapping := range mappings {
		if mapping.Status == MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	return &UserStats{
		UserID:           userID,
		TotalClients:     totalClients,
		OnlineClients:    onlineClients,
		TotalMappings:    totalMappings,
		ActiveMappings:   activeMappings,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		LastActive:       lastActive,
	}, nil
}

func (b *BaseCloudControl) GetClientStats(clientID int64) (*ClientStats, error) {
	client, err := b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", clientID))
	if err != nil {
		return nil, err
	}

	// 获取客户端的端口映射
	mappings, err := b.mappingRepo.ListClientMappings(b.Ctx(), fmt.Sprintf("%d", clientID))
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	totalMappings := len(mappings)
	activeMappings := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	uptime := int64(0)

	for _, mapping := range mappings {
		if mapping.Status == MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	// 计算在线时长
	if client.LastSeen != nil && client.Status == ClientStatusOnline {
		uptime = int64(time.Since(*client.LastSeen).Seconds())
	}

	return &ClientStats{
		ClientID:         clientID,
		UserID:           client.UserID,
		TotalMappings:    totalMappings,
		ActiveMappings:   activeMappings,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		Uptime:           uptime,
		LastSeen:         time.Now(),
	}, nil
}

func (b *BaseCloudControl) GetSystemStats() (*SystemStats, error) {
	// 获取所有用户
	users, err := b.userRepo.ListUsers(b.Ctx(), "")
	if err != nil {
		return nil, err
	}

	// 获取所有客户端
	clients, err := b.clientRepo.ListUserClients(b.Ctx(), "")
	if err != nil {
		return nil, err
	}

	// 获取所有端口映射
	mappings, err := b.mappingRepo.ListUserMappings(b.Ctx(), "")
	if err != nil {
		return nil, err
	}

	// 获取所有节点
	nodes, err := b.nodeRepo.ListNodes(b.Ctx())
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	totalUsers := len(users)
	totalClients := len(clients)
	onlineClients := 0
	totalMappings := len(mappings)
	activeMappings := 0
	totalNodes := len(nodes)
	onlineNodes := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	anonymousUsers := 0

	for _, client := range clients {
		if client.Status == ClientStatusOnline {
			onlineClients++
		}
		if client.Type == ClientTypeAnonymous {
			anonymousUsers++
		}
	}

	for _, mapping := range mappings {
		if mapping.Status == MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	// 简单假设所有节点都在线
	onlineNodes = totalNodes

	return &SystemStats{
		TotalUsers:       totalUsers,
		TotalClients:     totalClients,
		OnlineClients:    onlineClients,
		TotalMappings:    totalMappings,
		ActiveMappings:   activeMappings,
		TotalNodes:       totalNodes,
		OnlineNodes:      onlineNodes,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		AnonymousUsers:   anonymousUsers,
	}, nil
}

func (b *BaseCloudControl) GetTrafficStats(timeRange string) ([]*TrafficDataPoint, error) {
	// 简单实现：返回空数组
	return []*TrafficDataPoint{}, nil
}

func (b *BaseCloudControl) GetConnectionStats(timeRange string) ([]*ConnectionDataPoint, error) {
	// 简单实现：返回空数组
	return []*ConnectionDataPoint{}, nil
}

// 搜索相关
func (b *BaseCloudControl) SearchUsers(keyword string) ([]*User, error) {
	users, err := b.userRepo.ListUsers(b.Ctx(), "")
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

func (b *BaseCloudControl) SearchClients(keyword string) ([]*Client, error) {
	clients, err := b.clientRepo.ListUserClients(b.Ctx(), "")
	if err != nil {
		return nil, err
	}

	var results []*Client
	for _, client := range clients {
		if strings.Contains(strings.ToLower(client.Name), strings.ToLower(keyword)) ||
			strings.Contains(fmt.Sprintf("%d", client.ID), keyword) {
			results = append(results, client)
		}
	}

	return results, nil
}

func (b *BaseCloudControl) SearchPortMappings(keyword string) ([]*PortMapping, error) {
	mappings, err := b.mappingRepo.ListUserMappings(b.Ctx(), "")
	if err != nil {
		return nil, err
	}

	var results []*PortMapping
	for _, mapping := range mappings {
		if strings.Contains(mapping.ID, keyword) ||
			strings.Contains(mapping.TargetHost, keyword) ||
			strings.Contains(fmt.Sprintf("%d", mapping.SourcePort), keyword) ||
			strings.Contains(fmt.Sprintf("%d", mapping.TargetPort), keyword) {
			results = append(results, mapping)
		}
	}

	return results, nil
}

// 连接管理
func (b *BaseCloudControl) RegisterConnection(mappingId string, connInfo *ConnectionInfo) error {
	// 生成连接ID（使用映射ID作为前缀）
	connID, err := b.idGen.GenerateMappingID(b.Ctx())
	if err != nil {
		return fmt.Errorf("generate connection ID failed: %w", err)
	}

	connInfo.ConnId = connID
	connInfo.MappingId = mappingId
	connInfo.EstablishedAt = time.Now()
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()

	return b.connRepo.CreateConnection(b.Ctx(), connInfo)
}

func (b *BaseCloudControl) UnregisterConnection(connId string) error {
	return b.connRepo.DeleteConnection(b.Ctx(), connId)
}

func (b *BaseCloudControl) GetConnections(mappingId string) ([]*ConnectionInfo, error) {
	return b.connRepo.ListMappingConnections(b.Ctx(), mappingId)
}

func (b *BaseCloudControl) GetClientConnections(clientId int64) ([]*ConnectionInfo, error) {
	return b.connRepo.ListClientConnections(b.Ctx(), fmt.Sprintf("%d", clientId))
}

func (b *BaseCloudControl) UpdateConnectionStats(connId string, bytesSent, bytesReceived int64) error {
	return b.connRepo.UpdateConnectionStats(b.Ctx(), connId, bytesSent, bytesReceived)
}

// JWT管理
func (b *BaseCloudControl) GenerateJWTToken(clientId int64) (*JWTTokenInfo, error) {
	client, err := b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", clientId))
	if err != nil {
		return nil, err
	}
	return b.jwtManager.GenerateTokenPair(b.Ctx(), client)
}

func (b *BaseCloudControl) RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error) {
	// 验证刷新令牌
	claims, err := b.jwtManager.ValidateRefreshToken(b.Ctx(), refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// 获取客户端信息
	client, err := b.clientRepo.GetClient(b.Ctx(), claims.ClientID)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	// 生成新的令牌对
	return b.jwtManager.GenerateTokenPair(b.Ctx(), client)
}

func (b *BaseCloudControl) ValidateJWTToken(token string) (*JWTTokenInfo, error) {
	claims, err := b.jwtManager.ValidateAccessToken(b.Ctx(), token)
	if err != nil {
		return nil, err
	}

	client, err := b.clientRepo.GetClient(b.Ctx(), claims.ClientID)
	if err != nil {
		return nil, err
	}

	return &JWTTokenInfo{
		Token:    token,
		ClientId: client.ID,
		TokenID:  claims.ID,
	}, nil
}

func (b *BaseCloudControl) RevokeJWTToken(token string) error {
	// 验证令牌以获取客户端ID
	claims, err := b.jwtManager.ValidateAccessToken(b.Ctx(), token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// 将令牌加入黑名单
	return b.jwtManager.RevokeToken(b.Ctx(), claims.ID)
}

// 核心节点管理
func (b *BaseCloudControl) NodeRegister(req *NodeRegisterRequest) (*NodeRegisterResponse, error) {
	// 生成节点ID，确保不重复
	var nodeID string
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateNodeID(b.Ctx())
		if err != nil {
			return nil, fmt.Errorf("generate node ID failed: %w", err)
		}

		// 检查节点是否已存在
		existingNode, err := b.nodeRepo.GetNode(b.Ctx(), generatedID)
		if err != nil {
			// 节点不存在，可以使用这个ID
			nodeID = generatedID
			break
		}

		if existingNode != nil {
			// 节点已存在，释放ID并重试
			_ = b.idGen.ReleaseNodeID(b.Ctx(), generatedID)
			continue
		}

		nodeID = generatedID
		break
	}

	if nodeID == "" {
		return nil, fmt.Errorf("failed to generate unique node ID after %d attempts", DefaultMaxAttempts)
	}

	now := time.Now()
	node := &Node{
		ID:        nodeID,
		Name:      fmt.Sprintf("Node-%s", nodeID),
		Address:   req.Address,
		Meta:      req.Meta,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := b.nodeRepo.CreateNode(b.Ctx(), node); err != nil {
		// 如果保存失败，释放节点ID
		_ = b.idGen.ReleaseNodeID(b.Ctx(), nodeID)
		return nil, fmt.Errorf("save node failed: %w", err)
	}

	return &NodeRegisterResponse{
		NodeID:  nodeID,
		Success: true,
		Message: "Node registered successfully",
	}, nil
}

func (b *BaseCloudControl) NodeUnregister(req *NodeUnregisterRequest) error {
	// 获取节点信息，用于释放ID
	node, err := b.nodeRepo.GetNode(b.Ctx(), req.NodeID)
	if err == nil && node != nil {
		// 释放节点ID
		_ = b.idGen.ReleaseNodeID(b.Ctx(), req.NodeID)
	}
	return b.nodeRepo.DeleteNode(b.Ctx(), req.NodeID)
}

func (b *BaseCloudControl) NodeHeartbeat(req *NodeHeartbeatRequest) (*NodeHeartbeatResponse, error) {
	// 更新节点心跳时间
	node, err := b.nodeRepo.GetNode(b.Ctx(), req.NodeID)
	if err != nil {
		return &NodeHeartbeatResponse{
			Success: false,
			Message: "Node not found",
		}, nil
	}

	if node == nil {
		return &NodeHeartbeatResponse{
			Success: false,
			Message: "Node not found",
		}, nil
	}

	// 更新节点信息
	node.Address = req.Address
	node.UpdatedAt = time.Now()
	if err := b.nodeRepo.UpdateNode(b.Ctx(), node); err != nil {
		return &NodeHeartbeatResponse{
			Success: false,
			Message: "Failed to update node",
		}, nil
	}

	return &NodeHeartbeatResponse{
		Success: true,
		Message: "Heartbeat received",
	}, nil
}

func (b *BaseCloudControl) Authenticate(req *AuthRequest) (*AuthResponse, error) {
	// 获取客户端信息
	client, err := b.clientRepo.GetClient(b.Ctx(), fmt.Sprintf("%d", req.ClientID))
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	if client == nil {
		return &AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 验证认证码
	if client.AuthCode != req.AuthCode {
		return &AuthResponse{
			Success: false,
			Message: "Invalid auth code",
		}, nil
	}

	// 验证密钥（如果提供）
	if req.SecretKey != "" && client.SecretKey != req.SecretKey {
		return &AuthResponse{
			Success: false,
			Message: "Invalid secret key",
		}, nil
	}

	// 更新客户端状态
	client.Status = ClientStatusOnline
	client.NodeID = req.NodeID
	client.IPAddress = req.IPAddress
	client.Version = req.Version
	now := time.Now()
	client.LastSeen = &now
	client.UpdatedAt = now

	if err := b.clientRepo.UpdateClient(b.Ctx(), client); err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Failed to update client status",
		}, nil
	}

	// 生成JWT令牌
	tokenInfo, err := b.jwtManager.GenerateTokenPair(b.Ctx(), client)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Failed to generate token",
		}, nil
	}

	// 获取节点信息
	node, _ := b.nodeRepo.GetNode(b.Ctx(), req.NodeID)

	return &AuthResponse{
		Success:   true,
		Token:     tokenInfo.Token,
		Client:    client,
		Node:      node,
		ExpiresAt: tokenInfo.ExpiresAt,
		Message:   "Authentication successful",
	}, nil
}

func (b *BaseCloudControl) ValidateToken(token string) (*AuthResponse, error) {
	// 验证JWT令牌
	claims, err := b.jwtManager.ValidateAccessToken(b.Ctx(), token)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	// 获取客户端信息
	client, err := b.clientRepo.GetClient(b.Ctx(), claims.ClientID)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	if client == nil {
		return &AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 获取节点信息
	var node *Node
	if client.NodeID != "" {
		node, _ = b.nodeRepo.GetNode(b.Ctx(), client.NodeID)
	}

	return &AuthResponse{
		Success:   true,
		Token:     token,
		Client:    client,
		Node:      node,
		ExpiresAt: claims.ExpiresAt.Time,
		Message:   "Token validated successfully",
	}, nil
}
