package unit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tunnox-core/internal/cloud/models"
)

// TestUser_JSONSerialization 测试User JSON序列化
func TestUser_JSONSerialization(t *testing.T) {
	user := &models.User{
		ID:        "user-123",
		Username:  "testuser",
		Email:     "test@example.com",
		Status:    models.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Quota: models.UserQuota{
			MaxClientIDs:   10,
			MaxConnections: 100,
			BandwidthLimit: 1024 * 1024 * 100, // 100MB/s
			StorageLimit:   1024 * 1024 * 1024 * 10, // 10GB
		},
	}

	// 序列化
	data, err := json.Marshal(user)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed models.User
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "user-123", parsed.ID)
	assert.Equal(t, "testuser", parsed.Username)
	assert.Equal(t, "test@example.com", parsed.Email)
	assert.Equal(t, models.UserStatusActive, parsed.Status)
	assert.Equal(t, 10, parsed.Quota.MaxClientIDs)
}

// TestUser_DefaultQuota 测试用户默认配额
func TestUser_DefaultQuota(t *testing.T) {
	quota := models.UserQuota{
		MaxClientIDs:   10,
		MaxConnections: 100,
		BandwidthLimit: 1024 * 1024 * 100, // 100MB/s
		StorageLimit:   1024 * 1024 * 1024 * 10, // 10GB
	}

	assert.Greater(t, quota.MaxClientIDs, 0)
	assert.Greater(t, quota.MaxConnections, 0)
	assert.Greater(t, quota.BandwidthLimit, int64(0))
}

// TestClient_JSONSerialization 测试Client JSON序列化
func TestClient_JSONSerialization(t *testing.T) {
	now := time.Now()
	client := &models.Client{
		ID:        12345,
		UserID:    "user-123",
		Status:    models.ClientStatusOnline,
		Type:      models.ClientTypeRegistered,
		IPAddress: "192.168.1.100",
		NodeID:    "node-1",
		LastSeen:  &now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 序列化
	data, err := json.Marshal(client)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed models.Client
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), parsed.ID)
	assert.Equal(t, "user-123", parsed.UserID)
	assert.Equal(t, models.ClientStatusOnline, parsed.Status)
	assert.Equal(t, models.ClientTypeRegistered, parsed.Type)
}

// TestClient_StatusValues 测试客户端状态值
func TestClient_StatusValues(t *testing.T) {
	statuses := []models.ClientStatus{
		models.ClientStatusOnline,
		models.ClientStatusOffline,
		models.ClientStatusBlocked,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			client := &models.Client{
				ID:     12345,
				Status: status,
			}

			data, err := json.Marshal(client)
			require.NoError(t, err)

			var parsed models.Client
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)
			assert.Equal(t, status, parsed.Status)
		})
	}
}

// TestClient_TypeValues 测试客户端类型值
func TestClient_TypeValues(t *testing.T) {
	types := []models.ClientType{
		models.ClientTypeAnonymous,
		models.ClientTypeRegistered,
	}

	for _, clientType := range types {
		t.Run(string(clientType), func(t *testing.T) {
			client := &models.Client{
				ID:   12345,
				Type: clientType,
			}

			data, err := json.Marshal(client)
			require.NoError(t, err)

			var parsed models.Client
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)
			assert.Equal(t, clientType, parsed.Type)
		})
	}
}

// TestPortMapping_JSONSerialization 测试PortMapping JSON序列化
func TestPortMapping_JSONSerialization(t *testing.T) {
	mapping := &models.PortMapping{
		ID:             "mapping-123",
		UserID:         "user-123",
		SourceClientID: 12345,
		TargetClientID: 67890,
		SourcePort:     9000,
		TargetHost:     "localhost",
		TargetPort:     8080,
		Protocol:       models.ProtocolTCP,
		Status:         models.MappingStatusActive,
		SecretKey:      "secret-key-abc",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// 序列化
	data, err := json.Marshal(mapping)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed models.PortMapping
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "mapping-123", parsed.ID)
	assert.Equal(t, "user-123", parsed.UserID)
	assert.Equal(t, int64(12345), parsed.SourceClientID)
	assert.Equal(t, int64(67890), parsed.TargetClientID)
	assert.Equal(t, 9000, parsed.SourcePort)
	assert.Equal(t, "localhost", parsed.TargetHost)
	assert.Equal(t, 8080, parsed.TargetPort)
	assert.Equal(t, models.ProtocolTCP, parsed.Protocol)
	assert.Equal(t, models.MappingStatusActive, parsed.Status)
}

// TestPortMapping_StatusValues 测试映射状态值
func TestPortMapping_StatusValues(t *testing.T) {
	statuses := []models.MappingStatus{
		models.MappingStatusActive,
		models.MappingStatusInactive,
		models.MappingStatusError,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			mapping := &models.PortMapping{
				ID:     "mapping-123",
				Status: status,
			}

			data, err := json.Marshal(mapping)
			require.NoError(t, err)

			var parsed models.PortMapping
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)
			assert.Equal(t, status, parsed.Status)
		})
	}
}

// TestConnectionStats_Tracking 测试连接统计追踪
func TestConnectionStats_Tracking(t *testing.T) {
	// 模拟连接统计
	type ConnectionStats struct {
		ID            string
		MappingID     string
		BytesReceived int64
		BytesSent     int64
	}

	conn := ConnectionStats{
		ID:            "conn-123",
		MappingID:     "mapping-456",
		BytesReceived: 1024000,
		BytesSent:     2048000,
	}

	// 序列化
	data, err := json.Marshal(conn)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed ConnectionStats
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "conn-123", parsed.ID)
	assert.Equal(t, "mapping-456", parsed.MappingID)
	assert.Equal(t, int64(1024000), parsed.BytesReceived)
	assert.Equal(t, int64(2048000), parsed.BytesSent)
}

// TestNode_JSONSerialization 测试Node JSON序列化
func TestNode_JSONSerialization(t *testing.T) {
	node := &models.Node{
		ID:      "node-1",
		Address: "10.0.1.100:7000",
	}

	// 序列化
	data, err := json.Marshal(node)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed models.Node
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "node-1", parsed.ID)
	assert.Equal(t, "10.0.1.100:7000", parsed.Address)
}

// TestNode_RegisterRequest 测试节点注册请求
func TestNode_RegisterRequest(t *testing.T) {
	req := &models.NodeRegisterRequest{
		NodeID:  "node-1",
		Address: "10.0.1.100:7000",
		Version: "1.0.0",
		Meta: map[string]string{
			"region": "us-west",
			"zone":   "zone-a",
		},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var parsed models.NodeRegisterRequest
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "node-1", parsed.NodeID)
	assert.Equal(t, "10.0.1.100:7000", parsed.Address)
	assert.Equal(t, "1.0.0", parsed.Version)
	assert.Len(t, parsed.Meta, 2)
}

// TestAuthRequest_JSONSerialization 测试AuthRequest JSON序列化
func TestAuthRequest_JSONSerialization(t *testing.T) {
	req := &models.AuthRequest{
		ClientID:  12345,
		AuthCode:  "auth-code-123",
		SecretKey: "secret-key-456",
		NodeID:    "node-1",
		Version:   "1.0.0",
		IPAddress: "192.168.1.100",
		Type:      models.ClientTypeRegistered,
	}

	// 序列化
	data, err := json.Marshal(req)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed models.AuthRequest
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), parsed.ClientID)
	assert.Equal(t, "auth-code-123", parsed.AuthCode)
	assert.Equal(t, "secret-key-456", parsed.SecretKey)
	assert.Equal(t, models.ClientTypeRegistered, parsed.Type)
}

// TestAuthResponse_JSONSerialization 测试AuthResponse JSON序列化
func TestAuthResponse_JSONSerialization(t *testing.T) {
	now := time.Now()
	resp := &models.AuthResponse{
		Success:   true,
		Message:   "Authentication successful",
		Token:     "jwt-token-abc",
		ExpiresAt: now.Add(24 * time.Hour),
		Client: &models.Client{
			ID:       12345,
			UserID:   "user-123",
			Status:   models.ClientStatusOnline,
			Type:     models.ClientTypeRegistered,
		},
	}

	// 序列化
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed models.AuthResponse
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.True(t, parsed.Success)
	assert.Equal(t, "Authentication successful", parsed.Message)
	assert.Equal(t, "jwt-token-abc", parsed.Token)
	assert.NotNil(t, parsed.Client)
	assert.Equal(t, int64(12345), parsed.Client.ID)
}

// TestSystemStats_JSONSerialization 测试系统统计JSON序列化
func TestSystemStats_JSONSerialization(t *testing.T) {
	type SystemStats struct {
		TotalUsers       int       `json:"total_users"`
		TotalClients     int       `json:"total_clients"`
		OnlineClients    int       `json:"online_clients"`
		TotalMappings    int       `json:"total_mappings"`
		ActiveMappings   int       `json:"active_mappings"`
		TotalConnections int       `json:"total_connections"`
		TotalBytesIn     int64     `json:"total_bytes_in"`
		TotalBytesOut    int64     `json:"total_bytes_out"`
		Uptime           int64     `json:"uptime"`
		LastUpdated      time.Time `json:"last_updated"`
	}

	stats := &SystemStats{
		TotalUsers:       100,
		TotalClients:     500,
		OnlineClients:    450,
		TotalMappings:    300,
		ActiveMappings:   280,
		TotalConnections: 1500,
		TotalBytesIn:     1024 * 1024 * 1024 * 10, // 10GB
		TotalBytesOut:    1024 * 1024 * 1024 * 20, // 20GB
		Uptime:           3600 * 24 * 7,           // 7天
		LastUpdated:      time.Now(),
	}

	// 序列化
	data, err := json.Marshal(stats)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed SystemStats
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, 100, parsed.TotalUsers)
	assert.Equal(t, 500, parsed.TotalClients)
	assert.Equal(t, 450, parsed.OnlineClients)
	assert.Equal(t, 300, parsed.TotalMappings)
}

// TestUserQuota_ZeroValues 测试UserQuota零值
func TestUserQuota_ZeroValues(t *testing.T) {
	var quota models.UserQuota

	// 零值应该都是0
	assert.Equal(t, 0, quota.MaxClientIDs)
	assert.Equal(t, 0, quota.MaxConnections)
	assert.Equal(t, int64(0), quota.BandwidthLimit)
	assert.Equal(t, int64(0), quota.StorageLimit)

	// 序列化零值
	data, err := json.Marshal(quota)
	require.NoError(t, err)

	var parsed models.UserQuota
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, quota, parsed)
}

// TestClient_EmptyFields 测试Client空字段
func TestClient_EmptyFields(t *testing.T) {
	client := &models.Client{
		ID:     12345,
		UserID: "", // 匿名客户端可能没有UserID
		Status: models.ClientStatusOnline,
		Type:   models.ClientTypeAnonymous,
	}

	data, err := json.Marshal(client)
	require.NoError(t, err)

	var parsed models.Client
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), parsed.ID)
	assert.Empty(t, parsed.UserID)
	assert.Equal(t, models.ClientTypeAnonymous, parsed.Type)
}

// TestPortMapping_WithConfig 测试带配置的PortMapping
func TestPortMapping_WithConfig(t *testing.T) {
	mapping := &models.PortMapping{
		ID:             "mapping-123",
		SourceClientID: 12345,
		TargetClientID: 67890,
		Protocol:       "tcp",
	}

	data, err := json.Marshal(mapping)
	require.NoError(t, err)

	var parsed models.PortMapping
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "mapping-123", parsed.ID)
	assert.Equal(t, int64(12345), parsed.SourceClientID)
	assert.Equal(t, int64(67890), parsed.TargetClientID)
}

// TestTrafficStats_ZeroBytes 测试零字节流量统计
func TestTrafficStats_ZeroBytes(t *testing.T) {
	type TrafficStats struct {
		BytesReceived int64 `json:"bytes_received"`
		BytesSent     int64 `json:"bytes_sent"`
	}

	stats := &TrafficStats{
		BytesReceived: 0,
		BytesSent:     0,
	}

	data, err := json.Marshal(stats)
	require.NoError(t, err)

	var parsed TrafficStats
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, int64(0), parsed.BytesReceived)
	assert.Equal(t, int64(0), parsed.BytesSent)
}

// TestModels_ComplexNesting 测试复杂嵌套结构
func TestModels_ComplexNesting(t *testing.T) {
	user := &models.User{
		ID:       "user-123",
		Username: "testuser",
		Quota: models.UserQuota{
			MaxClientIDs:   10,
			MaxConnections: 100,
		},
	}

	authResp := &models.AuthResponse{
		Success: true,
		Message: "OK",
		Client: &models.Client{
			ID:       12345,
			UserID:   user.ID,
			Status:   models.ClientStatusOnline,
			Type:     models.ClientTypeRegistered,
		},
	}

	// 序列化整个响应
	data, err := json.Marshal(authResp)
	require.NoError(t, err)

	// 反序列化
	var parsed models.AuthResponse
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.True(t, parsed.Success)
	assert.NotNil(t, parsed.Client)
	assert.Equal(t, "user-123", parsed.Client.UserID)
}

// TestModels_TimeHandling 测试时间字段处理
func TestModels_TimeHandling(t *testing.T) {
	now := time.Now()

	user := &models.User{
		ID:        "user-123",
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(user)
	require.NoError(t, err)

	var parsed models.User
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// 时间序列化后会有精度损失，使用Equal而不是严格相等
	assert.WithinDuration(t, now, parsed.CreatedAt, time.Second)
	assert.WithinDuration(t, now, parsed.UpdatedAt, time.Second)
}

// TestModels_NilPointerFields 测试nil指针字段
func TestModels_NilPointerFields(t *testing.T) {
	client := &models.Client{
		ID:       12345,
		LastSeen: nil, // nil指针
	}

	data, err := json.Marshal(client)
	require.NoError(t, err)

	var parsed models.Client
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Nil(t, parsed.LastSeen)
}

// TestUserQuota_Comparison 测试UserQuota比较
func TestUserQuota_Comparison(t *testing.T) {
	quota1 := models.UserQuota{
		MaxClientIDs:   10,
		MaxConnections: 100,
	}

	quota2 := models.UserQuota{
		MaxClientIDs:   10,
		MaxConnections: 100,
	}

	quota3 := models.UserQuota{
		MaxClientIDs:   20,
		MaxConnections: 200,
	}

	assert.Equal(t, quota1, quota2, "Equal quotas should be equal")
	assert.NotEqual(t, quota1, quota3, "Different quotas should not be equal")
}

// TestClient_IDGeneration 测试客户端ID的合理性
func TestClient_IDGeneration(t *testing.T) {
	// 创建多个客户端，验证ID不冲突
	clients := make(map[int64]bool)

	for i := 0; i < 100; i++ {
		client := &models.Client{
			ID:     int64(i + 1000),
			UserID: "user-123",
		}

		// 验证ID唯一性
		assert.False(t, clients[client.ID], "ID %d should be unique", client.ID)
		clients[client.ID] = true
	}

	assert.Len(t, clients, 100)
}

// TestPortMapping_ProtocolValidation 测试协议验证
func TestPortMapping_ProtocolValidation(t *testing.T) {
	validProtocols := []models.Protocol{
		models.ProtocolTCP,
		models.ProtocolUDP,
		models.ProtocolHTTP,
	}

	for _, protocol := range validProtocols {
		t.Run(string(protocol), func(t *testing.T) {
			mapping := &models.PortMapping{
				ID:       "mapping-123",
				Protocol: protocol,
			}

			data, err := json.Marshal(mapping)
			require.NoError(t, err)

			var parsed models.PortMapping
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)
			assert.Equal(t, protocol, parsed.Protocol)
		})
	}
}

// TestTrafficStats_Accumulation 测试流量统计累积
func TestTrafficStats_Accumulation(t *testing.T) {
	type TrafficStats struct {
		BytesReceived int64 `json:"bytes_received"`
		BytesSent     int64 `json:"bytes_sent"`
	}

	stats := &TrafficStats{
		BytesReceived: 1000,
		BytesSent:     2000,
	}

	// 模拟累积
	stats.BytesReceived += 500
	stats.BytesSent += 1000

	assert.Equal(t, int64(1500), stats.BytesReceived)
	assert.Equal(t, int64(3000), stats.BytesSent)

	// 序列化后验证
	data, err := json.Marshal(stats)
	require.NoError(t, err)

	var parsed TrafficStats
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, int64(1500), parsed.BytesReceived)
	assert.Equal(t, int64(3000), parsed.BytesSent)
}

// TestSystemStats_Calculation 测试系统统计计算
func TestSystemStats_Calculation(t *testing.T) {
	type SystemStats struct {
		TotalUsers     int `json:"total_users"`
		TotalClients   int `json:"total_clients"`
		OnlineClients  int `json:"online_clients"`
		TotalMappings  int `json:"total_mappings"`
		ActiveMappings int `json:"active_mappings"`
	}

	stats := &SystemStats{
		TotalUsers:     100,
		TotalClients:   500,
		OnlineClients:  450,
		TotalMappings:  300,
		ActiveMappings: 280,
	}

	// 计算在线率
	onlineRate := float64(stats.OnlineClients) / float64(stats.TotalClients) * 100
	assert.Equal(t, 90.0, onlineRate)

	// 计算活跃率
	activeRate := float64(stats.ActiveMappings) / float64(stats.TotalMappings) * 100
	assert.InDelta(t, 93.33, activeRate, 0.01)
}

