package cloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tunnox-core/internal/utils"
)

// BuiltInCloudControl 内置云控实现
type BuiltInCloudControl struct {
	config         *CloudControlConfig
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

	// 资源管理
	utils.Dispose
}

// NewBuiltInCloudControl 创建新的内置云控
func NewBuiltInCloudControl(config *CloudControlConfig) *BuiltInCloudControl {
	ctx := context.Background()
	memoryStorage := NewMemoryStorage(ctx)
	storage := NewRepository(memoryStorage)
	lock := NewMemoryLock()

	cloudControl := &BuiltInCloudControl{
		config:         config,
		idGen:          NewDistributedIDGenerator(storage.GetStorage(), lock),
		userRepo:       NewUserRepository(storage),
		clientRepo:     NewClientRepository(storage),
		mappingRepo:    NewPortMappingRepository(storage),
		nodeRepo:       NewNodeRepository(storage),
		connRepo:       NewConnectionRepository(storage),
		jwtManager:     NewJWTManager(config, storage),
		configManager:  NewConfigManager(storage.GetStorage(), config, ctx),
		cleanupManager: NewCleanupManager(storage.GetStorage(), lock, ctx),
		lock:           lock,
		cleanupTicker:  time.NewTicker(DefaultCleanupInterval),
		done:           make(chan bool),
	}

	// 设置上下文和资源清理
	cloudControl.SetCtx(ctx, cloudControl.onClose)

	return cloudControl
}

// Start 启动内置云控
func (b *BuiltInCloudControl) Start() {
	if b.IsClosed() {
		utils.Warnf("Cloud control is already closed, cannot start")
		return
	}

	go b.cleanupRoutine()
	utils.Infof("Built-in cloud control started successfully")
}

// Stop 停止内置云控
func (b *BuiltInCloudControl) Stop() {
	if b.IsClosed() {
		return
	}

	utils.Infof("Stopping built-in cloud control...")

	// 通知清理例程退出 - 直接关闭通道更可靠
	select {
	case <-b.done:
		// 通道已关闭
		utils.Infof("Cleanup done channel already closed")
	default:
		utils.Info("Closing cleanup done channel...")
		close(b.done)
		utils.Infof("Cleanup done channel closed")
	}

	utils.Infof("Built-in cloud control stopped")
}

// Close 关闭内置云控（实现CloudControlAPI接口）
func (b *BuiltInCloudControl) Close() error {
	b.Ctx().Done()
	return nil
}

// onClose 资源清理回调
func (b *BuiltInCloudControl) onClose() {
	utils.Infof("Cleaning up cloud control resources...")

	// 先停止服务
	b.Stop()

	// 等待清理例程完全退出
	time.Sleep(100 * time.Millisecond)

	// 关闭done通道
	select {
	case <-b.done:
		// 通道已关闭
	default:
		close(b.done)
	}

	// 清理各个组件
	if b.jwtManager != nil {
		// JWT管理器可能有自己的清理逻辑
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

// NodeRegister 节点注册
func (b *BuiltInCloudControl) NodeRegister(ctx context.Context, req *NodeRegisterRequest) (*NodeRegisterResponse, error) {
	nodeID := req.NodeID
	if nodeID == "" {
		// 生成节点ID，确保不重复
		for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
			generatedID, err := b.idGen.GenerateNodeID(ctx)
			if err != nil {
				utils.LogErrorWithContext(err, "generate node ID", map[string]interface{}{
					"attempts":    attempts,
					"maxAttempts": DefaultMaxAttempts,
				})
				return nil, NewStorageError("generate node ID")
			}

			// 检查节点是否已存在
			existingNode, err := b.nodeRepo.GetNode(ctx, generatedID)
			if err != nil {
				// 节点不存在，可以使用这个ID
				nodeID = generatedID
				break
			}

			if existingNode != nil {
				// 节点已存在，释放ID并重试
				_ = b.idGen.ReleaseNodeID(ctx, generatedID)
				utils.Warnf("Node ID %s already exists, retrying...", generatedID)
				continue
			}

			nodeID = generatedID
			break
		}

		if nodeID == "" {
			utils.Errorf("Failed to generate unique node ID after %d attempts", DefaultMaxAttempts)
			return nil, ErrIDExhausted
		}
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
	if err := b.nodeRepo.CreateNode(ctx, node); err != nil {
		// 如果保存失败，释放ID
		if req.NodeID == "" {
			_ = b.idGen.ReleaseNodeID(ctx, nodeID)
		}
		utils.LogOperation(OperationCreate, "node", nodeID, false, err)
		return nil, NewStorageError("save node")
	}

	// 添加到节点列表
	if err := b.nodeRepo.AddNodeToList(ctx, node); err != nil {
		// 如果添加到列表失败，删除节点并释放ID
		_ = b.nodeRepo.DeleteNode(ctx, nodeID)
		if req.NodeID == "" {
			_ = b.idGen.ReleaseNodeID(ctx, nodeID)
		}
		utils.LogOperation(OperationCreate, "node list", nodeID, false, err)
		return nil, NewStorageError("add node to list")
	}

	utils.LogOperation(OperationCreate, "node", nodeID, true, nil)
	utils.LogSystemEvent("node_registered", "cloud_control", map[string]interface{}{
		"nodeID":  nodeID,
		"address": req.Address,
	})

	// 记录详细的节点注册信息
	utils.Infof("云控节点注册成功 - 节点ID: %s, 节点名称: %s, 服务地址: %s, 版本: %s",
		nodeID, node.Name, req.Address, req.Version)

	// 记录元数据信息（如果有）
	if len(req.Meta) > 0 {
		utils.Infof("节点元数据: %+v", req.Meta)
	}

	return &NodeRegisterResponse{
		NodeID:  nodeID,
		Success: true,
		Message: SuccessMsgNodeRegistered,
	}, nil
}

// NodeUnregister 节点反注册
func (b *BuiltInCloudControl) NodeUnregister(ctx context.Context, req *NodeUnregisterRequest) error {
	// 获取节点信息，用于释放ID
	if node, err := b.nodeRepo.GetNode(ctx, req.NodeID); err == nil && node != nil {
		// 释放节点ID
		_ = b.idGen.ReleaseNodeID(ctx, req.NodeID)
	}

	return b.nodeRepo.DeleteNode(ctx, req.NodeID)
}

// NodeHeartbeat 节点心跳
func (b *BuiltInCloudControl) NodeHeartbeat(ctx context.Context, req *NodeHeartbeatRequest) (*NodeHeartbeatResponse, error) {
	node, err := b.nodeRepo.GetNode(ctx, req.NodeID)
	if err != nil {
		utils.LogHeartbeat(req.NodeID, false, err)
		return &NodeHeartbeatResponse{
			Success: false,
			Message: ErrMsgNodeNotFound,
		}, nil
	}

	// 更新节点信息
	node.Address = req.Address
	node.UpdatedAt = time.Now()

	if err := b.nodeRepo.UpdateNode(ctx, node); err != nil {
		utils.LogHeartbeat(req.NodeID, false, err)
		return &NodeHeartbeatResponse{
			Success: false,
			Message: ErrMsgStorageError,
		}, nil
	}

	utils.LogHeartbeat(req.NodeID, true, nil)
	return &NodeHeartbeatResponse{
		Success: true,
		Message: SuccessMsgHeartbeatReceived,
	}, nil
}

// Authenticate 用户认证
func (b *BuiltInCloudControl) Authenticate(ctx context.Context, req *AuthRequest) (*AuthResponse, error) {
	// 获取客户端
	client, err := b.clientRepo.GetClient(ctx, req.ClientID)
	if err != nil {
		utils.LogAuthentication("", req.ClientID, false, err)
		return &AuthResponse{
			Success: false,
			Message: ErrMsgClientNotFound,
		}, nil
	}

	// 验证认证码
	if client.AuthCode != req.AuthCode {
		utils.LogAuthentication(client.UserID, req.ClientID, false, ErrInvalidAuthCode)
		return &AuthResponse{
			Success: false,
			Message: ErrMsgInvalidAuthCode,
		}, nil
	}

	// 验证密钥（如果提供）
	if req.SecretKey != "" && client.SecretKey != req.SecretKey {
		utils.LogAuthentication(client.UserID, req.ClientID, false, ErrInvalidSecretKey)
		return &AuthResponse{
			Success: false,
			Message: ErrMsgInvalidSecretKey,
		}, nil
	}

	// 检查客户端状态
	if client.Status == ClientStatusBlocked {
		utils.LogAuthentication(client.UserID, req.ClientID, false, ErrClientBlocked)
		return &AuthResponse{
			Success: false,
			Message: ErrMsgClientBlocked,
		}, nil
	}

	// 更新客户端状态
	now := time.Now()
	client.Status = ClientStatusOnline
	client.NodeID = req.NodeID
	client.LastSeen = &now
	client.UpdatedAt = now

	if err := b.clientRepo.UpdateClient(ctx, client); err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Failed to update client status",
		}, nil
	}

	// 生成JWT令牌
	tokenInfo, err := b.jwtManager.GenerateTokenPair(ctx, client)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Failed to generate token",
		}, nil
	}

	return &AuthResponse{
		Success:   true,
		Message:   "Authentication successful",
		Token:     tokenInfo.Token,
		Client:    client,
		ExpiresAt: tokenInfo.ExpiresAt,
	}, nil
}

// ValidateToken 验证令牌
func (b *BuiltInCloudControl) ValidateToken(ctx context.Context, token string) (*AuthResponse, error) {
	// 验证JWT令牌
	claims, err := b.jwtManager.ValidateAccessToken(ctx, token)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	// 获取客户端信息
	client, err := b.clientRepo.GetClient(ctx, claims.ClientID)
	if err != nil {
		return &AuthResponse{
			Success: false,
			Message: "Client not found",
		}, nil
	}

	return &AuthResponse{
		Success: true,
		Message: "Token is valid",
		Client:  client,
	}, nil
}

// CreateUser 创建用户
func (b *BuiltInCloudControl) CreateUser(ctx context.Context, username, email string) (*User, error) {
	// 生成用户ID，确保不重复
	var userID string
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateUserID(ctx)
		if err != nil {
			return nil, fmt.Errorf("generate user ID failed: %w", err)
		}

		// 检查用户是否已存在
		existingUser, err := b.userRepo.GetUser(ctx, generatedID)
		if err != nil {
			// 用户不存在，可以使用这个ID
			userID = generatedID
			break
		}

		if existingUser != nil {
			// 用户已存在，释放ID并重试
			_ = b.idGen.ReleaseUserID(ctx, generatedID)
			continue
		}

		userID = generatedID
		break
	}

	if userID == "" {
		return nil, fmt.Errorf("failed to generate unique user ID after %d attempts", DefaultMaxAttempts)
	}

	// 创建用户
	now := time.Now()
	user := &User{
		ID:        userID,
		Username:  username,
		Email:     email,
		Type:      UserTypeRegistered,
		Status:    UserStatusActive,
		Plan:      UserPlanFree,
		CreatedAt: now,
		UpdatedAt: now,
		Quota: UserQuota{
			MaxClientIds:   DefaultUserMaxConnections,
			MaxConnections: DefaultUserMaxConnections,
			BandwidthLimit: DefaultUserBandwidthLimit,
			StorageLimit:   DefaultUserStorageLimit,
		},
	}

	if err := b.userRepo.CreateUser(ctx, user); err != nil {
		// 如果保存失败，释放ID
		_ = b.idGen.ReleaseUserID(ctx, userID)
		return nil, fmt.Errorf("save user failed: %w", err)
	}

	if err := b.userRepo.AddUserToList(ctx, user); err != nil {
		// 如果添加到列表失败，删除用户并释放ID
		_ = b.userRepo.DeleteUser(ctx, userID)
		_ = b.idGen.ReleaseUserID(ctx, userID)
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
	return b.userRepo.UpdateUser(ctx, user)
}

// DeleteUser 删除用户
func (b *BuiltInCloudControl) DeleteUser(ctx context.Context, userID string) error {
	// 获取用户信息，用于释放ID
	user, err := b.userRepo.GetUser(ctx, userID)
	if err == nil && user != nil {
		// 释放用户ID
		_ = b.idGen.ReleaseUserID(ctx, userID)
	}

	return b.userRepo.DeleteUser(ctx, userID)
}

// ListUsers 列出用户
func (b *BuiltInCloudControl) ListUsers(ctx context.Context, userType UserType) ([]*User, error) {
	return b.userRepo.ListUsers(ctx, userType)
}

// CreateClient 创建客户端
func (b *BuiltInCloudControl) CreateClient(ctx context.Context, userID, clientName string) (*Client, error) {
	// 生成客户端ID，确保不重复
	var clientID int64
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateClientID(ctx)
		if err != nil {
			return nil, fmt.Errorf("generate client ID failed: %w", err)
		}

		// 检查客户端是否已存在
		existingClient, err := b.clientRepo.GetClient(ctx, fmt.Sprintf("%d", generatedID))
		if err != nil {
			// 客户端不存在，可以使用这个ID
			clientID = generatedID
			break
		}

		if existingClient != nil {
			// 客户端已存在，释放ID并重试
			_ = b.idGen.ReleaseClientID(ctx, generatedID)
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
		_ = b.idGen.ReleaseClientID(ctx, clientID)
		return nil, fmt.Errorf("generate auth code failed: %w", err)
	}

	secretKey, err := b.idGen.GenerateSecretKey()
	if err != nil {
		// 如果生成密钥失败，释放客户端ID
		_ = b.idGen.ReleaseClientID(ctx, clientID)
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

	if err := b.clientRepo.CreateClient(ctx, client); err != nil {
		// 如果保存失败，释放客户端ID
		_ = b.idGen.ReleaseClientID(ctx, clientID)
		return nil, fmt.Errorf("save client failed: %w", err)
	}

	// 强制添加到用户列表（即使 userID 为空也加到匿名列表）
	if err := b.clientRepo.AddClientToUser(ctx, userID, client); err != nil {
		// 如果添加到用户失败，删除客户端并释放ID
		_ = b.clientRepo.DeleteClient(ctx, client.ID)
		_ = b.idGen.ReleaseClientID(ctx, clientID)
		return nil, fmt.Errorf("add client to user failed: %w", err)
	}

	return client, nil
}

func (b *BuiltInCloudControl) TouchClient(ctx context.Context, clientID string) {
	client, err := b.clientRepo.GetClient(ctx, clientID)
	if (err == nil) && (client != nil) {
		client.UpdatedAt = time.Now()
		_ = b.clientRepo.UpdateClient(ctx, client)
		_ = b.clientRepo.TouchClient(ctx, clientID)
	}
}

// GetClient 获取客户端
func (b *BuiltInCloudControl) GetClient(ctx context.Context, clientID string) (*Client, error) {
	return b.clientRepo.GetClient(ctx, clientID)
}

// UpdateClient 更新客户端
func (b *BuiltInCloudControl) UpdateClient(ctx context.Context, client *Client) error {
	client.UpdatedAt = time.Now()
	return b.clientRepo.UpdateClient(ctx, client)
}

// DeleteClient 删除客户端
func (b *BuiltInCloudControl) DeleteClient(ctx context.Context, clientID string) error {
	// 获取客户端信息，用于释放ID
	client, err := b.clientRepo.GetClient(ctx, clientID)
	if err == nil && client != nil {
		// 解析客户端ID为int64并释放
		var clientIDInt int64
		if _, err := fmt.Sscanf(clientID, "%d", &clientIDInt); err == nil {
			_ = b.idGen.ReleaseClientID(ctx, clientIDInt)
		}
	}

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
	// 生成端口映射ID，确保不重复
	var mappingID string
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateMappingID(ctx)
		if err != nil {
			return nil, fmt.Errorf("generate mapping ID failed: %w", err)
		}

		// 检查端口映射是否已存在
		existingMapping, err := b.mappingRepo.GetPortMapping(ctx, generatedID)
		if err != nil {
			// 端口映射不存在，可以使用这个ID
			mappingID = generatedID
			break
		}

		if existingMapping != nil {
			// 端口映射已存在，释放ID并重试
			_ = b.idGen.ReleaseMappingID(ctx, generatedID)
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

	if err := b.mappingRepo.CreatePortMapping(ctx, mapping); err != nil {
		// 如果保存失败，释放ID
		_ = b.idGen.ReleaseMappingID(ctx, mappingID)
		return nil, fmt.Errorf("save port mapping failed: %w", err)
	}

	if mapping.UserID != "" {
		if err := b.mappingRepo.AddMappingToUser(ctx, mapping.UserID, mapping); err != nil {
			// 如果添加到用户失败，删除端口映射并释放ID
			_ = b.mappingRepo.DeletePortMapping(ctx, mappingID)
			_ = b.idGen.ReleaseMappingID(ctx, mappingID)
			return nil, fmt.Errorf("add mapping to user failed: %w", err)
		}
	}

	return mapping, nil
}

// GetPortMappings 获取用户的端口映射
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
	return b.mappingRepo.UpdatePortMapping(ctx, mapping)
}

// DeletePortMapping 删除端口映射
func (b *BuiltInCloudControl) DeletePortMapping(ctx context.Context, mappingID string) error {
	// 获取端口映射信息，用于释放ID
	mapping, err := b.mappingRepo.GetPortMapping(ctx, mappingID)
	if err == nil && mapping != nil {
		// 释放端口映射ID
		_ = b.idGen.ReleaseMappingID(ctx, mappingID)
	}

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
	// 简化实现：返回所有映射
	return b.mappingRepo.ListUserMappings(ctx, "")
}

// GenerateAnonymousCredentials 生成匿名凭据
func (b *BuiltInCloudControl) GenerateAnonymousCredentials(ctx context.Context) (*Client, error) {
	// 生成客户端ID，确保不重复
	var clientID int64
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateClientID(ctx)
		if err != nil {
			return nil, fmt.Errorf("generate client ID failed: %w", err)
		}

		// 检查客户端是否已存在
		existingClient, err := b.clientRepo.GetClient(ctx, fmt.Sprintf("%d", generatedID))
		if err != nil {
			// 客户端不存在，可以使用这个ID
			clientID = generatedID
			break
		}

		if existingClient != nil {
			// 客户端已存在，释放ID并重试
			_ = b.idGen.ReleaseClientID(ctx, generatedID)
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
		_ = b.idGen.ReleaseClientID(ctx, clientID)
		return nil, fmt.Errorf("generate auth code failed: %w", err)
	}

	secretKey, err := b.idGen.GenerateSecretKey()
	if err != nil {
		// 如果生成密钥失败，释放客户端ID
		_ = b.idGen.ReleaseClientID(ctx, clientID)
		return nil, fmt.Errorf("generate secret key failed: %w", err)
	}

	now := time.Now()
	client := &Client{
		ID:        fmt.Sprintf("%d", clientID),
		UserID:    "", // 匿名用户
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

	if err := b.clientRepo.CreateClient(ctx, client); err != nil {
		// 如果保存失败，释放客户端ID
		_ = b.idGen.ReleaseClientID(ctx, clientID)
		return nil, fmt.Errorf("save anonymous client failed: %w", err)
	}

	// 添加到匿名列表
	if err := b.clientRepo.AddClientToUser(ctx, "", client); err != nil {
		// 如果添加到匿名列表失败，删除客户端并释放ID
		_ = b.clientRepo.DeleteClient(ctx, client.ID)
		_ = b.idGen.ReleaseClientID(ctx, clientID)
		return nil, fmt.Errorf("add anonymous client to list failed: %w", err)
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
	return b.DeleteClient(ctx, clientID)
}

// ListAnonymousClients 列出匿名客户端
func (b *BuiltInCloudControl) ListAnonymousClients(ctx context.Context) ([]*Client, error) {
	return b.clientRepo.ListUserClients(ctx, "")
}

// CreateAnonymousMapping 创建匿名端口映射
func (b *BuiltInCloudControl) CreateAnonymousMapping(ctx context.Context, sourceClientID, targetClientID string, protocol Protocol, sourcePort, targetPort int) (*PortMapping, error) {
	// 生成端口映射ID，确保不重复
	var mappingID string
	for attempts := 0; attempts < DefaultMaxAttempts; attempts++ {
		generatedID, err := b.idGen.GenerateMappingID(ctx)
		if err != nil {
			return nil, fmt.Errorf("generate mapping ID failed: %w", err)
		}

		// 检查端口映射是否已存在
		existingMapping, err := b.mappingRepo.GetPortMapping(ctx, generatedID)
		if err != nil {
			// 端口映射不存在，可以使用这个ID
			mappingID = generatedID
			break
		}

		if existingMapping != nil {
			// 端口映射已存在，释放ID并重试
			_ = b.idGen.ReleaseMappingID(ctx, generatedID)
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
		UserID:         "", // 匿名映射
		SourceClientID: sourceClientID,
		TargetClientID: targetClientID,
		Protocol:       protocol,
		SourcePort:     sourcePort,
		TargetPort:     targetPort,
		Status:         MappingStatusActive,
		Type:           MappingTypeAnonymous,
		Config: MappingConfig{
			EnableCompression: DefaultEnableCompression,
			BandwidthLimit:    DefaultAnonymousBandwidthLimit,
			Timeout:           30,
			RetryCount:        3,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := b.mappingRepo.CreatePortMapping(ctx, mapping); err != nil {
		// 如果保存失败，释放ID
		_ = b.idGen.ReleaseMappingID(ctx, mappingID)
		return nil, fmt.Errorf("save anonymous mapping failed: %w", err)
	}

	// 添加到匿名映射列表
	if err := b.mappingRepo.AddMappingToUser(ctx, "", mapping); err != nil {
		// 如果添加到匿名列表失败，删除映射并释放ID
		_ = b.mappingRepo.DeletePortMapping(ctx, mappingID)
		_ = b.idGen.ReleaseMappingID(ctx, mappingID)
		return nil, fmt.Errorf("add anonymous mapping to list failed: %w", err)
	}

	return mapping, nil
}

// GetAnonymousMappings 获取匿名端口映射
func (b *BuiltInCloudControl) GetAnonymousMappings(ctx context.Context) ([]*PortMapping, error) {
	return b.mappingRepo.ListUserMappings(ctx, "")
}

// CleanupExpiredAnonymous 清理过期的匿名资源
func (b *BuiltInCloudControl) CleanupExpiredAnonymous(ctx context.Context) error {
	// 这里可以实现清理逻辑
	return nil
}

// GetNodeServiceInfo 获取节点服务信息
func (b *BuiltInCloudControl) GetNodeServiceInfo(ctx context.Context, nodeID string) (*NodeServiceInfo, error) {
	node, err := b.nodeRepo.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	// 获取节点的客户端数量
	clients, err := b.clientRepo.ListUserClients(ctx, "")
	if err != nil {
		return nil, err
	}

	var nodeClients []*Client
	for _, client := range clients {
		if client.NodeID == nodeID {
			nodeClients = append(nodeClients, client)
		}
	}

	return &NodeServiceInfo{
		NodeID:  nodeID,
		Address: node.Address,
	}, nil
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (b *BuiltInCloudControl) GetAllNodeServiceInfo(ctx context.Context) ([]*NodeServiceInfo, error) {
	nodes, err := b.nodeRepo.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	var nodeInfos []*NodeServiceInfo
	for _, node := range nodes {
		info, err := b.GetNodeServiceInfo(ctx, node.ID)
		if err != nil {
			continue
		}
		nodeInfos = append(nodeInfos, info)
	}

	return nodeInfos, nil
}

// GetUserStats 获取用户统计
func (b *BuiltInCloudControl) GetUserStats(ctx context.Context, userID string) (*UserStats, error) {
	user, err := b.userRepo.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 获取用户的客户端
	clients, err := b.clientRepo.ListUserClients(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 获取用户的端口映射
	mappings, err := b.mappingRepo.ListUserMappings(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	var totalTraffic int64
	var activeConnections int
	for _, mapping := range mappings {
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		// 这里可以添加连接数统计
	}

	return &UserStats{
		UserID:           userID,
		TotalClients:     len(clients),
		TotalMappings:    len(mappings),
		TotalTraffic:     totalTraffic,
		TotalConnections: int64(activeConnections),
		LastActive:       user.UpdatedAt,
	}, nil
}

// GetClientStats 获取客户端统计
func (b *BuiltInCloudControl) GetClientStats(ctx context.Context, clientID string) (*ClientStats, error) {
	client, err := b.clientRepo.GetClient(ctx, clientID)
	if err != nil {
		return nil, err
	}

	// 获取客户端的端口映射
	mappings, err := b.mappingRepo.ListClientMappings(ctx, clientID)
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	var totalTraffic int64
	var activeConnections int
	for _, mapping := range mappings {
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		// 这里可以添加连接数统计
	}

	return &ClientStats{
		ClientID:         clientID,
		UserID:           client.UserID,
		TotalMappings:    len(mappings),
		TotalTraffic:     totalTraffic,
		TotalConnections: int64(activeConnections),
		LastSeen:         client.UpdatedAt,
	}, nil
}

// GetSystemStats 获取系统统计
func (b *BuiltInCloudControl) GetSystemStats(ctx context.Context) (*SystemStats, error) {
	// 获取所有用户
	users, err := b.userRepo.ListUsers(ctx, "")
	if err != nil {
		return nil, err
	}

	// 获取所有客户端
	clients, err := b.clientRepo.ListUserClients(ctx, "")
	if err != nil {
		return nil, err
	}

	// 获取所有端口映射
	mappings, err := b.mappingRepo.ListUserMappings(ctx, "")
	if err != nil {
		return nil, err
	}

	// 获取所有节点
	nodes, err := b.nodeRepo.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	var totalTraffic int64
	var activeConnections int
	var onlineClients int
	for _, client := range clients {
		if client.Status == ClientStatusOnline {
			onlineClients++
		}
	}

	for _, mapping := range mappings {
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		// 这里可以添加连接数统计
	}

	return &SystemStats{
		TotalUsers:       len(users),
		TotalClients:     len(clients),
		OnlineClients:    onlineClients,
		TotalMappings:    len(mappings),
		TotalNodes:       len(nodes),
		TotalTraffic:     totalTraffic,
		TotalConnections: int64(activeConnections),
		AnonymousUsers:   0, // 简化实现
	}, nil
}

// GetTrafficStats 获取流量统计
func (b *BuiltInCloudControl) GetTrafficStats(ctx context.Context, timeRange string) ([]*TrafficDataPoint, error) {
	// 简化实现：返回空数组
	return []*TrafficDataPoint{}, nil
}

// GetConnectionStats 获取连接统计
func (b *BuiltInCloudControl) GetConnectionStats(ctx context.Context, timeRange string) ([]*ConnectionDataPoint, error) {
	// 简化实现：返回空数组
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
		if strings.Contains(strings.ToLower(client.Name), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(client.ID), strings.ToLower(keyword)) {
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
	// 验证端口映射是否存在
	mapping, err := b.mappingRepo.GetPortMapping(ctx, mappingId)
	if err != nil {
		return fmt.Errorf("mapping not found: %w", err)
	}

	if mapping == nil {
		return fmt.Errorf("mapping not found")
	}

	// 设置连接信息
	connInfo.MappingId = mappingId
	connInfo.EstablishedAt = time.Now()
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()

	// 保存连接信息
	if err := b.connRepo.CreateConnection(ctx, connInfo); err != nil {
		return fmt.Errorf("create connection failed: %w", err)
	}

	// 添加到映射连接列表
	if err := b.connRepo.AddConnectionToMapping(ctx, mappingId, connInfo); err != nil {
		// 如果添加到列表失败，删除连接
		_ = b.connRepo.DeleteConnection(ctx, connInfo.ConnId)
		return fmt.Errorf("add connection to mapping failed: %w", err)
	}

	return nil
}

// UnregisterConnection 注销连接
func (b *BuiltInCloudControl) UnregisterConnection(ctx context.Context, connId string) error {
	// 先获取连接信息以获取映射ID
	connInfo, err := b.connRepo.GetConnection(ctx, connId)
	if err != nil {
		return fmt.Errorf("connection not found: %w", err)
	}

	// 从映射连接列表中删除
	if connInfo.MappingId != "" {
		// 这里需要实现从列表中删除的逻辑
		// 由于当前存储层没有提供从列表中删除特定项的方法，我们暂时跳过
		// 在实际实现中，应该从映射连接列表中删除这个连接
	}

	// 删除连接
	return b.connRepo.DeleteConnection(ctx, connId)
}

// GetConnections 获取端口映射的连接
func (b *BuiltInCloudControl) GetConnections(ctx context.Context, mappingId string) ([]*ConnectionInfo, error) {
	return b.connRepo.ListMappingConnections(ctx, mappingId)
}

// GetClientConnections 获取客户端的连接
func (b *BuiltInCloudControl) GetClientConnections(ctx context.Context, clientId string) ([]*ConnectionInfo, error) {
	return b.connRepo.ListClientConnections(ctx, clientId)
}

// UpdateConnectionStats 更新连接统计
func (b *BuiltInCloudControl) UpdateConnectionStats(ctx context.Context, connId string, bytesSent, bytesReceived int64) error {
	return b.connRepo.UpdateConnectionStats(ctx, connId, bytesSent, bytesReceived)
}

// GenerateJWTToken 生成JWT令牌
func (b *BuiltInCloudControl) GenerateJWTToken(ctx context.Context, clientId string) (*JWTTokenInfo, error) {
	client, err := b.clientRepo.GetClient(ctx, clientId)
	if err != nil {
		return nil, err
	}
	return b.jwtManager.GenerateTokenPair(ctx, client)
}

// RefreshJWTToken 刷新JWT令牌
func (b *BuiltInCloudControl) RefreshJWTToken(ctx context.Context, refreshToken string) (*JWTTokenInfo, error) {
	// 验证刷新令牌
	claims, err := b.jwtManager.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// 获取客户端信息
	client, err := b.clientRepo.GetClient(ctx, claims.ClientID)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	// 生成新的令牌对
	return b.jwtManager.GenerateTokenPair(ctx, client)
}

// ValidateJWTToken 验证JWT令牌
func (b *BuiltInCloudControl) ValidateJWTToken(ctx context.Context, token string) (*JWTTokenInfo, error) {
	claims, err := b.jwtManager.ValidateAccessToken(ctx, token)
	if err != nil {
		return nil, err
	}

	client, err := b.clientRepo.GetClient(ctx, claims.ClientID)
	if err != nil {
		return nil, err
	}

	return &JWTTokenInfo{
		Token:    token,
		ClientId: client.ID,
		TokenID:  claims.ID,
	}, nil
}

// RevokeJWTToken 撤销JWT令牌
func (b *BuiltInCloudControl) RevokeJWTToken(ctx context.Context, token string) error {
	// 验证令牌以获取客户端ID
	claims, err := b.jwtManager.ValidateAccessToken(ctx, token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// 将令牌加入黑名单
	return b.jwtManager.RevokeToken(ctx, claims.ID)
}

// cleanupRoutine 清理例程
func (b *BuiltInCloudControl) cleanupRoutine() {
	utils.LogSystemEvent("cleanup_routine_started", "cloud_control", nil)

	// 注册清理任务
	ctx := context.Background()
	tasks := []struct {
		taskType string
		interval time.Duration
	}{
		{"expired_tokens", 5 * time.Minute},
		{"orphaned_connections", 2 * time.Minute},
		{"stale_mappings", 10 * time.Minute},
	}

	for _, task := range tasks {
		if err := b.cleanupManager.RegisterCleanupTask(ctx, task.taskType, task.interval); err != nil {
			utils.Errorf("Failed to register cleanup task %s: %v", task.taskType, err)
		} else {
			utils.Infof("Registered cleanup task: %s (interval: %v)", task.taskType, task.interval)
		}
	}

	for {
		// 优先检查退出条件
		select {
		case <-b.done:
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "manual_stop",
			})
			utils.Info("Cloud control cleanup routine exited (manual stop)")
			return

		case <-b.Ctx().Done():
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "context_cancelled",
			})
			utils.Info("Cloud control cleanup routine exited (context cancelled)")
			return

		default:
			// 如果没有退出信号，检查ticker
		}

		// 检查是否已关闭
		if b.IsClosed() {
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "disposed",
			})
			utils.Info("Cloud control cleanup routine exited (disposed)")
			return
		}

		// 等待ticker或退出信号
		select {
		case <-b.cleanupTicker.C:
			// 执行清理逻辑
			ctx := context.Background()
			startTime := time.Now()

			// 使用分布式清理管理器
			if _, acquired, err := b.cleanupManager.AcquireCleanupTask(ctx, "expired_tokens"); err == nil && acquired {
				// 清理过期的JWT令牌（简化实现）
				cleanupErr := b.cleanupManager.CompleteCleanupTask(ctx, "expired_tokens", nil)
				utils.LogCleanup("expired_tokens", 0, time.Since(startTime), cleanupErr)
			} else if err != nil {
				utils.LogErrorWithContext(err, "acquire cleanup task", map[string]interface{}{
					"task": "expired_tokens",
				})
			}

			if _, acquired, err := b.cleanupManager.AcquireCleanupTask(ctx, "orphaned_connections"); err == nil && acquired {
				// 清理孤立的连接（简化实现）
				cleanupErr := b.cleanupManager.CompleteCleanupTask(ctx, "orphaned_connections", nil)
				utils.LogCleanup("orphaned_connections", 0, time.Since(startTime), cleanupErr)
			} else if err != nil {
				utils.LogErrorWithContext(err, "acquire cleanup task", map[string]interface{}{
					"task": "orphaned_connections",
				})
			}

			if _, acquired, err := b.cleanupManager.AcquireCleanupTask(ctx, "stale_mappings"); err == nil && acquired {
				// 清理过期的匿名映射
				cleanupErr := b.CleanupExpiredAnonymous(ctx)
				if cleanupErr != nil {
					_ = b.cleanupManager.CompleteCleanupTask(ctx, "stale_mappings", cleanupErr)
				} else {
					_ = b.cleanupManager.CompleteCleanupTask(ctx, "stale_mappings", nil)
				}
				utils.LogCleanup("stale_mappings", 0, time.Since(startTime), cleanupErr)
			} else if err != nil {
				utils.LogErrorWithContext(err, "acquire cleanup task", map[string]interface{}{
					"task": "stale_mappings",
				})
			}

		case <-b.done:
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "manual_stop",
			})
			utils.Info("Cloud control cleanup routine exited (manual stop)")
			return

		case <-b.Ctx().Done():
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "context_cancelled",
			})
			utils.Info("Cloud control cleanup routine exited (context cancelled)")
			return
		}
	}
}
