package api

import (
	"encoding/json"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
)

// TestClientListResponse_Marshal 测试客户端列表响应序列化
func TestClientListResponse_Marshal(t *testing.T) {
	response := ClientListResponse{
		Clients: []*models.Client{
			{ID: 1, Name: "test-client-1"},
			{ID: 2, Name: "test-client-2"},
		},
		Total: 2,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal ClientListResponse: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result["total"].(float64) != 2 {
		t.Errorf("Expected total=2, got %v", result["total"])
	}
}

// TestMappingListResponse_Marshal 测试映射列表响应序列化
func TestMappingListResponse_Marshal(t *testing.T) {
	response := MappingListResponse{
		Mappings: []*models.PortMapping{
			{ID: "mapping-1", Protocol: "tcp"},
		},
		Total: 1,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal MappingListResponse: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result["total"].(float64) != 1 {
		t.Errorf("Expected total=1, got %v", result["total"])
	}
}

// TestLoginResponse_Marshal 测试登录响应序列化
func TestLoginResponse_Marshal(t *testing.T) {
	now := time.Now()
	response := LoginResponse{
		Success:   true,
		Token:     "test-token-12345",
		ExpiresAt: now,
		Client:    &models.Client{ID: 1, Name: "test-client"},
		Message:   "Login successful",
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal LoginResponse: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result["success"].(bool) != true {
		t.Errorf("Expected success=true, got %v", result["success"])
	}

	if result["token"].(string) != "test-token-12345" {
		t.Errorf("Expected token='test-token-12345', got %v", result["token"])
	}
}

// TestRefreshTokenResponse_Marshal 测试刷新令牌响应序列化
func TestRefreshTokenResponse_Marshal(t *testing.T) {
	now := time.Now()
	response := RefreshTokenResponse{
		Success:   true,
		Token:     "new-token-67890",
		ExpiresAt: now,
		Message:   "Token refreshed",
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal RefreshTokenResponse: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result["token"].(string) != "new-token-67890" {
		t.Errorf("Expected token='new-token-67890', got %v", result["token"])
	}
}

// TestConnectionListResponse_Marshal 测试连接列表响应序列化
func TestConnectionListResponse_Marshal(t *testing.T) {
	response := ConnectionListResponse{
		Connections: []string{"conn-1", "conn-2"}, // 简化示例
		Total:       2,
		MappingID:   "mapping-123",
		ClientID:    456,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal ConnectionListResponse: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result["total"].(float64) != 2 {
		t.Errorf("Expected total=2, got %v", result["total"])
	}

	if result["mapping_id"].(string) != "mapping-123" {
		t.Errorf("Expected mapping_id='mapping-123', got %v", result["mapping_id"])
	}
}

// TestNodeListResponse_Marshal 测试节点列表响应序列化
func TestNodeListResponse_Marshal(t *testing.T) {
	response := NodeListResponse{
		Nodes: []*models.NodeServiceInfo{
			{NodeID: "node-1"},
			{NodeID: "node-2"},
		},
		Total: 2,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal NodeListResponse: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result["total"].(float64) != 2 {
		t.Errorf("Expected total=2, got %v", result["total"])
	}
}

// TestStatsResponse_Marshal 测试统计响应序列化
func TestStatsResponse_Marshal(t *testing.T) {
	response := StatsResponse{
		TimeRange: "24h",
		Data:      map[string]int{"connections": 100},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal StatsResponse: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result["time_range"].(string) != "24h" {
		t.Errorf("Expected time_range='24h', got %v", result["time_range"])
	}
}

