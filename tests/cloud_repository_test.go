package tests

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud"
	"tunnox-core/internal/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRepository_CreateUser(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)
	ctx := context.Background()

	userID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	user := &cloud.User{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Type:      cloud.UserTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(ctx, user)
	require.NoError(t, err)
	require.NotNil(t, user)

	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, cloud.UserTypeRegistered, user.Type)
	assert.NotEmpty(t, user.ID)
	assert.NotZero(t, user.CreatedAt)
	assert.NotZero(t, user.UpdatedAt)

	// 测试重复ID
	user2 := &cloud.User{
		ID:        userID,
		Username:  "testuser2",
		Email:     "another@example.com",
		Type:      cloud.UserTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(ctx, user2)
	assert.Error(t, err)

	// 测试重复ID（应该成功，因为当前实现只检查ID重复）
	userID3, err := utils.GenerateRandomString(16)
	require.NoError(t, err)
	user3 := &cloud.User{
		ID:        userID3,
		Username:  "testuser",         // 相同用户名
		Email:     "test@example.com", // 相同邮箱
		Type:      cloud.UserTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(ctx, user3)
	require.NoError(t, err) // 应该成功，因为ID不同
}

func TestUserRepository_GetUser(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)
	ctx := context.Background()

	userID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	user := &cloud.User{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Type:      cloud.UserTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(ctx, user)
	require.NoError(t, err)

	retrievedUser, err := userRepo.GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedUser)

	assert.Equal(t, user.ID, retrievedUser.ID)
	assert.Equal(t, user.Username, retrievedUser.Username)
	assert.Equal(t, user.Email, retrievedUser.Email)

	// 不支持GetUserByUsername/GetUserByEmail
	// _, err = userRepo.GetUserByUsername(ctx, user.Username)
	// assert.Error(t, err)
	// _, err = userRepo.GetUserByEmail(ctx, user.Email)
	// assert.Error(t, err)

	_, err = userRepo.GetUser(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestUserRepository_UpdateUser(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)
	ctx := context.Background()

	userID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	user := &cloud.User{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Type:      cloud.UserTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(ctx, user)
	require.NoError(t, err)

	user.Username = "updateduser"
	user.Email = "updated@example.com"
	user.Status = cloud.UserStatusSuspended
	err = userRepo.UpdateUser(ctx, user)
	require.NoError(t, err)

	retrievedUser, err := userRepo.GetUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "updateduser", retrievedUser.Username)
	assert.Equal(t, "updated@example.com", retrievedUser.Email)
	assert.Equal(t, cloud.UserStatusSuspended, retrievedUser.Status)
}

func TestUserRepository_DeleteUser(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)
	ctx := context.Background()

	userID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	user := &cloud.User{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Type:      cloud.UserTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(ctx, user)
	require.NoError(t, err)

	err = userRepo.DeleteUser(ctx, user.ID)
	require.NoError(t, err)

	_, err = userRepo.GetUser(ctx, user.ID)
	assert.Error(t, err)
}

func TestUserRepository_ListUsers(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)
	ctx := context.Background()

	userID1, err := utils.GenerateRandomString(16)
	require.NoError(t, err)
	userID2, err := utils.GenerateRandomString(16)
	require.NoError(t, err)
	userID3, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	user1 := &cloud.User{
		ID:        userID1,
		Username:  "user1",
		Email:     "user1@example.com",
		Type:      cloud.UserTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	user2 := &cloud.User{
		ID:        userID2,
		Username:  "user2",
		Email:     "user2@example.com",
		Type:      cloud.UserTypeAnonymous,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	user3 := &cloud.User{
		ID:        userID3,
		Username:  "user3",
		Email:     "user3@example.com",
		Type:      cloud.UserTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(ctx, user1)
	require.NoError(t, err)
	err = userRepo.AddUserToList(ctx, user1)
	require.NoError(t, err)
	err = userRepo.CreateUser(ctx, user2)
	require.NoError(t, err)
	err = userRepo.AddUserToList(ctx, user2)
	require.NoError(t, err)
	err = userRepo.CreateUser(ctx, user3)
	require.NoError(t, err)
	err = userRepo.AddUserToList(ctx, user3)
	require.NoError(t, err)

	users, err := userRepo.ListUsers(ctx, "")
	require.NoError(t, err)
	assert.Len(t, users, 3)

	registeredUsers, err := userRepo.ListUsers(ctx, cloud.UserTypeRegistered)
	require.NoError(t, err)
	assert.Len(t, registeredUsers, 2)

	anonymousUsers, err := userRepo.ListUsers(ctx, cloud.UserTypeAnonymous)
	require.NoError(t, err)
	assert.Len(t, anonymousUsers, 1)
}

func TestClientRepository_CreateClient(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)
	ctx := context.Background()

	clientID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	client := &cloud.Client{
		ID:        clientID,
		Name:      "testclient",
		UserID:    "user123",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = clientRepo.CreateClient(ctx, client)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, "testclient", client.Name)
	assert.Equal(t, "user123", client.UserID)
	assert.Equal(t, cloud.ClientTypeRegistered, client.Type)
	assert.NotEmpty(t, client.ID)
	assert.NotZero(t, client.CreatedAt)
	assert.NotZero(t, client.UpdatedAt)

	// 测试重复ID（应该失败）
	client2 := &cloud.Client{
		ID:        clientID, // 使用相同的ID
		Name:      "testclient2",
		UserID:    "user456",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = clientRepo.CreateClient(ctx, client2)
	assert.Error(t, err)

	// 测试不同ID（应该成功）
	clientID2, err := utils.GenerateRandomString(16)
	require.NoError(t, err)
	client3 := &cloud.Client{
		ID:        clientID2,
		Name:      "testclient", // 相同名称
		UserID:    "user123",    // 相同用户ID
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = clientRepo.CreateClient(ctx, client3)
	require.NoError(t, err) // 应该成功，因为ID不同
}

func TestClientRepository_GetClient(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)
	ctx := context.Background()

	clientID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	client := &cloud.Client{
		ID:        clientID,
		Name:      "testclient",
		UserID:    "user123",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = clientRepo.CreateClient(ctx, client)
	require.NoError(t, err)

	retrievedClient, err := clientRepo.GetClient(ctx, client.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedClient)

	assert.Equal(t, client.ID, retrievedClient.ID)
	assert.Equal(t, client.Name, retrievedClient.Name)
	assert.Equal(t, client.UserID, retrievedClient.UserID)

	// 不支持GetClientByName
	// _, err = clientRepo.GetClientByName(ctx, client.Name)
	// assert.Error(t, err)

	_, err = clientRepo.GetClient(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestClientRepository_UpdateClient(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)
	ctx := context.Background()

	clientID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	client := &cloud.Client{
		ID:        clientID,
		Name:      "testclient",
		UserID:    "user123",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = clientRepo.CreateClient(ctx, client)
	require.NoError(t, err)

	client.Name = "updatedclient"
	client.Status = cloud.ClientStatusBlocked
	client.NodeID = "node123"
	err = clientRepo.UpdateClient(ctx, client)
	require.NoError(t, err)

	retrievedClient, err := clientRepo.GetClient(ctx, client.ID)
	require.NoError(t, err)
	assert.Equal(t, "updatedclient", retrievedClient.Name)
	assert.Equal(t, cloud.ClientStatusBlocked, retrievedClient.Status)
	assert.Equal(t, "node123", retrievedClient.NodeID)
}

func TestClientRepository_DeleteClient(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)
	ctx := context.Background()

	clientID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	client := &cloud.Client{
		ID:        clientID,
		Name:      "testclient",
		UserID:    "user123",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = clientRepo.CreateClient(ctx, client)
	require.NoError(t, err)

	err = clientRepo.DeleteClient(ctx, client.ID)
	require.NoError(t, err)

	_, err = clientRepo.GetClient(ctx, client.ID)
	assert.Error(t, err)
}

func TestClientRepository_ListClients(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)
	ctx := context.Background()

	clientID1, err := utils.GenerateRandomString(16)
	require.NoError(t, err)
	clientID2, err := utils.GenerateRandomString(16)
	require.NoError(t, err)
	clientID3, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	client1 := &cloud.Client{
		ID:        clientID1,
		Name:      "client1",
		UserID:    "user1",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	client2 := &cloud.Client{
		ID:        clientID2,
		Name:      "client2",
		UserID:    "user1",
		Type:      cloud.ClientTypeAnonymous,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	client3 := &cloud.Client{
		ID:        clientID3,
		Name:      "client3",
		UserID:    "user2",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = clientRepo.CreateClient(ctx, client1)
	require.NoError(t, err)
	err = clientRepo.AddClientToUser(ctx, "user1", client1)
	require.NoError(t, err)
	err = clientRepo.CreateClient(ctx, client2)
	require.NoError(t, err)
	err = clientRepo.AddClientToUser(ctx, "user1", client2)
	require.NoError(t, err)
	err = clientRepo.CreateClient(ctx, client3)
	require.NoError(t, err)
	err = clientRepo.AddClientToUser(ctx, "user2", client3)
	require.NoError(t, err)

	// List all clients for user1
	clients, err := clientRepo.ListUserClients(ctx, "user1")
	require.NoError(t, err)
	assert.Len(t, clients, 2)

	// List registered clients for user1
	registeredClients := []*cloud.Client{}
	for _, c := range clients {
		if c.Type == cloud.ClientTypeRegistered {
			registeredClients = append(registeredClients, c)
		}
	}
	assert.Len(t, registeredClients, 1)

	// List anonymous clients for user1
	anonymousClients := []*cloud.Client{}
	for _, c := range clients {
		if c.Type == cloud.ClientTypeAnonymous {
			anonymousClients = append(anonymousClients, c)
		}
	}
	assert.Len(t, anonymousClients, 1)
}

func TestPortMappingRepository_CreateMapping(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepository(repo)
	ctx := context.Background()

	mappingID, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	mapping := &cloud.PortMapping{
		ID:             mappingID,
		SourceClientID: "client1",
		TargetClientID: "client2",
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(ctx, mapping)
	require.NoError(t, err)
	require.NotNil(t, mapping)

	assert.Equal(t, "client1", mapping.SourceClientID)
	assert.Equal(t, "client2", mapping.TargetClientID)
	assert.Equal(t, cloud.ProtocolTCP, mapping.Protocol)
	assert.Equal(t, 8080, mapping.SourcePort)
	assert.Equal(t, 9090, mapping.TargetPort)
	assert.NotEmpty(t, mapping.ID)
	assert.NotZero(t, mapping.CreatedAt)
	assert.NotZero(t, mapping.UpdatedAt)
}

func TestPortMappingRepository_GetMapping(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepository(repo)
	ctx := context.Background()

	mappingID, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	mapping := &cloud.PortMapping{
		ID:             mappingID,
		SourceClientID: "client1",
		TargetClientID: "client2",
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(ctx, mapping)
	require.NoError(t, err)

	retrievedMapping, err := mappingRepo.GetPortMapping(ctx, mapping.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedMapping)

	assert.Equal(t, mapping.ID, retrievedMapping.ID)
	assert.Equal(t, mapping.SourceClientID, retrievedMapping.SourceClientID)
	assert.Equal(t, mapping.TargetClientID, retrievedMapping.TargetClientID)

	_, err = mappingRepo.GetPortMapping(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestPortMappingRepository_UpdateMapping(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepository(repo)
	ctx := context.Background()

	mappingID, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	mapping := &cloud.PortMapping{
		ID:             mappingID,
		SourceClientID: "client1",
		TargetClientID: "client2",
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(ctx, mapping)
	require.NoError(t, err)

	// 更新映射
	mapping.Status = cloud.MappingStatusInactive
	mapping.SourcePort = 8081
	err = mappingRepo.UpdatePortMapping(ctx, mapping)
	require.NoError(t, err)

	// 验证更新
	retrievedMapping, err := mappingRepo.GetPortMapping(ctx, mapping.ID)
	require.NoError(t, err)

	assert.Equal(t, cloud.MappingStatusInactive, retrievedMapping.Status)
	assert.Equal(t, 8081, retrievedMapping.SourcePort)
}

func TestPortMappingRepository_DeleteMapping(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepository(repo)
	ctx := context.Background()

	mappingID, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	mapping := &cloud.PortMapping{
		ID:             mappingID,
		SourceClientID: "client1",
		TargetClientID: "client2",
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(ctx, mapping)
	require.NoError(t, err)

	// 删除映射
	err = mappingRepo.DeletePortMapping(ctx, mapping.ID)
	require.NoError(t, err)

	// 验证删除
	_, err = mappingRepo.GetPortMapping(ctx, mapping.ID)
	assert.Error(t, err)
}

func TestPortMappingRepository_ListMappings(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepository(repo)
	ctx := context.Background()

	mappingID1, err := utils.GenerateRandomString(12)
	require.NoError(t, err)
	mappingID2, err := utils.GenerateRandomString(12)
	require.NoError(t, err)
	mappingID3, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	// 创建多个映射
	mapping1 := &cloud.PortMapping{
		ID:             mappingID1,
		SourceClientID: "client1",
		TargetClientID: "client2",
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(ctx, mapping1)
	require.NoError(t, err)
	err = mappingRepo.AddMappingToUser(ctx, "user1", mapping1)
	require.NoError(t, err)

	mapping2 := &cloud.PortMapping{
		ID:             mappingID2,
		SourceClientID: "client3",
		TargetClientID: "client4",
		Protocol:       cloud.ProtocolUDP,
		SourcePort:     8081,
		TargetPort:     9091,
		UserID:         "user1",
		Status:         cloud.MappingStatusInactive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(ctx, mapping2)
	require.NoError(t, err)
	err = mappingRepo.AddMappingToUser(ctx, "user1", mapping2)
	require.NoError(t, err)

	mapping3 := &cloud.PortMapping{
		ID:             mappingID3,
		SourceClientID: "client5",
		TargetClientID: "client6",
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8082,
		TargetPort:     9092,
		UserID:         "user2",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(ctx, mapping3)
	require.NoError(t, err)
	err = mappingRepo.AddMappingToUser(ctx, "user2", mapping3)
	require.NoError(t, err)

	// 列出用户的所有映射
	userMappings, err := mappingRepo.ListUserMappings(ctx, "user1")
	require.NoError(t, err)
	assert.Len(t, userMappings, 2)

	// 列出所有映射 (通过用户映射来验证总数)
	user2Mappings, err := mappingRepo.ListUserMappings(ctx, "user2")
	require.NoError(t, err)
	assert.Len(t, user2Mappings, 1)

	totalMappings := len(userMappings) + len(user2Mappings)
	assert.Equal(t, 3, totalMappings)

	// 验证TCP映射数量
	tcpCount := 0
	for _, m := range userMappings {
		if m.Protocol == cloud.ProtocolTCP {
			tcpCount++
		}
	}
	for _, m := range user2Mappings {
		if m.Protocol == cloud.ProtocolTCP {
			tcpCount++
		}
	}
	assert.Equal(t, 2, tcpCount)

	// 验证UDP映射数量
	udpCount := 0
	for _, m := range userMappings {
		if m.Protocol == cloud.ProtocolUDP {
			udpCount++
		}
	}
	for _, m := range user2Mappings {
		if m.Protocol == cloud.ProtocolUDP {
			udpCount++
		}
	}
	assert.Equal(t, 1, udpCount)
}

func TestNodeRepository_CreateNode(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)
	ctx := context.Background()

	nodeID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	// 创建节点
	node := &cloud.Node{
		ID:        nodeID,
		Name:      "Test Node",
		Address:   "127.0.0.1:8080",
		Meta:      map[string]string{"region": "us-west"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(ctx, node)
	require.NoError(t, err)
	require.NotNil(t, node)

	assert.Equal(t, nodeID, node.ID)
	assert.Equal(t, "Test Node", node.Name)
	assert.Equal(t, "127.0.0.1:8080", node.Address)
	assert.Equal(t, "us-west", node.Meta["region"])
	assert.NotZero(t, node.CreatedAt)
	assert.NotZero(t, node.UpdatedAt)

	// 测试重复ID
	node2 := &cloud.Node{
		ID:        nodeID,
		Name:      "Another Node",
		Address:   "127.0.0.1:8081",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(ctx, node2)
	assert.Error(t, err)
}

func TestNodeRepository_GetNode(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)
	ctx := context.Background()

	nodeID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	// 创建节点
	node := &cloud.Node{
		ID:        nodeID,
		Name:      "Test Node",
		Address:   "127.0.0.1:8080",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(ctx, node)
	require.NoError(t, err)

	// 通过ID获取节点
	retrievedNode, err := nodeRepo.GetNode(ctx, node.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedNode)

	assert.Equal(t, node.ID, retrievedNode.ID)
	assert.Equal(t, node.Name, retrievedNode.Name)
	assert.Equal(t, node.Address, retrievedNode.Address)

	// 测试不存在的节点
	_, err = nodeRepo.GetNode(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestNodeRepository_UpdateNode(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)
	ctx := context.Background()

	nodeID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	// 创建节点
	node := &cloud.Node{
		ID:        nodeID,
		Name:      "Test Node",
		Address:   "127.0.0.1:8080",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(ctx, node)
	require.NoError(t, err)

	// 更新节点
	node.Name = "Updated Node"
	node.Address = "127.0.0.1:8081"

	err = nodeRepo.UpdateNode(ctx, node)
	require.NoError(t, err)

	// 验证更新
	retrievedNode, err := nodeRepo.GetNode(ctx, node.ID)
	require.NoError(t, err)

	assert.Equal(t, "Updated Node", retrievedNode.Name)
	assert.Equal(t, "127.0.0.1:8081", retrievedNode.Address)
}

func TestNodeRepository_DeleteNode(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)
	ctx := context.Background()

	nodeID, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	// 创建节点
	node := &cloud.Node{
		ID:        nodeID,
		Name:      "Test Node",
		Address:   "127.0.0.1:8080",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(ctx, node)
	require.NoError(t, err)

	// 删除节点
	err = nodeRepo.DeleteNode(ctx, node.ID)
	require.NoError(t, err)

	// 验证删除
	_, err = nodeRepo.GetNode(ctx, node.ID)
	assert.Error(t, err)
}

func TestNodeRepository_ListNodes(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)
	ctx := context.Background()

	nodeID1, err := utils.GenerateRandomString(16)
	require.NoError(t, err)
	nodeID2, err := utils.GenerateRandomString(16)
	require.NoError(t, err)
	nodeID3, err := utils.GenerateRandomString(16)
	require.NoError(t, err)

	// 创建多个节点
	node1 := &cloud.Node{
		ID:        nodeID1,
		Name:      "Node 1",
		Address:   "127.0.0.1:8080",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(ctx, node1)
	require.NoError(t, err)
	err = nodeRepo.AddNodeToList(ctx, node1)
	require.NoError(t, err)

	node2 := &cloud.Node{
		ID:        nodeID2,
		Name:      "Node 2",
		Address:   "127.0.0.1:8081",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(ctx, node2)
	require.NoError(t, err)
	err = nodeRepo.AddNodeToList(ctx, node2)
	require.NoError(t, err)

	node3 := &cloud.Node{
		ID:        nodeID3,
		Name:      "Node 3",
		Address:   "127.0.0.1:8082",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(ctx, node3)
	require.NoError(t, err)
	err = nodeRepo.AddNodeToList(ctx, node3)
	require.NoError(t, err)

	// 列出所有节点
	nodes, err := nodeRepo.ListNodes(ctx)
	require.NoError(t, err)
	assert.Len(t, nodes, 3)

	// 验证节点列表包含所有创建的节点
	nodeIDs := make(map[string]bool)
	for _, node := range nodes {
		nodeIDs[node.ID] = true
	}

	assert.True(t, nodeIDs[nodeID1])
	assert.True(t, nodeIDs[nodeID2])
	assert.True(t, nodeIDs[nodeID3])
}

func TestRepository_KeyPrefixes(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	repo := cloud.NewRepository(storage)
	userRepo := cloud.NewUserRepository(repo)
	clientRepo := cloud.NewClientRepository(repo)
	mappingRepo := cloud.NewPortMappingRepository(repo)
	nodeRepo := cloud.NewNodeRepository(repo)
	ctx := context.Background()

	t.Run("Verify Key Prefixes", func(t *testing.T) {
		// 创建测试数据
		user := &cloud.User{
			ID:        "prefix_test_user",
			Username:  "prefixtest",
			Email:     "prefix@example.com",
			Status:    cloud.UserStatusActive,
			Type:      cloud.UserTypeRegistered,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		client := &cloud.Client{
			ID:        "prefix_test_client",
			UserID:    user.ID,
			Name:      "Prefix Test Client",
			AuthCode:  "prefix_auth",
			SecretKey: "prefix_secret",
			Status:    cloud.ClientStatusOffline,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mapping := &cloud.PortMapping{
			ID:             "prefix_test_mapping",
			UserID:         user.ID,
			SourceClientID: client.ID,
			TargetClientID: "target_client",
			Protocol:       cloud.ProtocolTCP,
			SourcePort:     9090,
			TargetHost:     "localhost",
			TargetPort:     90,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		node := &cloud.Node{
			ID:        "prefix_test_node",
			Name:      "Prefix Test Node",
			Address:   "192.168.1.200:8080",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// 保存数据
		err := userRepo.SaveUser(ctx, user)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		err = clientRepo.SaveClient(ctx, client)
		if err != nil {
			t.Fatalf("SaveClient failed: %v", err)
		}

		err = mappingRepo.SavePortMapping(ctx, mapping)
		if err != nil {
			t.Fatalf("SavePortMapping failed: %v", err)
		}

		err = nodeRepo.SaveNode(ctx, node)
		if err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}

		// 验证键值前缀
		expectedUserKey := "tunnox:user:prefix_test_user"
		exists, err := storage.Exists(ctx, expectedUserKey)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Errorf("Expected key %s to exist", expectedUserKey)
		}

		expectedClientKey := "tunnox:client:prefix_test_client"
		exists, err = storage.Exists(ctx, expectedClientKey)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Errorf("Expected key %s to exist", expectedClientKey)
		}

		expectedMappingKey := "tunnox:port_mapping:prefix_test_mapping"
		exists, err = storage.Exists(ctx, expectedMappingKey)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Errorf("Expected key %s to exist", expectedMappingKey)
		}

		expectedNodeKey := "tunnox:node:prefix_test_node"
		exists, err = storage.Exists(ctx, expectedNodeKey)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Errorf("Expected key %s to exist", expectedNodeKey)
		}
	})
}
