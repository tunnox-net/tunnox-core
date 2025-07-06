package tests

import (
	"context"
	"fmt"
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

	user := &cloud.User{
		ID:        "testuser",
		Email:     "test@example.com",
		Type:      cloud.UserTypeRegistered,
		Status:    cloud.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := userRepo.CreateUser(user)
	require.NoError(t, err)
	require.NotNil(t, user)

	assert.Equal(t, "testuser", user.ID)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, cloud.UserTypeRegistered, user.Type)
	assert.Equal(t, cloud.UserStatusActive, user.Status)
	assert.NotZero(t, user.CreatedAt)
	assert.NotZero(t, user.UpdatedAt)

	// 测试重复ID（应该失败）
	user2 := &cloud.User{
		ID:        "testuser", // 使用相同的ID
		Email:     "test2@example.com",
		Type:      cloud.UserTypeRegistered,
		Status:    cloud.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(user2)
	assert.Error(t, err)

	// 测试不同ID（应该成功）
	user3 := &cloud.User{
		ID:        "testuser2",
		Email:     "test@example.com", // 相同邮箱
		Type:      cloud.UserTypeRegistered,
		Status:    cloud.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = userRepo.CreateUser(user3)
	require.NoError(t, err) // 应该成功，因为ID不同
}

func TestUserRepository_GetUser(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)

	user := &cloud.User{
		ID:        "testuser",
		Email:     "test@example.com",
		Type:      cloud.UserTypeRegistered,
		Status:    cloud.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := userRepo.CreateUser(user)
	require.NoError(t, err)

	retrievedUser, err := userRepo.GetUser(user.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedUser)

	assert.Equal(t, user.ID, retrievedUser.ID)
	assert.Equal(t, user.Email, retrievedUser.Email)
	assert.Equal(t, user.Type, retrievedUser.Type)

	_, err = userRepo.GetUser("nonexistent")
	assert.Error(t, err)
}

func TestUserRepository_UpdateUser(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)

	user := &cloud.User{
		ID:        "testuser",
		Email:     "test@example.com",
		Type:      cloud.UserTypeRegistered,
		Status:    cloud.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := userRepo.CreateUser(user)
	require.NoError(t, err)

	user.Email = "updated@example.com"
	user.Status = cloud.UserStatusSuspended
	err = userRepo.UpdateUser(user)
	require.NoError(t, err)

	retrievedUser, err := userRepo.GetUser(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated@example.com", retrievedUser.Email)
	assert.Equal(t, cloud.UserStatusSuspended, retrievedUser.Status)
}

func TestUserRepository_DeleteUser(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)

	user := &cloud.User{
		ID:        "testuser",
		Email:     "test@example.com",
		Type:      cloud.UserTypeRegistered,
		Status:    cloud.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := userRepo.CreateUser(user)
	require.NoError(t, err)

	err = userRepo.DeleteUser(user.ID)
	require.NoError(t, err)

	_, err = userRepo.GetUser(user.ID)
	assert.Error(t, err)
}

func TestUserRepository_ListUsers(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	userRepo := cloud.NewUserRepository(repo)

	user1 := &cloud.User{
		ID:        "user1",
		Email:     "user1@example.com",
		Type:      cloud.UserTypeRegistered,
		Status:    cloud.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	user2 := &cloud.User{
		ID:        "user2",
		Email:     "user2@example.com",
		Type:      cloud.UserTypeAnonymous,
		Status:    cloud.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := userRepo.CreateUser(user1)
	require.NoError(t, err)
	err = userRepo.AddUserToList(user1)
	require.NoError(t, err)

	err = userRepo.CreateUser(user2)
	require.NoError(t, err)
	err = userRepo.AddUserToList(user2)
	require.NoError(t, err)

	// List all users
	users, err := userRepo.ListUsers("")
	require.NoError(t, err)
	assert.Len(t, users, 2)

	// List registered users only
	registeredUsers, err := userRepo.ListUsers(cloud.UserTypeRegistered)
	require.NoError(t, err)
	assert.Len(t, registeredUsers, 1)
	assert.Equal(t, "user1", registeredUsers[0].ID)
}

func TestClientRepository_CreateClient(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)

	clientID := int64(12345678) // 使用 int64 类型的 ClientID

	client := &cloud.Client{
		ID:        clientID,
		Name:      "testclient",
		UserID:    "user123",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := clientRepo.CreateClient(client)
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
	err = clientRepo.CreateClient(client2)
	assert.Error(t, err)

	// 测试不同ID（应该成功）
	clientID2 := int64(87654321) // 使用不同的 int64 ID
	client3 := &cloud.Client{
		ID:        clientID2,
		Name:      "testclient", // 相同名称
		UserID:    "user123",    // 相同用户ID
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = clientRepo.CreateClient(client3)
	require.NoError(t, err) // 应该成功，因为ID不同
}

func TestClientRepository_GetClient(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)

	clientID := int64(12345678)

	client := &cloud.Client{
		ID:        clientID,
		Name:      "testclient",
		UserID:    "user123",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := clientRepo.CreateClient(client)
	require.NoError(t, err)

	retrievedClient, err := clientRepo.GetClient(fmt.Sprintf("%d", client.ID))
	require.NoError(t, err)
	require.NotNil(t, retrievedClient)

	assert.Equal(t, client.ID, retrievedClient.ID)
	assert.Equal(t, client.Name, retrievedClient.Name)
	assert.Equal(t, client.UserID, retrievedClient.UserID)

	_, err = clientRepo.GetClient("nonexistent")
	assert.Error(t, err)
}

func TestClientRepository_UpdateClient(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)

	clientID := int64(12345678)

	client := &cloud.Client{
		ID:        clientID,
		Name:      "testclient",
		UserID:    "user123",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := clientRepo.CreateClient(client)
	require.NoError(t, err)

	client.Name = "updatedclient"
	client.Status = cloud.ClientStatusBlocked
	client.NodeID = "node123"
	err = clientRepo.UpdateClient(client)
	require.NoError(t, err)

	retrievedClient, err := clientRepo.GetClient(fmt.Sprintf("%d", client.ID))
	require.NoError(t, err)
	assert.Equal(t, "updatedclient", retrievedClient.Name)
	assert.Equal(t, cloud.ClientStatusBlocked, retrievedClient.Status)
	assert.Equal(t, "node123", retrievedClient.NodeID)
}

func TestClientRepository_DeleteClient(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)

	clientID := int64(12345678)

	client := &cloud.Client{
		ID:        clientID,
		Name:      "testclient",
		UserID:    "user123",
		Type:      cloud.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := clientRepo.CreateClient(client)
	require.NoError(t, err)

	err = clientRepo.DeleteClient(fmt.Sprintf("%d", client.ID))
	require.NoError(t, err)

	_, err = clientRepo.GetClient(fmt.Sprintf("%d", client.ID))
	assert.Error(t, err)
}

func TestClientRepository_ListClients(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	clientRepo := cloud.NewClientRepository(repo)

	clientID1 := int64(12345678)
	clientID2 := int64(87654321)
	clientID3 := int64(11111111)

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
	err := clientRepo.CreateClient(client1)
	require.NoError(t, err)
	err = clientRepo.AddClientToUser("user1", client1)
	require.NoError(t, err)
	err = clientRepo.CreateClient(client2)
	require.NoError(t, err)
	err = clientRepo.AddClientToUser("user1", client2)
	require.NoError(t, err)
	err = clientRepo.CreateClient(client3)
	require.NoError(t, err)
	err = clientRepo.AddClientToUser("user2", client3)
	require.NoError(t, err)

	// List all clients for user1
	clients, err := clientRepo.ListUserClients("user1")
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
	assert.Equal(t, "client1", registeredClients[0].Name)

	// List all clients for user2
	clients, err = clientRepo.ListUserClients("user2")
	require.NoError(t, err)
	assert.Len(t, clients, 1)
	assert.Equal(t, "client3", clients[0].Name)

	// List all clients
	allClients, err := clientRepo.ListClients()
	require.NoError(t, err)
	assert.Len(t, allClients, 3)
}

func TestPortMappingRepo_CreateMapping(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	repo := cloud.NewRepository(storage)
	mappingRepo := cloud.NewPortMappingRepo(repo)

	mappingID, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	mapping := &cloud.PortMapping{
		ID:             mappingID,
		SourceClientID: 1,
		TargetClientID: 2,
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(mapping)
	require.NoError(t, err)
	require.NotNil(t, mapping)

	assert.Equal(t, int64(1), mapping.SourceClientID)
	assert.Equal(t, int64(2), mapping.TargetClientID)
	assert.Equal(t, cloud.ProtocolTCP, mapping.Protocol)
	assert.Equal(t, 8080, mapping.SourcePort)
	assert.Equal(t, 9090, mapping.TargetPort)
	assert.NotEmpty(t, mapping.ID)
	assert.NotZero(t, mapping.CreatedAt)
	assert.NotZero(t, mapping.UpdatedAt)
}

func TestPortMappingRepo_GetMapping(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepo(repo)

	mappingID, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	mapping := &cloud.PortMapping{
		ID:             mappingID,
		SourceClientID: 1,
		TargetClientID: 2,
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(mapping)
	require.NoError(t, err)

	retrievedMapping, err := mappingRepo.GetPortMapping(mapping.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedMapping)

	assert.Equal(t, mapping.ID, retrievedMapping.ID)
	assert.Equal(t, mapping.SourceClientID, retrievedMapping.SourceClientID)
	assert.Equal(t, mapping.TargetClientID, retrievedMapping.TargetClientID)

	_, err = mappingRepo.GetPortMapping("nonexistent")
	assert.Error(t, err)
}

func TestPortMappingRepo_UpdateMapping(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepo(repo)

	mappingID, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	mapping := &cloud.PortMapping{
		ID:             mappingID,
		SourceClientID: 1,
		TargetClientID: 2,
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(mapping)
	require.NoError(t, err)

	// 更新映射
	mapping.Status = cloud.MappingStatusInactive
	mapping.SourcePort = 8081
	err = mappingRepo.UpdatePortMapping(mapping)
	require.NoError(t, err)

	// 验证更新
	retrievedMapping, err := mappingRepo.GetPortMapping(mapping.ID)
	require.NoError(t, err)

	assert.Equal(t, cloud.MappingStatusInactive, retrievedMapping.Status)
	assert.Equal(t, 8081, retrievedMapping.SourcePort)
}

func TestPortMappingRepo_DeleteMapping(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepo(repo)

	mappingID, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	mapping := &cloud.PortMapping{
		ID:             mappingID,
		SourceClientID: 1,
		TargetClientID: 2,
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(mapping)
	require.NoError(t, err)

	// 删除映射
	err = mappingRepo.DeletePortMapping(mapping.ID)
	require.NoError(t, err)

	// 验证删除
	_, err = mappingRepo.GetPortMapping(mapping.ID)
	assert.Error(t, err)
}

func TestPortMappingRepo_ListMappings(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	mappingRepo := cloud.NewPortMappingRepo(repo)

	mappingID1, err := utils.GenerateRandomString(12)
	require.NoError(t, err)
	mappingID2, err := utils.GenerateRandomString(12)
	require.NoError(t, err)
	mappingID3, err := utils.GenerateRandomString(12)
	require.NoError(t, err)

	// 创建多个映射
	mapping1 := &cloud.PortMapping{
		ID:             mappingID1,
		SourceClientID: 1,
		TargetClientID: 2,
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     9090,
		UserID:         "user1",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(mapping1)
	require.NoError(t, err)
	err = mappingRepo.AddMappingToUser("user1", mapping1)
	require.NoError(t, err)

	mapping2 := &cloud.PortMapping{
		ID:             mappingID2,
		SourceClientID: 3,
		TargetClientID: 4,
		Protocol:       cloud.ProtocolUDP,
		SourcePort:     8081,
		TargetPort:     9091,
		UserID:         "user1",
		Status:         cloud.MappingStatusInactive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(mapping2)
	require.NoError(t, err)
	err = mappingRepo.AddMappingToUser("user1", mapping2)
	require.NoError(t, err)

	mapping3 := &cloud.PortMapping{
		ID:             mappingID3,
		SourceClientID: 5,
		TargetClientID: 6,
		Protocol:       cloud.ProtocolTCP,
		SourcePort:     8082,
		TargetPort:     9092,
		UserID:         "user2",
		Status:         cloud.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = mappingRepo.CreatePortMapping(mapping3)
	require.NoError(t, err)
	err = mappingRepo.AddMappingToUser("user2", mapping3)
	require.NoError(t, err)

	// 列出用户的所有映射
	userMappings, err := mappingRepo.GetUserPortMappings("user1")
	require.NoError(t, err)
	assert.Len(t, userMappings, 2)

	// 列出所有映射 (通过用户映射来验证总数)
	user2Mappings, err := mappingRepo.GetUserPortMappings("user2")
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

	node := &cloud.Node{
		ID:        "testnode",
		Name:      "Test Node",
		Address:   "127.0.0.1:8080",
		Meta:      map[string]string{"region": "us-west"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := nodeRepo.CreateNode(node)
	require.NoError(t, err)
	require.NotNil(t, node)

	assert.Equal(t, "testnode", node.ID)
	assert.Equal(t, "Test Node", node.Name)
	assert.Equal(t, "127.0.0.1:8080", node.Address)
	assert.Equal(t, "us-west", node.Meta["region"])
	assert.NotZero(t, node.CreatedAt)
	assert.NotZero(t, node.UpdatedAt)

	// 测试重复ID
	node2 := &cloud.Node{
		ID:        "testnode",
		Name:      "Another Node",
		Address:   "127.0.0.1:8081",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(node2)
	assert.Error(t, err)
}

func TestNodeRepository_GetNode(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)

	node := &cloud.Node{
		ID:        "testnode",
		Name:      "Test Node",
		Address:   "127.0.0.1:8080",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := nodeRepo.CreateNode(node)
	require.NoError(t, err)

	retrievedNode, err := nodeRepo.GetNode(node.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedNode)

	assert.Equal(t, node.ID, retrievedNode.ID)
	assert.Equal(t, node.Name, retrievedNode.Name)
	assert.Equal(t, node.Address, retrievedNode.Address)

	_, err = nodeRepo.GetNode("nonexistent")
	assert.Error(t, err)
}

func TestNodeRepository_UpdateNode(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)

	node := &cloud.Node{
		ID:        "testnode",
		Name:      "Test Node",
		Address:   "127.0.0.1:8080",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := nodeRepo.CreateNode(node)
	require.NoError(t, err)

	// 更新节点
	node.Name = "Updated Node"
	node.Address = "127.0.0.1:8081"

	err = nodeRepo.UpdateNode(node)
	require.NoError(t, err)

	// 验证更新
	retrievedNode, err := nodeRepo.GetNode(node.ID)
	require.NoError(t, err)

	assert.Equal(t, "Updated Node", retrievedNode.Name)
	assert.Equal(t, "127.0.0.1:8081", retrievedNode.Address)
}

func TestNodeRepository_DeleteNode(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)

	node := &cloud.Node{
		ID:        "testnode",
		Name:      "Test Node",
		Address:   "127.0.0.1:8080",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := nodeRepo.CreateNode(node)
	require.NoError(t, err)

	// 删除节点
	err = nodeRepo.DeleteNode(node.ID)
	require.NoError(t, err)

	// 验证删除
	_, err = nodeRepo.GetNode(node.ID)
	assert.Error(t, err)
}

func TestNodeRepository_ListNodes(t *testing.T) {
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	nodeRepo := cloud.NewNodeRepository(repo)

	node1 := &cloud.Node{
		ID:        "node1",
		Name:      "Node 1",
		Address:   "127.0.0.1:8080",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := nodeRepo.CreateNode(node1)
	require.NoError(t, err)
	err = nodeRepo.AddNodeToList(node1)
	require.NoError(t, err)

	node2 := &cloud.Node{
		ID:        "node2",
		Name:      "Node 2",
		Address:   "127.0.0.1:8081",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = nodeRepo.CreateNode(node2)
	require.NoError(t, err)
	err = nodeRepo.AddNodeToList(node2)
	require.NoError(t, err)

	// 列出所有节点
	nodes, err := nodeRepo.ListNodes()
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	// 验证节点列表包含所有创建的节点
	nodeIDs := make(map[string]bool)
	for _, node := range nodes {
		nodeIDs[node.ID] = true
	}

	assert.True(t, nodeIDs["node1"])
	assert.True(t, nodeIDs["node2"])
}

func TestRepository_KeyPrefixes(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	repo := cloud.NewRepository(storage)
	userRepo := cloud.NewUserRepository(repo)
	clientRepo := cloud.NewClientRepository(repo)
	mappingRepo := cloud.NewPortMappingRepo(repo)
	nodeRepo := cloud.NewNodeRepository(repo)

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
			ID:        12345,
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
			TargetClientID: 67890,
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
		err := userRepo.SaveUser(user)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		err = clientRepo.SaveClient(client)
		if err != nil {
			t.Fatalf("SaveClient failed: %v", err)
		}

		err = mappingRepo.SavePortMapping(mapping)
		if err != nil {
			t.Fatalf("SavePortMapping failed: %v", err)
		}

		err = nodeRepo.SaveNode(node)
		if err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}

		// 验证键值前缀
		expectedUserKey := "tunnox:user:prefix_test_user"
		exists, err := storage.Exists(expectedUserKey)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Errorf("Expected key %s to exist", expectedUserKey)
		}

		expectedClientKey := "tunnox:client:12345"
		exists, err = storage.Exists(expectedClientKey)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Errorf("Expected key %s to exist", expectedClientKey)
		}

		expectedMappingKey := "tunnox:port_mapping:prefix_test_mapping"
		exists, err = storage.Exists(expectedMappingKey)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Errorf("Expected key %s to exist", expectedMappingKey)
		}

		expectedNodeKey := "tunnox:node:prefix_test_node"
		exists, err = storage.Exists(expectedNodeKey)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Errorf("Expected key %s to exist", expectedNodeKey)
		}
	})
}
