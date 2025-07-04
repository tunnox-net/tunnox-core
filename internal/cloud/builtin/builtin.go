package builtin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tunnox-core/internal/cloud"
	"tunnox-core/internal/cloud/idgen"
	"tunnox-core/internal/cloud/repository"
	"tunnox-core/internal/cloud/storage"
)

// BuiltInCloudControl 内置云控实现
type BuiltInCloudControl struct {
	config        *cloud.CloudControlConfig
	idGen         *idgen.IDGenerator
	userRepo      *repository.UserRepository
	clientRepo    *repository.ClientRepository
	mappingRepo   *repository.PortMappingRepository
	nodeRepo      *repository.NodeRepository
	cleanupTicker *time.Ticker
	done          chan bool
}

// NewBuiltInCloudControl 创建新的内置云控
func NewBuiltInCloudControl(config *cloud.CloudControlConfig) *BuiltInCloudControl {
	storage := repository.NewRepository(storage.NewMemoryStorage())

	return &BuiltInCloudControl{
		config:        config,
		idGen:         idgen.NewIDGenerator(),
		userRepo:      repository.NewUserRepository(storage),
		clientRepo:    repository.NewClientRepository(storage),
		mappingRepo:   repository.NewPortMappingRepository(storage),
		nodeRepo:      repository.NewNodeRepository(storage),
		cleanupTicker: time.NewTicker(5 * time.Minute), // 每5分钟清理一次
		done:          make(chan bool),
	}
}

// Start 启动内置云控
func (b *BuiltInCloudControl) Start() {
	go b.cleanupRoutine()
}

// Stop 停止内置云控
func (b *BuiltInCloudControl) Stop() {
	b.cleanupTicker.Stop()
	close(b.done)
}

// NodeRegister 节点注册
func (b *BuiltInCloudControl) NodeRegister(ctx context.Context, req *cloud.NodeRegisterRequest) (*cloud.NodeRegisterResponse, error) {
	nodeID := req.NodeID
	if nodeID == "" {
		// 生成节点ID
		generatedID, err := b.idGen.GenerateUserID()
		if err != nil {
			return nil, fmt.Errorf("generate node ID failed: %w", err)
		}
		nodeID = generatedID
	}

	// 创建节点
	node := &cloud.Node{
		ID:        nodeID,
		Name:      fmt.Sprintf("Node-%s", nodeID[:8]),
		Address:   req.Address,
		Meta:      req.Meta,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 保存节点
	if err := b.nodeRepo.SaveNode(ctx, node); err != nil {
		return nil, fmt.Errorf("save node failed: %w", err)
	}

	// 添加到节点列表
	if err := b.nodeRepo.AddNodeToList(ctx, node); err != nil {
		return nil, fmt.Errorf("add node to list failed: %w", err)
	}

	return &cloud.NodeRegisterResponse{
		NodeID:  nodeID,
		Success: true,
		Message: "Node registered successfully",
	}, nil
}

// NodeUnregister 节点反注册
func (b *BuiltInCloudControl) NodeUnregister(ctx context.Context, req *cloud.NodeUnregisterRequest) error {
	return b.nodeRepo.DeleteNode(ctx, req.NodeID)
}

// NodeHeartbeat 节点心跳
func (b *BuiltInCloudControl) NodeHeartbeat(ctx context.Context, req *cloud.NodeHeartbeatRequest) (*cloud.NodeHeartbeatResponse, error) {
	node, err := b.nodeRepo.GetNode(ctx, req.NodeID)
	if err != nil {
		return &cloud.NodeHeartbeatResponse{
			Success: false,
			Message: "Node not found",
		}, nil
	}

	// 更新节点信息
	node.Address = req.Address
	node.UpdatedAt = time.Now()

	if err := b.nodeRepo.SaveNode(ctx, node); err != nil {
		return &cloud.NodeHeartbeatResponse{
			Success: false,
			Message: "Failed to update node",
		}, nil
	}

	return &cloud.NodeHeartbeatResponse{
		Success: true,
		Message: "Heartbeat received",
	}, nil
}

// Authenticate 用户认证
func (b *BuiltInCloudControl) Authenticate(ctx context.Context, req *cloud.AuthRequest) (*cloud.AuthResponse, error) {
	// 获取客户端
	client, err := b.clientRepo.GetClient(ctx, req.ClientID)
	if err != nil {
		return &cloud.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	// 验证认证码
	if client.AuthCode != req.AuthCode {
		return &cloud.AuthResponse{
			Success: false,
			Message: "Invalid auth code",
		}, nil
	}

	// 验证密钥（如果提供）
	if req.SecretKey != "" && client.SecretKey != req.SecretKey {
		return &cloud.AuthResponse{
			Success: false,
			Message: "Invalid secret key",
		}, nil
	}

	// 检查客户端状态
	if client.Status == cloud.ClientStatusBlocked {
		return &cloud.AuthResponse{
			Success: false,
			Message: "Client is blocked",
		}, nil
	}

	// 更新客户端状态
	now := time.Now()
	client.Status = cloud.ClientStatusOnline
	client.NodeID = req.NodeID
	client.IPAddress = req.IPAddress
	client.Version = req.Version
	client.LastSeen = &now
	client.UpdatedAt = now

	if err := b.clientRepo.SaveClient(ctx, client); err != nil {
		return &cloud.AuthResponse{
			Success: false,
			Message: "Failed to update client",
		}, nil
	}

	// 获取节点信息
	node, _ := b.nodeRepo.GetNode(ctx, req.NodeID)

	// 生成令牌（简单实现）
	token := fmt.Sprintf("token_%s_%d", client.ID, time.Now().Unix())

	return &cloud.AuthResponse{
		Success:   true,
		Token:     token,
		Client:    client,
		Node:      node,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Message:   "Authentication successful",
	}, nil
}

// ValidateToken 验证令牌
func (b *BuiltInCloudControl) ValidateToken(ctx context.Context, token string) (*cloud.AuthResponse, error) {
	// 简单实现：从令牌中提取客户端ID
	parts := strings.Split(token, "_")
	if len(parts) != 3 {
		return &cloud.AuthResponse{
			Success: false,
			Message: "Invalid token format",
		}, nil
	}

	clientID := parts[1]
	client, err := b.clientRepo.GetClient(ctx, clientID)
	if err != nil {
		return &cloud.AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	if client.Status != cloud.ClientStatusOnline {
		return &cloud.AuthResponse{
			Success: false,
			Message: "Client is not online",
		}, nil
	}

	node, _ := b.nodeRepo.GetNode(ctx, client.NodeID)

	return &cloud.AuthResponse{
		Success:   true,
		Token:     token,
		Client:    client,
		Node:      node,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Message:   "Token is valid",
	}, nil
}

// CreateUser 创建用户
func (b *BuiltInCloudControl) CreateUser(ctx context.Context, username, email string, userType cloud.UserType) (*cloud.User, error) {
	userID, err := b.idGen.GenerateUserID()
	if err != nil {
		return nil, fmt.Errorf("generate user ID failed: %w", err)
	}

	now := time.Now()
	user := &cloud.User{
		ID:        userID,
		Username:  username,
		Email:     email,
		Status:    cloud.UserStatusActive,
		Type:      userType,
		CreatedAt: now,
		UpdatedAt: now,
		Plan:      cloud.UserPlanFree,
		Quota: cloud.UserQuota{
			MaxClientIds:   10,
			MaxConnections: 100,
			BandwidthLimit: 1024 * 1024 * 100,  // 100MB/s
			StorageLimit:   1024 * 1024 * 1024, // 1GB
		},
	}

	if err := b.userRepo.SaveUser(ctx, user); err != nil {
		return nil, fmt.Errorf("save user failed: %w", err)
	}

	if err := b.userRepo.AddUserToList(ctx, user); err != nil {
		return nil, fmt.Errorf("add user to list failed: %w", err)
	}

	return user, nil
}

// GetUser 获取用户
func (b *BuiltInCloudControl) GetUser(ctx context.Context, userID string) (*cloud.User, error) {
	return b.userRepo.GetUser(ctx, userID)
}

// UpdateUser 更新用户
func (b *BuiltInCloudControl) UpdateUser(ctx context.Context, user *cloud.User) error {
	user.UpdatedAt = time.Now()
	return b.userRepo.SaveUser(ctx, user)
}

// DeleteUser 删除用户
func (b *BuiltInCloudControl) DeleteUser(ctx context.Context, userID string) error {
	return b.userRepo.DeleteUser(ctx, userID)
}

// ListUsers 列出用户
func (b *BuiltInCloudControl) ListUsers(ctx context.Context, userType cloud.UserType) ([]*cloud.User, error) {
	return b.userRepo.ListUsers(ctx, userType)
}

// CreateClient 创建客户端
func (b *BuiltInCloudControl) CreateClient(ctx context.Context, userID, clientName string) (*cloud.Client, error) {
	clientID, err := b.idGen.GenerateClientID()
	if err != nil {
		return nil, fmt.Errorf("generate client ID failed: %w", err)
	}

	authCode, err := b.idGen.GenerateAuthCode()
	if err != nil {
		return nil, fmt.Errorf("generate auth code failed: %w", err)
	}

	secretKey, err := b.idGen.GenerateSecretKey()
	if err != nil {
		return nil, fmt.Errorf("generate secret key failed: %w", err)
	}

	now := time.Now()
	client := &cloud.Client{
		ID:        fmt.Sprintf("%d", clientID),
		UserID:    userID,
		Name:      clientName,
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    cloud.ClientStatusOffline,
		Type:      cloud.ClientTypeRegistered,
		Config: cloud.ClientConfig{
			EnableCompression: true,
			BandwidthLimit:    1024 * 1024 * 10, // 10MB/s
			MaxConnections:    10,
			AllowedPorts:      []int{80, 443, 8080, 3000, 5000},
			BlockedPorts:      []int{22, 23, 25},
			AutoReconnect:     true,
			HeartbeatInterval: 30,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := b.clientRepo.SaveClient(ctx, client); err != nil {
		return nil, fmt.Errorf("save client failed: %w", err)
	}

	if userID != "" {
		if err := b.clientRepo.AddClientToUser(ctx, userID, client); err != nil {
			return nil, fmt.Errorf("add client to user failed: %w", err)
		}
	}

	return client, nil
}

// GetClient 获取客户端
func (b *BuiltInCloudControl) GetClient(ctx context.Context, clientID string) (*cloud.Client, error) {
	return b.clientRepo.GetClient(ctx, clientID)
}

// UpdateClient 更新客户端
func (b *BuiltInCloudControl) UpdateClient(ctx context.Context, client *cloud.Client) error {
	client.UpdatedAt = time.Now()
	return b.clientRepo.SaveClient(ctx, client)
}

// DeleteClient 删除客户端
func (b *BuiltInCloudControl) DeleteClient(ctx context.Context, clientID string) error {
	return b.clientRepo.DeleteClient(ctx, clientID)
}

// UpdateClientStatus 更新客户端状态
func (b *BuiltInCloudControl) UpdateClientStatus(ctx context.Context, clientID string, status cloud.ClientStatus, nodeID string) error {
	return b.clientRepo.UpdateClientStatus(ctx, clientID, status, nodeID)
}

// ListClients 列出客户端
func (b *BuiltInCloudControl) ListClients(ctx context.Context, userID string, clientType cloud.ClientType) ([]*cloud.Client, error) {
	if userID != "" {
		return b.clientRepo.ListUserClients(ctx, userID)
	}

	// 简单实现：返回所有客户端
	clients, err := b.clientRepo.ListUserClients(ctx, "")
	if err != nil {
		return nil, err
	}

	if clientType == "" {
		return clients, nil
	}

	var filtered []*cloud.Client
	for _, client := range clients {
		if client.Type == clientType {
			filtered = append(filtered, client)
		}
	}

	return filtered, nil
}

// GetUserClients 获取用户的客户端
func (b *BuiltInCloudControl) GetUserClients(ctx context.Context, userID string) ([]*cloud.Client, error) {
	return b.clientRepo.ListUserClients(ctx, userID)
}

// GetClientPortMappings 获取客户端的端口映射
func (b *BuiltInCloudControl) GetClientPortMappings(ctx context.Context, clientID string) ([]*cloud.PortMapping, error) {
	return b.mappingRepo.ListClientMappings(ctx, clientID)
}

// CreatePortMapping 创建端口映射
func (b *BuiltInCloudControl) CreatePortMapping(ctx context.Context, mapping *cloud.PortMapping) (*cloud.PortMapping, error) {
	mappingID, err := b.idGen.GenerateMappingID()
	if err != nil {
		return nil, fmt.Errorf("generate mapping ID failed: %w", err)
	}

	mapping.ID = mappingID
	mapping.Status = cloud.MappingStatusActive
	mapping.CreatedAt = time.Now()
	mapping.UpdatedAt = time.Now()

	if err := b.mappingRepo.SavePortMapping(ctx, mapping); err != nil {
		return nil, fmt.Errorf("save port mapping failed: %w", err)
	}

	if mapping.UserID != "" {
		if err := b.mappingRepo.AddMappingToUser(ctx, mapping.UserID, mapping); err != nil {
			return nil, fmt.Errorf("add mapping to user failed: %w", err)
		}
	}

	if err := b.mappingRepo.AddMappingToClient(ctx, mapping.SourceClientID, mapping); err != nil {
		return nil, fmt.Errorf("add mapping to source client failed: %w", err)
	}

	return mapping, nil
}

// GetPortMappings 获取端口映射
func (b *BuiltInCloudControl) GetPortMappings(ctx context.Context, userID string) ([]*cloud.PortMapping, error) {
	return b.mappingRepo.ListUserMappings(ctx, userID)
}

// GetPortMapping 获取端口映射
func (b *BuiltInCloudControl) GetPortMapping(ctx context.Context, mappingID string) (*cloud.PortMapping, error) {
	return b.mappingRepo.GetPortMapping(ctx, mappingID)
}

// UpdatePortMapping 更新端口映射
func (b *BuiltInCloudControl) UpdatePortMapping(ctx context.Context, mapping *cloud.PortMapping) error {
	mapping.UpdatedAt = time.Now()
	return b.mappingRepo.SavePortMapping(ctx, mapping)
}

// DeletePortMapping 删除端口映射
func (b *BuiltInCloudControl) DeletePortMapping(ctx context.Context, mappingID string) error {
	return b.mappingRepo.DeletePortMapping(ctx, mappingID)
}

// UpdatePortMappingStatus 更新端口映射状态
func (b *BuiltInCloudControl) UpdatePortMappingStatus(ctx context.Context, mappingID string, status cloud.MappingStatus) error {
	return b.mappingRepo.UpdatePortMappingStatus(ctx, mappingID, status)
}

// UpdatePortMappingStats 更新端口映射统计
func (b *BuiltInCloudControl) UpdatePortMappingStats(ctx context.Context, mappingID string, stats *cloud.TrafficStats) error {
	return b.mappingRepo.UpdatePortMappingStats(ctx, mappingID, stats)
}

// ListPortMappings 列出端口映射
func (b *BuiltInCloudControl) ListPortMappings(ctx context.Context, mappingType cloud.MappingType) ([]*cloud.PortMapping, error) {
	// 简单实现：返回所有映射
	return b.mappingRepo.ListUserMappings(ctx, "")
}

// GenerateAnonymousCredentials 生成匿名客户端凭据
func (b *BuiltInCloudControl) GenerateAnonymousCredentials(ctx context.Context) (*cloud.Client, error) {
	clientID, err := b.idGen.GenerateClientID()
	if err != nil {
		return nil, fmt.Errorf("generate client ID failed: %w", err)
	}

	authCode, err := b.idGen.GenerateAuthCode()
	if err != nil {
		return nil, fmt.Errorf("generate auth code failed: %w", err)
	}

	secretKey, err := b.idGen.GenerateSecretKey()
	if err != nil {
		return nil, fmt.Errorf("generate secret key failed: %w", err)
	}

	now := time.Now()
	client := &cloud.Client{
		ID:        fmt.Sprintf("%d", clientID),
		UserID:    "", // 匿名用户没有UserID
		Name:      fmt.Sprintf("Anonymous-%s", authCode),
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    cloud.ClientStatusOffline,
		Type:      cloud.ClientTypeAnonymous,
		Config: cloud.ClientConfig{
			EnableCompression: true,
			BandwidthLimit:    1024 * 1024 * 5, // 5MB/s
			MaxConnections:    5,
			AllowedPorts:      []int{80, 443, 8080},
			BlockedPorts:      []int{22, 23, 25},
			AutoReconnect:     true,
			HeartbeatInterval: 30,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := b.clientRepo.SaveClient(ctx, client); err != nil {
		return nil, fmt.Errorf("save anonymous client failed: %w", err)
	}

	return client, nil
}

// GetAnonymousClient 获取匿名客户端
func (b *BuiltInCloudControl) GetAnonymousClient(ctx context.Context, clientID string) (*cloud.Client, error) {
	client, err := b.clientRepo.GetClient(ctx, clientID)
	if err != nil {
		return nil, err
	}

	if client.Type != cloud.ClientTypeAnonymous {
		return nil, fmt.Errorf("client is not anonymous")
	}

	return client, nil
}

// DeleteAnonymousClient 删除匿名客户端
func (b *BuiltInCloudControl) DeleteAnonymousClient(ctx context.Context, clientID string) error {
	return b.clientRepo.DeleteClient(ctx, clientID)
}

// ListAnonymousClients 列出匿名客户端
func (b *BuiltInCloudControl) ListAnonymousClients(ctx context.Context) ([]*cloud.Client, error) {
	return b.ListClients(ctx, "", cloud.ClientTypeAnonymous)
}

// CreateAnonymousMapping 创建匿名端口映射
func (b *BuiltInCloudControl) CreateAnonymousMapping(ctx context.Context, sourceClientID, targetClientID string, protocol cloud.Protocol, sourcePort, targetPort int) (*cloud.PortMapping, error) {
	mappingID, err := b.idGen.GenerateMappingID()
	if err != nil {
		return nil, fmt.Errorf("generate mapping ID failed: %w", err)
	}

	now := time.Now()
	mapping := &cloud.PortMapping{
		ID:             mappingID,
		UserID:         "", // 匿名映射没有UserID
		SourceClientID: sourceClientID,
		TargetClientID: targetClientID,
		Protocol:       protocol,
		SourcePort:     sourcePort,
		TargetHost:     "localhost",
		TargetPort:     targetPort,
		Status:         cloud.MappingStatusActive,
		Type:           cloud.MappingTypeAnonymous,
		Config: cloud.MappingConfig{
			EnableCompression: true,
			BandwidthLimit:    1024 * 1024 * 5, // 5MB/s
			Timeout:           30,
			RetryCount:        3,
		},
		CreatedAt:    now,
		UpdatedAt:    now,
		TrafficStats: cloud.TrafficStats{},
	}

	if err := b.mappingRepo.SavePortMapping(ctx, mapping); err != nil {
		return nil, fmt.Errorf("save anonymous mapping failed: %w", err)
	}

	if err := b.mappingRepo.AddMappingToClient(ctx, sourceClientID, mapping); err != nil {
		return nil, fmt.Errorf("add mapping to source client failed: %w", err)
	}

	return mapping, nil
}

// GetAnonymousMappings 获取匿名端口映射
func (b *BuiltInCloudControl) GetAnonymousMappings(ctx context.Context) ([]*cloud.PortMapping, error) {
	// 简单实现：返回所有映射
	return b.mappingRepo.ListUserMappings(ctx, "")
}

// CleanupExpiredAnonymous 清理过期的匿名数据
func (b *BuiltInCloudControl) CleanupExpiredAnonymous(ctx context.Context) error {
	// 这里可以实现更复杂的清理逻辑
	// 目前依赖存储层的自动过期机制
	return nil
}

// GetNodeServiceInfo 获取节点服务信息
func (b *BuiltInCloudControl) GetNodeServiceInfo(ctx context.Context, nodeID string) (*cloud.NodeServiceInfo, error) {
	node, err := b.nodeRepo.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	return &cloud.NodeServiceInfo{
		NodeID:  node.ID,
		Address: node.Address,
	}, nil
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (b *BuiltInCloudControl) GetAllNodeServiceInfo(ctx context.Context) ([]*cloud.NodeServiceInfo, error) {
	nodes, err := b.nodeRepo.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	var infos []*cloud.NodeServiceInfo
	for _, node := range nodes {
		infos = append(infos, &cloud.NodeServiceInfo{
			NodeID:  node.ID,
			Address: node.Address,
		})
	}

	return infos, nil
}

// GetUserStats 获取用户统计信息
func (b *BuiltInCloudControl) GetUserStats(ctx context.Context, userID string) (*cloud.UserStats, error) {
	clients, err := b.clientRepo.ListUserClients(ctx, userID)
	if err != nil {
		return nil, err
	}

	mappings, err := b.mappingRepo.ListUserMappings(ctx, userID)
	if err != nil {
		return nil, err
	}

	onlineClients := 0
	activeMappings := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	lastActive := time.Time{}

	for _, client := range clients {
		if client.Status == cloud.ClientStatusOnline {
			onlineClients++
		}
		if client.LastSeen != nil && client.LastSeen.After(lastActive) {
			lastActive = *client.LastSeen
		}
	}

	for _, mapping := range mappings {
		if mapping.Status == cloud.MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
		if mapping.LastActive != nil && mapping.LastActive.After(lastActive) {
			lastActive = *mapping.LastActive
		}
	}

	return &cloud.UserStats{
		UserID:           userID,
		TotalClients:     len(clients),
		OnlineClients:    onlineClients,
		TotalMappings:    len(mappings),
		ActiveMappings:   activeMappings,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		LastActive:       lastActive,
	}, nil
}

// GetClientStats 获取客户端统计信息
func (b *BuiltInCloudControl) GetClientStats(ctx context.Context, clientID string) (*cloud.ClientStats, error) {
	client, err := b.clientRepo.GetClient(ctx, clientID)
	if err != nil {
		return nil, err
	}

	mappings, err := b.mappingRepo.ListClientMappings(ctx, clientID)
	if err != nil {
		return nil, err
	}

	activeMappings := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	uptime := int64(0)

	if client.LastSeen != nil && client.CreatedAt.Before(*client.LastSeen) {
		uptime = int64(client.LastSeen.Sub(client.CreatedAt).Seconds())
	}

	for _, mapping := range mappings {
		if mapping.Status == cloud.MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	return &cloud.ClientStats{
		ClientID:         clientID,
		UserID:           client.UserID,
		TotalMappings:    len(mappings),
		ActiveMappings:   activeMappings,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		Uptime:           uptime,
		LastSeen:         *client.LastSeen,
	}, nil
}

// GetSystemStats 获取系统整体统计
func (b *BuiltInCloudControl) GetSystemStats(ctx context.Context) (*cloud.SystemStats, error) {
	users, err := b.userRepo.ListUsers(ctx, "")
	if err != nil {
		return nil, err
	}

	clients, err := b.clientRepo.ListUserClients(ctx, "")
	if err != nil {
		return nil, err
	}

	mappings, err := b.mappingRepo.ListUserMappings(ctx, "")
	if err != nil {
		return nil, err
	}

	nodes, err := b.nodeRepo.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	onlineClients := 0
	activeMappings := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	anonymousUsers := 0

	for _, client := range clients {
		if client.Status == cloud.ClientStatusOnline {
			onlineClients++
		}
		if client.Type == cloud.ClientTypeAnonymous {
			anonymousUsers++
		}
	}

	for _, mapping := range mappings {
		if mapping.Status == cloud.MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	onlineNodes := 0
	for _, node := range nodes {
		if time.Since(node.UpdatedAt) < 5*time.Minute {
			onlineNodes++
		}
	}

	return &cloud.SystemStats{
		TotalUsers:       len(users),
		TotalClients:     len(clients),
		OnlineClients:    onlineClients,
		TotalMappings:    len(mappings),
		ActiveMappings:   activeMappings,
		TotalNodes:       len(nodes),
		OnlineNodes:      onlineNodes,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		AnonymousUsers:   anonymousUsers,
	}, nil
}

// GetTrafficStats 获取流量统计图表数据
func (b *BuiltInCloudControl) GetTrafficStats(ctx context.Context, timeRange string) ([]*cloud.TrafficDataPoint, error) {
	// 简单实现：返回空数据
	return []*cloud.TrafficDataPoint{}, nil
}

// GetConnectionStats 获取连接数统计图表数据
func (b *BuiltInCloudControl) GetConnectionStats(ctx context.Context, timeRange string) ([]*cloud.ConnectionDataPoint, error) {
	// 简单实现：返回空数据
	return []*cloud.ConnectionDataPoint{}, nil
}

// SearchUsers 搜索用户
func (b *BuiltInCloudControl) SearchUsers(ctx context.Context, keyword string) ([]*cloud.User, error) {
	users, err := b.userRepo.ListUsers(ctx, "")
	if err != nil {
		return nil, err
	}

	var results []*cloud.User
	for _, user := range users {
		if strings.Contains(strings.ToLower(user.Username), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(user.Email), strings.ToLower(keyword)) {
			results = append(results, user)
		}
	}

	return results, nil
}

// SearchClients 搜索客户端
func (b *BuiltInCloudControl) SearchClients(ctx context.Context, keyword string) ([]*cloud.Client, error) {
	clients, err := b.clientRepo.ListUserClients(ctx, "")
	if err != nil {
		return nil, err
	}

	var results []*cloud.Client
	for _, client := range clients {
		if strings.Contains(strings.ToLower(client.ID), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(client.Name), strings.ToLower(keyword)) {
			results = append(results, client)
		}
	}

	return results, nil
}

// SearchPortMappings 搜索端口映射
func (b *BuiltInCloudControl) SearchPortMappings(ctx context.Context, keyword string) ([]*cloud.PortMapping, error) {
	mappings, err := b.mappingRepo.ListUserMappings(ctx, "")
	if err != nil {
		return nil, err
	}

	var results []*cloud.PortMapping
	for _, mapping := range mappings {
		if strings.Contains(strings.ToLower(mapping.ID), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(mapping.SourceClientID), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(mapping.TargetClientID), strings.ToLower(keyword)) {
			results = append(results, mapping)
		}
	}

	return results, nil
}

// Close 关闭内置云控
func (b *BuiltInCloudControl) Close() error {
	b.Stop()
	return nil
}

// cleanupRoutine 清理过期数据的协程
func (b *BuiltInCloudControl) cleanupRoutine() {
	for {
		select {
		case <-b.cleanupTicker.C:
			ctx := context.Background()
			b.CleanupExpiredAnonymous(ctx)
		case <-b.done:
			return
		}
	}
}
