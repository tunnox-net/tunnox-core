package tests

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud"
)

func TestUserRepository(t *testing.T) {
	storage := cloud.NewMemoryStorage()
	defer storage.Close()

	repo := cloud.NewRepository(storage)
	userRepo := cloud.NewUserRepository(repo)
	ctx := context.Background()

	t.Run("SaveUser and GetUser", func(t *testing.T) {
		user := &cloud.User{
			ID:        "test_user_1",
			Username:  "testuser",
			Email:     "test@example.com",
			Status:    cloud.UserStatusActive,
			Type:      cloud.UserTypeRegistered,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Plan:      cloud.UserPlanFree,
			Quota: cloud.UserQuota{
				MaxClientIds:   5,
				MaxConnections: 10,
				BandwidthLimit: 1024 * 1024,       // 1MB
				StorageLimit:   100 * 1024 * 1024, // 100MB
			},
		}

		err := userRepo.SaveUser(ctx, user)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		retrieved, err := userRepo.GetUser(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if retrieved.ID != user.ID {
			t.Errorf("Expected user ID %s, got %s", user.ID, retrieved.ID)
		}
		if retrieved.Username != user.Username {
			t.Errorf("Expected username %s, got %s", user.Username, retrieved.Username)
		}
		if retrieved.Email != user.Email {
			t.Errorf("Expected email %s, got %s", user.Email, retrieved.Email)
		}
	})

	t.Run("DeleteUser", func(t *testing.T) {
		user := &cloud.User{
			ID:        "test_user_2",
			Username:  "testuser2",
			Email:     "test2@example.com",
			Status:    cloud.UserStatusActive,
			Type:      cloud.UserTypeRegistered,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := userRepo.SaveUser(ctx, user)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		err = userRepo.DeleteUser(ctx, user.ID)
		if err != nil {
			t.Fatalf("DeleteUser failed: %v", err)
		}

		_, err = userRepo.GetUser(ctx, user.ID)
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("ListUsers and AddUserToList", func(t *testing.T) {
		user1 := &cloud.User{
			ID:        "list_user_1",
			Username:  "listuser1",
			Email:     "list1@example.com",
			Status:    cloud.UserStatusActive,
			Type:      cloud.UserTypeRegistered,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		user2 := &cloud.User{
			ID:        "list_user_2",
			Username:  "listuser2",
			Email:     "list2@example.com",
			Status:    cloud.UserStatusActive,
			Type:      cloud.UserTypeAnonymous,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := userRepo.AddUserToList(ctx, user1)
		if err != nil {
			t.Fatalf("AddUserToList failed: %v", err)
		}

		err = userRepo.AddUserToList(ctx, user2)
		if err != nil {
			t.Fatalf("AddUserToList failed: %v", err)
		}

		// 列出所有用户
		users, err := userRepo.ListUsers(ctx, "")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}

		if len(users) < 2 {
			t.Errorf("Expected at least 2 users, got %d", len(users))
		}

		// 列出注册用户
		registeredUsers, err := userRepo.ListUsers(ctx, cloud.UserTypeRegistered)
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}

		found := false
		for _, u := range registeredUsers {
			if u.ID == user1.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find registered user in list")
		}
	})
}

func TestClientRepository(t *testing.T) {
	storage := cloud.NewMemoryStorage()
	defer storage.Close()

	repo := cloud.NewRepository(storage)
	clientRepo := cloud.NewClientRepository(repo)
	ctx := context.Background()

	t.Run("SaveClient and GetClient", func(t *testing.T) {
		client := &cloud.Client{
			ID:        "test_client_1",
			UserID:    "test_user_1",
			Name:      "Test Client",
			AuthCode:  "auth123",
			SecretKey: "secret123",
			Status:    cloud.ClientStatusOffline,
			Config: cloud.ClientConfig{
				EnableCompression: true,
				BandwidthLimit:    1024 * 1024,
				MaxConnections:    5,
				AllowedPorts:      []int{80, 443, 8080},
				BlockedPorts:      []int{22, 23},
				AutoReconnect:     true,
				HeartbeatInterval: 30,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Type:      cloud.ClientTypeRegistered,
		}

		err := clientRepo.SaveClient(ctx, client)
		if err != nil {
			t.Fatalf("SaveClient failed: %v", err)
		}

		retrieved, err := clientRepo.GetClient(ctx, client.ID)
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}

		if retrieved.ID != client.ID {
			t.Errorf("Expected client ID %s, got %s", client.ID, retrieved.ID)
		}
		if retrieved.Name != client.Name {
			t.Errorf("Expected client name %s, got %s", client.Name, retrieved.Name)
		}
		if retrieved.AuthCode != client.AuthCode {
			t.Errorf("Expected auth code %s, got %s", client.AuthCode, retrieved.AuthCode)
		}
	})

	t.Run("DeleteClient", func(t *testing.T) {
		client := &cloud.Client{
			ID:        "test_client_2",
			UserID:    "test_user_1",
			Name:      "Test Client 2",
			AuthCode:  "auth456",
			SecretKey: "secret456",
			Status:    cloud.ClientStatusOffline,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := clientRepo.SaveClient(ctx, client)
		if err != nil {
			t.Fatalf("SaveClient failed: %v", err)
		}

		err = clientRepo.DeleteClient(ctx, client.ID)
		if err != nil {
			t.Fatalf("DeleteClient failed: %v", err)
		}

		_, err = clientRepo.GetClient(ctx, client.ID)
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("UpdateClientStatus", func(t *testing.T) {
		client := &cloud.Client{
			ID:        "test_client_3",
			UserID:    "test_user_1",
			Name:      "Test Client 3",
			AuthCode:  "auth789",
			SecretKey: "secret789",
			Status:    cloud.ClientStatusOffline,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := clientRepo.SaveClient(ctx, client)
		if err != nil {
			t.Fatalf("SaveClient failed: %v", err)
		}

		err = clientRepo.UpdateClientStatus(ctx, client.ID, cloud.ClientStatusOnline, "node_1")
		if err != nil {
			t.Fatalf("UpdateClientStatus failed: %v", err)
		}

		updated, err := clientRepo.GetClient(ctx, client.ID)
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}

		if updated.Status != cloud.ClientStatusOnline {
			t.Errorf("Expected status %s, got %s", cloud.ClientStatusOnline, updated.Status)
		}
		if updated.NodeID != "node_1" {
			t.Errorf("Expected node ID %s, got %s", "node_1", updated.NodeID)
		}
		if updated.LastSeen == nil {
			t.Error("Expected LastSeen to be set")
		}
	})

	t.Run("ListUserClients and AddClientToUser", func(t *testing.T) {
		client1 := &cloud.Client{
			ID:        "list_client_1",
			UserID:    "test_user_1",
			Name:      "List Client 1",
			AuthCode:  "auth_list1",
			SecretKey: "secret_list1",
			Status:    cloud.ClientStatusOffline,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		client2 := &cloud.Client{
			ID:        "list_client_2",
			UserID:    "test_user_1",
			Name:      "List Client 2",
			AuthCode:  "auth_list2",
			SecretKey: "secret_list2",
			Status:    cloud.ClientStatusOffline,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := clientRepo.AddClientToUser(ctx, "test_user_1", client1)
		if err != nil {
			t.Fatalf("AddClientToUser failed: %v", err)
		}

		err = clientRepo.AddClientToUser(ctx, "test_user_1", client2)
		if err != nil {
			t.Fatalf("AddClientToUser failed: %v", err)
		}

		clients, err := clientRepo.ListUserClients(ctx, "test_user_1")
		if err != nil {
			t.Fatalf("ListUserClients failed: %v", err)
		}

		if len(clients) < 2 {
			t.Errorf("Expected at least 2 clients, got %d", len(clients))
		}

		found := false
		for _, c := range clients {
			if c.ID == client1.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find client in list")
		}
	})
}

func TestPortMappingRepository(t *testing.T) {
	storage := cloud.NewMemoryStorage()
	defer storage.Close()

	repo := cloud.NewRepository(storage)
	mappingRepo := cloud.NewPortMappingRepository(repo)
	ctx := context.Background()

	t.Run("SavePortMapping and GetPortMapping", func(t *testing.T) {
		mapping := &cloud.PortMapping{
			ID:             "test_mapping_1",
			UserID:         "test_user_1",
			SourceClientID: "test_client_1",
			TargetClientID: "test_client_2",
			Protocol:       cloud.ProtocolTCP,
			SourcePort:     8080,
			TargetHost:     "localhost",
			TargetPort:     80,
			Status:         cloud.MappingStatusActive,
			Config: cloud.MappingConfig{
				EnableCompression: true,
				BandwidthLimit:    1024 * 1024,
				Timeout:           30,
				RetryCount:        3,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Type:      cloud.MappingTypeRegistered,
		}

		err := mappingRepo.SavePortMapping(ctx, mapping)
		if err != nil {
			t.Fatalf("SavePortMapping failed: %v", err)
		}

		retrieved, err := mappingRepo.GetPortMapping(ctx, mapping.ID)
		if err != nil {
			t.Fatalf("GetPortMapping failed: %v", err)
		}

		if retrieved.ID != mapping.ID {
			t.Errorf("Expected mapping ID %s, got %s", mapping.ID, retrieved.ID)
		}
		if retrieved.Protocol != mapping.Protocol {
			t.Errorf("Expected protocol %s, got %s", mapping.Protocol, retrieved.Protocol)
		}
		if retrieved.SourcePort != mapping.SourcePort {
			t.Errorf("Expected source port %d, got %d", mapping.SourcePort, retrieved.SourcePort)
		}
	})

	t.Run("DeletePortMapping", func(t *testing.T) {
		mapping := &cloud.PortMapping{
			ID:             "test_mapping_2",
			UserID:         "test_user_1",
			SourceClientID: "test_client_1",
			TargetClientID: "test_client_2",
			Protocol:       cloud.ProtocolTCP,
			SourcePort:     8081,
			TargetHost:     "localhost",
			TargetPort:     81,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		err := mappingRepo.SavePortMapping(ctx, mapping)
		if err != nil {
			t.Fatalf("SavePortMapping failed: %v", err)
		}

		err = mappingRepo.DeletePortMapping(ctx, mapping.ID)
		if err != nil {
			t.Fatalf("DeletePortMapping failed: %v", err)
		}

		_, err = mappingRepo.GetPortMapping(ctx, mapping.ID)
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("UpdatePortMappingStatus", func(t *testing.T) {
		mapping := &cloud.PortMapping{
			ID:             "test_mapping_3",
			UserID:         "test_user_1",
			SourceClientID: "test_client_1",
			TargetClientID: "test_client_2",
			Protocol:       cloud.ProtocolTCP,
			SourcePort:     8082,
			TargetHost:     "localhost",
			TargetPort:     82,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		err := mappingRepo.SavePortMapping(ctx, mapping)
		if err != nil {
			t.Fatalf("SavePortMapping failed: %v", err)
		}

		err = mappingRepo.UpdatePortMappingStatus(ctx, mapping.ID, cloud.MappingStatusInactive)
		if err != nil {
			t.Fatalf("UpdatePortMappingStatus failed: %v", err)
		}

		updated, err := mappingRepo.GetPortMapping(ctx, mapping.ID)
		if err != nil {
			t.Fatalf("GetPortMapping failed: %v", err)
		}

		if updated.Status != cloud.MappingStatusInactive {
			t.Errorf("Expected status %s, got %s", cloud.MappingStatusInactive, updated.Status)
		}
	})

	t.Run("UpdatePortMappingStats", func(t *testing.T) {
		mapping := &cloud.PortMapping{
			ID:             "test_mapping_4",
			UserID:         "test_user_1",
			SourceClientID: "test_client_1",
			TargetClientID: "test_client_2",
			Protocol:       cloud.ProtocolTCP,
			SourcePort:     8083,
			TargetHost:     "localhost",
			TargetPort:     83,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		err := mappingRepo.SavePortMapping(ctx, mapping)
		if err != nil {
			t.Fatalf("SavePortMapping failed: %v", err)
		}

		stats := &cloud.TrafficStats{
			BytesSent:     1024,
			BytesReceived: 2048,
			Connections:   5,
		}

		err = mappingRepo.UpdatePortMappingStats(ctx, mapping.ID, stats)
		if err != nil {
			t.Fatalf("UpdatePortMappingStats failed: %v", err)
		}

		updated, err := mappingRepo.GetPortMapping(ctx, mapping.ID)
		if err != nil {
			t.Fatalf("GetPortMapping failed: %v", err)
		}

		if updated.TrafficStats.BytesSent != stats.BytesSent {
			t.Errorf("Expected bytes sent %d, got %d", stats.BytesSent, updated.TrafficStats.BytesSent)
		}
		if updated.TrafficStats.BytesReceived != stats.BytesReceived {
			t.Errorf("Expected bytes received %d, got %d", stats.BytesReceived, updated.TrafficStats.BytesReceived)
		}
		if updated.TrafficStats.Connections != stats.Connections {
			t.Errorf("Expected connections %d, got %d", stats.Connections, updated.TrafficStats.Connections)
		}
		if updated.LastActive == nil {
			t.Error("Expected LastActive to be set")
		}
	})

	t.Run("ListUserMappings and AddMappingToUser", func(t *testing.T) {
		mapping1 := &cloud.PortMapping{
			ID:             "list_mapping_1",
			UserID:         "test_user_1",
			SourceClientID: "test_client_1",
			TargetClientID: "test_client_2",
			Protocol:       cloud.ProtocolTCP,
			SourcePort:     8084,
			TargetHost:     "localhost",
			TargetPort:     84,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		mapping2 := &cloud.PortMapping{
			ID:             "list_mapping_2",
			UserID:         "test_user_1",
			SourceClientID: "test_client_2",
			TargetClientID: "test_client_3",
			Protocol:       cloud.ProtocolUDP,
			SourcePort:     8085,
			TargetHost:     "localhost",
			TargetPort:     85,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		err := mappingRepo.AddMappingToUser(ctx, "test_user_1", mapping1)
		if err != nil {
			t.Fatalf("AddMappingToUser failed: %v", err)
		}

		err = mappingRepo.AddMappingToUser(ctx, "test_user_1", mapping2)
		if err != nil {
			t.Fatalf("AddMappingToUser failed: %v", err)
		}

		mappings, err := mappingRepo.ListUserMappings(ctx, "test_user_1")
		if err != nil {
			t.Fatalf("ListUserMappings failed: %v", err)
		}

		if len(mappings) < 2 {
			t.Errorf("Expected at least 2 mappings, got %d", len(mappings))
		}

		found := false
		for _, m := range mappings {
			if m.ID == mapping1.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find mapping in list")
		}
	})

	t.Run("ListClientMappings and AddMappingToClient", func(t *testing.T) {
		mapping1 := &cloud.PortMapping{
			ID:             "client_mapping_1",
			UserID:         "test_user_1",
			SourceClientID: "test_client_1",
			TargetClientID: "test_client_2",
			Protocol:       cloud.ProtocolTCP,
			SourcePort:     8086,
			TargetHost:     "localhost",
			TargetPort:     86,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		mapping2 := &cloud.PortMapping{
			ID:             "client_mapping_2",
			UserID:         "test_user_1",
			SourceClientID: "test_client_1",
			TargetClientID: "test_client_3",
			Protocol:       cloud.ProtocolHTTP,
			SourcePort:     8087,
			TargetHost:     "localhost",
			TargetPort:     87,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		err := mappingRepo.AddMappingToClient(ctx, "test_client_1", mapping1)
		if err != nil {
			t.Fatalf("AddMappingToClient failed: %v", err)
		}

		err = mappingRepo.AddMappingToClient(ctx, "test_client_1", mapping2)
		if err != nil {
			t.Fatalf("AddMappingToClient failed: %v", err)
		}

		mappings, err := mappingRepo.ListClientMappings(ctx, "test_client_1")
		if err != nil {
			t.Fatalf("ListClientMappings failed: %v", err)
		}

		if len(mappings) < 2 {
			t.Errorf("Expected at least 2 mappings, got %d", len(mappings))
		}

		found := false
		for _, m := range mappings {
			if m.ID == mapping1.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find mapping in list")
		}
	})
}

func TestNodeRepository(t *testing.T) {
	storage := cloud.NewMemoryStorage()
	defer storage.Close()

	repo := cloud.NewRepository(storage)
	nodeRepo := cloud.NewNodeRepository(repo)
	ctx := context.Background()

	t.Run("SaveNode and GetNode", func(t *testing.T) {
		node := &cloud.Node{
			ID:      "test_node_1",
			Name:    "Test Node 1",
			Address: "192.168.1.100:8080",
			Meta: map[string]string{
				"region": "us-west",
				"zone":   "us-west-1a",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := nodeRepo.SaveNode(ctx, node)
		if err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}

		retrieved, err := nodeRepo.GetNode(ctx, node.ID)
		if err != nil {
			t.Fatalf("GetNode failed: %v", err)
		}

		if retrieved.ID != node.ID {
			t.Errorf("Expected node ID %s, got %s", node.ID, retrieved.ID)
		}
		if retrieved.Name != node.Name {
			t.Errorf("Expected node name %s, got %s", node.Name, retrieved.Name)
		}
		if retrieved.Address != node.Address {
			t.Errorf("Expected node address %s, got %s", node.Address, retrieved.Address)
		}
		if retrieved.Meta["region"] != node.Meta["region"] {
			t.Errorf("Expected region %s, got %s", node.Meta["region"], retrieved.Meta["region"])
		}
	})

	t.Run("DeleteNode", func(t *testing.T) {
		node := &cloud.Node{
			ID:        "test_node_2",
			Name:      "Test Node 2",
			Address:   "192.168.1.101:8080",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := nodeRepo.SaveNode(ctx, node)
		if err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}

		err = nodeRepo.DeleteNode(ctx, node.ID)
		if err != nil {
			t.Fatalf("DeleteNode failed: %v", err)
		}

		_, err = nodeRepo.GetNode(ctx, node.ID)
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("ListNodes and AddNodeToList", func(t *testing.T) {
		node1 := &cloud.Node{
			ID:        "list_node_1",
			Name:      "List Node 1",
			Address:   "192.168.1.102:8080",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		node2 := &cloud.Node{
			ID:        "list_node_2",
			Name:      "List Node 2",
			Address:   "192.168.1.103:8080",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := nodeRepo.AddNodeToList(ctx, node1)
		if err != nil {
			t.Fatalf("AddNodeToList failed: %v", err)
		}

		err = nodeRepo.AddNodeToList(ctx, node2)
		if err != nil {
			t.Fatalf("AddNodeToList failed: %v", err)
		}

		nodes, err := nodeRepo.ListNodes(ctx)
		if err != nil {
			t.Fatalf("ListNodes failed: %v", err)
		}

		if len(nodes) < 2 {
			t.Errorf("Expected at least 2 nodes, got %d", len(nodes))
		}

		found := false
		for _, n := range nodes {
			if n.ID == node1.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find node in list")
		}
	})
}

func TestRepository_KeyPrefixes(t *testing.T) {
	storage := cloud.NewMemoryStorage()
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
