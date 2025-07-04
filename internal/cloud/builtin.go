package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BuiltInCloudControl 内置云控实现
type BuiltInCloudControl struct {
	config        *CloudControlConfig
	idGen         *IDGenerator
	userRepo      *UserRepository
	clientRepo    *ClientRepository
	mappingRepo   *PortMappingRepository
	nodeRepo      *NodeRepository
	jwtManager    *JWTManager
	cleanupTicker *time.Ticker
	done          chan bool
}

// NewBuiltInCloudControl 创建新的内置云控
func NewBuiltInCloudControl(config *CloudControlConfig) *BuiltInCloudControl {
	memoryStorage := NewMemoryStorage()
	storage := NewRepository(memoryStorage)

	return &BuiltInCloudControl{
		config:        config,
		idGen:         NewIDGenerator(),
		userRepo:      NewUserRepository(storage),
		clientRepo:    NewClientRepository(storage),
		mappingRepo:   NewPortMappingRepository(storage),
		nodeRepo:      NewNodeRepository(storage),
		jwtManager:    NewJWTManager(config, memoryStorage),
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
func (b *BuiltInCloudControl) NodeRegister(ctx context.Context, req *NodeRegisterRequest) (*NodeRegisterResponse, error) {
	nodeID := req.NodeID
	if nodeID == "" {
		// 生成节点ID
		generatedID, err := b.idGen.GenerateTerminalID()
		if err != nil {
			return nil, fmt.Errorf("generate node ID failed: %w", err)
		}
		nodeID = generatedID
	}

	// 创建节点
	node := &Node{
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

	return &NodeRegisterResponse{
		NodeID:  nodeID,
		Success: true,
		Message: "Node registered successfully",
	}, nil
}

// NodeUnregister 节点反注册
func (b *BuiltInCloudControl) NodeUnregister(ctx context.Context, req *NodeUnregisterRequest) error {
	return b.nodeRepo.DeleteNode(ctx, req.NodeID)
}

// NodeHeartbeat 节点心跳
func (b *BuiltInCloudControl) NodeHeartbeat(ctx context.Context, req *NodeHeartbeatRequest) (*NodeHeartbeatResponse, error) {
	node, err := b.nodeRepo.GetNode(ctx, req.NodeID)
	if err != nil {
		return &NodeHeartbeatResponse{
			Success: false,
			Message: "Node not found",
		}, nil
	}

	// 更新节点信息
	node.Address = req.Address
	node.UpdatedAt = time.Now()

	if err := b.nodeRepo.SaveNode(ctx, node); err != nil {
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

// Authenticate 用户认证
func (b *BuiltInCloudControl) Authenticate(ctx context.Context, req *AuthRequest) (*AuthResponse, error) {
	// 获取客户端
	client, err := b.clientRepo.GetClient(ctx, req.ClientID)
	if err != nil {
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

	// 检查客户端状态
	if client.Status == ClientStatusBlocked {
		return &AuthResponse{
			Success: false,
			Message: "Client is blocked",
		}, nil
	}

	// 更新客户端状态
	now := time.Now()
	client.Status = ClientStatusOnline
	client.NodeID = req.NodeID
	client.IPAddress = req.IPAddress
	client.Version = req.Version
	client.LastSeen = &now
	client.UpdatedAt = now

	if err := b.clientRepo.SaveClient(ctx, client); err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Failed to update client",
		}, nil
	}

	// 获取节点信息
	node, _ := b.nodeRepo.GetNode(ctx, req.NodeID)

	// 生成JWT Token
	jwtInfo, err := b.jwtManager.GenerateTokenPair(ctx, client)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Failed to generate JWT token",
		}, nil
	}

	// 更新客户端Token信息
	client.JWTToken = jwtInfo.TokenID
	client.TokenExpiresAt = &jwtInfo.ExpiresAt
	client.RefreshToken = jwtInfo.RefreshToken
	client.UpdatedAt = time.Now()

	if err := b.clientRepo.SaveClient(ctx, client); err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Failed to update client token",
		}, nil
	}

	return &AuthResponse{
		Success:   true,
		Token:     jwtInfo.Token,
		Client:    client,
		Node:      node,
		ExpiresAt: jwtInfo.ExpiresAt,
		Message:   "Authentication successful",
	}, nil
}

// ValidateToken 验证令牌
func (b *BuiltInCloudControl) ValidateToken(ctx context.Context, token string) (*AuthResponse, error) {
	// 使用JWT Token验证
	claims, err := b.jwtManager.ValidateAccessToken(ctx, token)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	client, err := b.clientRepo.GetClient(ctx, claims.ClientID)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	if client.Status != ClientStatusOnline {
		return &AuthResponse{
			Success: false,
			Message: "Client is not online",
		}, nil
	}

	// 验证Token ID是否匹配
	if client.JWTToken != claims.ID {
		return &AuthResponse{
			Success: false,
			Message: "Token has been revoked",
		}, nil
	}

	node, _ := b.nodeRepo.GetNode(ctx, client.NodeID)

	return &AuthResponse{
		Success:   true,
		Token:     token,
		Client:    client,
		Node:      node,
		ExpiresAt: claims.ExpiresAt.Time,
		Message:   "Token is valid",
	}, nil
}

// CreateUser 创建用户
func (b *BuiltInCloudControl) CreateUser(ctx context.Context, username, email string, userType UserType) (*User, error) {
	userID, err := b.idGen.GenerateTerminalID()
	if err != nil {
		return nil, fmt.Errorf("generate user ID failed: %w", err)
	}

	now := time.Now()
	user := &User{
		ID:        userID,
		Username:  username,
		Email:     email,
		Status:    UserStatusActive,
		Type:      userType,
		CreatedAt: now,
		UpdatedAt: now,
		Plan:      UserPlanFree,
		Quota: UserQuota{
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
func (b *BuiltInCloudControl) GetUser(ctx context.Context, userID string) (*User, error) {
	return b.userRepo.GetUser(ctx, userID)
}

// UpdateUser 更新用户
func (b *BuiltInCloudControl) UpdateUser(ctx context.Context, user *User) error {
	user.UpdatedAt = time.Now()
	return b.userRepo.SaveUser(ctx, user)
}

// DeleteUser 删除用户
func (b *BuiltInCloudControl) DeleteUser(ctx context.Context, userID string) error {
	return b.userRepo.DeleteUser(ctx, userID)
}

// ListUsers 列出用户
func (b *BuiltInCloudControl) ListUsers(ctx context.Context, userType UserType) ([]*User, error) {
	return b.userRepo.ListUsers(ctx, userType)
}

// CreateClient 创建客户端
func (b *BuiltInCloudControl) CreateClient(ctx context.Context, userID, clientName string) (*Client, error) {
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
	client := &Client{
		ID:        fmt.Sprintf("%d", clientID),
		UserID:    userID,
		Name:      clientName,
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    ClientStatusOffline,
		Type:      ClientTypeRegistered,
		Config: ClientConfig{
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

	// 强制添加到用户列表（即使 userID 为空也加到匿名列表）
	if err := b.clientRepo.AddClientToUser(ctx, userID, client); err != nil {
		return nil, fmt.Errorf("add client to user failed: %w", err)
	}

	return client, nil
}

// GetClient 获取客户端
func (b *BuiltInCloudControl) GetClient(ctx context.Context, clientID string) (*Client, error) {
	return b.clientRepo.GetClient(ctx, clientID)
}

// UpdateClient 更新客户端
func (b *BuiltInCloudControl) UpdateClient(ctx context.Context, client *Client) error {
	client.UpdatedAt = time.Now()
	return b.clientRepo.SaveClient(ctx, client)
}

// DeleteClient 删除客户端
func (b *BuiltInCloudControl) DeleteClient(ctx context.Context, clientID string) error {
	return b.clientRepo.DeleteClient(ctx, clientID)
}

// UpdateClientStatus 更新客户端状态
func (b *BuiltInCloudControl) UpdateClientStatus(ctx context.Context, clientID string, status ClientStatus, nodeID string) error {
	return b.clientRepo.UpdateClientStatus(ctx, clientID, status, nodeID)
}

// ListClients 列出客户端
func (b *BuiltInCloudControl) ListClients(ctx context.Context, userID string, clientType ClientType) ([]*Client, error) {
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

	var filtered []*Client
	for _, client := range clients {
		if client.Type == clientType {
			filtered = append(filtered, client)
		}
	}

	return filtered, nil
}

// GetUserClients 获取用户的客户端
func (b *BuiltInCloudControl) GetUserClients(ctx context.Context, userID string) ([]*Client, error) {
	return b.clientRepo.ListUserClients(ctx, userID)
}

// GetClientPortMappings 获取客户端的端口映射
func (b *BuiltInCloudControl) GetClientPortMappings(ctx context.Context, clientID string) ([]*PortMapping, error) {
	return b.mappingRepo.ListClientMappings(ctx, clientID)
}

// CreatePortMapping 创建端口映射
func (b *BuiltInCloudControl) CreatePortMapping(ctx context.Context, mapping *PortMapping) (*PortMapping, error) {
	mappingID, err := b.idGen.GenerateMappingID()
	if err != nil {
		return nil, fmt.Errorf("generate mapping ID failed: %w", err)
	}

	mapping.ID = mappingID
	mapping.Status = MappingStatusActive
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
func (b *BuiltInCloudControl) GetPortMappings(ctx context.Context, userID string) ([]*PortMapping, error) {
	return b.mappingRepo.ListUserMappings(ctx, userID)
}

// GetPortMapping 获取端口映射
func (b *BuiltInCloudControl) GetPortMapping(ctx context.Context, mappingID string) (*PortMapping, error) {
	return b.mappingRepo.GetPortMapping(ctx, mappingID)
}

// UpdatePortMapping 更新端口映射
func (b *BuiltInCloudControl) UpdatePortMapping(ctx context.Context, mapping *PortMapping) error {
	mapping.UpdatedAt = time.Now()
	return b.mappingRepo.SavePortMapping(ctx, mapping)
}

// DeletePortMapping 删除端口映射
func (b *BuiltInCloudControl) DeletePortMapping(ctx context.Context, mappingID string) error {
	return b.mappingRepo.DeletePortMapping(ctx, mappingID)
}

// UpdatePortMappingStatus 更新端口映射状态
func (b *BuiltInCloudControl) UpdatePortMappingStatus(ctx context.Context, mappingID string, status MappingStatus) error {
	return b.mappingRepo.UpdatePortMappingStatus(ctx, mappingID, status)
}

// UpdatePortMappingStats 更新端口映射统计
func (b *BuiltInCloudControl) UpdatePortMappingStats(ctx context.Context, mappingID string, stats *TrafficStats) error {
	return b.mappingRepo.UpdatePortMappingStats(ctx, mappingID, stats)
}

// ListPortMappings 列出端口映射
func (b *BuiltInCloudControl) ListPortMappings(ctx context.Context, mappingType MappingType) ([]*PortMapping, error) {
	// 简单实现：返回所有映射
	return b.mappingRepo.ListUserMappings(ctx, "")
}

// GenerateAnonymousCredentials 生成匿名客户端凭据
func (b *BuiltInCloudControl) GenerateAnonymousCredentials(ctx context.Context) (*Client, error) {
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
	client := &Client{
		ID:        fmt.Sprintf("%d", clientID),
		UserID:    "", // 匿名用户没有UserID
		Name:      fmt.Sprintf("Anonymous-%s", authCode),
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    ClientStatusOffline,
		Type:      ClientTypeAnonymous,
		Config: ClientConfig{
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
func (b *BuiltInCloudControl) GetAnonymousClient(ctx context.Context, clientID string) (*Client, error) {
	client, err := b.clientRepo.GetClient(ctx, clientID)
	if err != nil {
		return nil, err
	}

	if client.Type != ClientTypeAnonymous {
		return nil, fmt.Errorf("client is not anonymous")
	}

	return client, nil
}

// DeleteAnonymousClient 删除匿名客户端
func (b *BuiltInCloudControl) DeleteAnonymousClient(ctx context.Context, clientID string) error {
	return b.clientRepo.DeleteClient(ctx, clientID)
}

// ListAnonymousClients 列出匿名客户端
func (b *BuiltInCloudControl) ListAnonymousClients(ctx context.Context) ([]*Client, error) {
	return b.ListClients(ctx, "", ClientTypeAnonymous)
}

// CreateAnonymousMapping 创建匿名端口映射
func (b *BuiltInCloudControl) CreateAnonymousMapping(ctx context.Context, sourceClientID, targetClientID string, protocol Protocol, sourcePort, targetPort int) (*PortMapping, error) {
	mappingID, err := b.idGen.GenerateMappingID()
	if err != nil {
		return nil, fmt.Errorf("generate mapping ID failed: %w", err)
	}

	now := time.Now()
	mapping := &PortMapping{
		ID:             mappingID,
		UserID:         "", // 匿名映射没有UserID
		SourceClientID: sourceClientID,
		TargetClientID: targetClientID,
		Protocol:       protocol,
		SourcePort:     sourcePort,
		TargetHost:     "localhost",
		TargetPort:     targetPort,
		Status:         MappingStatusActive,
		Type:           MappingTypeAnonymous,
		Config: MappingConfig{
			EnableCompression: true,
			BandwidthLimit:    1024 * 1024 * 5, // 5MB/s
			Timeout:           30,
			RetryCount:        3,
		},
		CreatedAt:    now,
		UpdatedAt:    now,
		TrafficStats: TrafficStats{},
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
func (b *BuiltInCloudControl) GetAnonymousMappings(ctx context.Context) ([]*PortMapping, error) {
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
func (b *BuiltInCloudControl) GetNodeServiceInfo(ctx context.Context, nodeID string) (*NodeServiceInfo, error) {
	node, err := b.nodeRepo.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	return &NodeServiceInfo{
		NodeID:  node.ID,
		Address: node.Address,
	}, nil
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (b *BuiltInCloudControl) GetAllNodeServiceInfo(ctx context.Context) ([]*NodeServiceInfo, error) {
	nodes, err := b.nodeRepo.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	var infos []*NodeServiceInfo
	for _, node := range nodes {
		infos = append(infos, &NodeServiceInfo{
			NodeID:  node.ID,
			Address: node.Address,
		})
	}

	return infos, nil
}

// GetUserStats 获取用户统计信息
func (b *BuiltInCloudControl) GetUserStats(ctx context.Context, userID string) (*UserStats, error) {
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
		if mapping.LastActive != nil && mapping.LastActive.After(lastActive) {
			lastActive = *mapping.LastActive
		}
	}

	return &UserStats{
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
func (b *BuiltInCloudControl) GetClientStats(ctx context.Context, clientID string) (*ClientStats, error) {
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
		if mapping.Status == MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	return &ClientStats{
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
func (b *BuiltInCloudControl) GetSystemStats(ctx context.Context) (*SystemStats, error) {
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

	onlineNodes := 0
	for _, node := range nodes {
		if time.Since(node.UpdatedAt) < 5*time.Minute {
			onlineNodes++
		}
	}

	return &SystemStats{
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
func (b *BuiltInCloudControl) GetTrafficStats(ctx context.Context, timeRange string) ([]*TrafficDataPoint, error) {
	// 简单实现：返回空数据
	return []*TrafficDataPoint{}, nil
}

// GetConnectionStats 获取连接数统计图表数据
func (b *BuiltInCloudControl) GetConnectionStats(ctx context.Context, timeRange string) ([]*ConnectionDataPoint, error) {
	// 简单实现：返回空数据
	return []*ConnectionDataPoint{}, nil
}

// SearchUsers 搜索用户
func (b *BuiltInCloudControl) SearchUsers(ctx context.Context, keyword string) ([]*User, error) {
	users, err := b.userRepo.ListUsers(ctx, "")
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
func (b *BuiltInCloudControl) SearchClients(ctx context.Context, keyword string) ([]*Client, error) {
	clients, err := b.clientRepo.ListUserClients(ctx, "")
	if err != nil {
		return nil, err
	}

	var results []*Client
	for _, client := range clients {
		if strings.Contains(strings.ToLower(client.ID), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(client.Name), strings.ToLower(keyword)) {
			results = append(results, client)
		}
	}

	return results, nil
}

// SearchPortMappings 搜索端口映射
func (b *BuiltInCloudControl) SearchPortMappings(ctx context.Context, keyword string) ([]*PortMapping, error) {
	mappings, err := b.mappingRepo.ListUserMappings(ctx, "")
	if err != nil {
		return nil, err
	}

	var results []*PortMapping
	for _, mapping := range mappings {
		if strings.Contains(strings.ToLower(mapping.ID), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(mapping.SourceClientID), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(mapping.TargetClientID), strings.ToLower(keyword)) {
			results = append(results, mapping)
		}
	}

	return results, nil
}

// RegisterConnection 注册连接
func (b *BuiltInCloudControl) RegisterConnection(ctx context.Context, mappingId string, connInfo *ConnectionInfo) error {
	// 简单实现：保存连接信息
	key := fmt.Sprintf("connection:%s", connInfo.ConnId)
	data, err := json.Marshal(connInfo)
	if err != nil {
		return err
	}

	storage := NewMemoryStorage()
	return storage.Set(ctx, key, string(data), 24*time.Hour)
}

// UnregisterConnection 注销连接
func (b *BuiltInCloudControl) UnregisterConnection(ctx context.Context, connId string) error {
	key := fmt.Sprintf("connection:%s", connId)
	storage := NewMemoryStorage()
	return storage.Delete(ctx, key)
}

// GetConnections 获取映射的连接列表
func (b *BuiltInCloudControl) GetConnections(ctx context.Context, mappingId string) ([]*ConnectionInfo, error) {
	// 简单实现：返回空列表
	return []*ConnectionInfo{}, nil
}

// GetClientConnections 获取客户端的连接列表
func (b *BuiltInCloudControl) GetClientConnections(ctx context.Context, clientId string) ([]*ConnectionInfo, error) {
	// 简单实现：返回空列表
	return []*ConnectionInfo{}, nil
}

// UpdateConnectionStats 更新连接统计
func (b *BuiltInCloudControl) UpdateConnectionStats(ctx context.Context, connId string, bytesSent, bytesReceived int64) error {
	// 简单实现：更新连接统计
	return nil
}

// GenerateJWTToken 生成JWT Token
func (b *BuiltInCloudControl) GenerateJWTToken(ctx context.Context, clientId string) (*JWTTokenInfo, error) {
	client, err := b.clientRepo.GetClient(ctx, clientId)
	if err != nil {
		return nil, err
	}

	jwtInfo, err := b.jwtManager.GenerateTokenPair(ctx, client)
	if err != nil {
		return nil, err
	}

	// 更新客户端Token信息
	client.JWTToken = jwtInfo.TokenID
	client.TokenExpiresAt = &jwtInfo.ExpiresAt
	client.RefreshToken = jwtInfo.RefreshToken
	client.UpdatedAt = time.Now()

	if err := b.clientRepo.SaveClient(ctx, client); err != nil {
		return nil, fmt.Errorf("failed to update client token: %w", err)
	}

	return jwtInfo, nil
}

// RefreshJWTToken 刷新JWT Token
func (b *BuiltInCloudControl) RefreshJWTToken(ctx context.Context, refreshToken string) (*JWTTokenInfo, error) {
	// 验证刷新Token并获取客户端ID
	refreshClaims, err := b.jwtManager.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// 获取客户端
	client, err := b.clientRepo.GetClient(ctx, refreshClaims.ClientID)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	// 验证Token ID是否匹配
	if client.JWTToken != refreshClaims.TokenID {
		return nil, fmt.Errorf("token ID mismatch")
	}

	// 生成新的Token对
	jwtInfo, err := b.jwtManager.GenerateTokenPair(ctx, client)
	if err != nil {
		return nil, err
	}

	// 更新客户端Token信息
	client.JWTToken = jwtInfo.TokenID
	client.TokenExpiresAt = &jwtInfo.ExpiresAt
	client.RefreshToken = jwtInfo.RefreshToken
	client.UpdatedAt = time.Now()

	if err := b.clientRepo.SaveClient(ctx, client); err != nil {
		return nil, fmt.Errorf("failed to update client token: %w", err)
	}

	return jwtInfo, nil
}

// ValidateJWTToken 验证JWT Token
func (b *BuiltInCloudControl) ValidateJWTToken(ctx context.Context, token string) (*JWTTokenInfo, error) {
	// 验证访问Token
	claims, err := b.jwtManager.ValidateAccessToken(ctx, token)
	if err != nil {
		return nil, err
	}

	// 获取客户端
	client, err := b.clientRepo.GetClient(ctx, claims.ClientID)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	// 验证Token ID是否匹配
	if client.JWTToken != claims.ID {
		return nil, fmt.Errorf("token has been revoked")
	}

	return &JWTTokenInfo{
		Token:        token,
		RefreshToken: client.RefreshToken,
		ExpiresAt:    claims.ExpiresAt.Time,
		ClientId:     claims.ClientID,
		TokenID:      claims.ID,
	}, nil
}

// RevokeJWTToken 撤销JWT Token
func (b *BuiltInCloudControl) RevokeJWTToken(ctx context.Context, token string) error {
	// 验证Token并获取客户端ID
	claims, err := b.jwtManager.ValidateAccessToken(ctx, token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// 获取客户端
	client, err := b.clientRepo.GetClient(ctx, claims.ClientID)
	if err != nil {
		return fmt.Errorf("client not found: %w", err)
	}

	// 验证Token ID是否匹配
	if client.JWTToken != claims.ID {
		return fmt.Errorf("token has already been revoked")
	}

	// 撤销缓存中的Token
	if err := b.jwtManager.cache.RevokeAccessToken(ctx, token); err != nil {
		return fmt.Errorf("revoke access token from cache failed: %w", err)
	}

	// 撤销缓存中的刷新Token
	if client.RefreshToken != "" {
		if err := b.jwtManager.cache.RevokeRefreshToken(ctx, client.RefreshToken); err != nil {
			return fmt.Errorf("revoke refresh token from cache failed: %w", err)
		}
	}

	// 清除客户端Token信息
	client.JWTToken = ""
	client.TokenExpiresAt = nil
	client.RefreshToken = ""
	client.UpdatedAt = time.Now()

	return b.clientRepo.SaveClient(ctx, client)
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
