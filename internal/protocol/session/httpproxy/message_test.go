// Package httpproxy 消息类型测试
package httpproxy

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// Message 测试
// ============================================================================

func TestMessage_JSONSerialization(t *testing.T) {
	msg := Message{
		RequestID: "req-001",
		ClientID:  12345,
		Request:   json.RawMessage(`{"method":"GET","url":"http://example.com"}`),
	}

	// 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded Message
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	// 验证字段
	if loaded.RequestID != msg.RequestID {
		t.Errorf("RequestID should be %s, got %s", msg.RequestID, loaded.RequestID)
	}

	if loaded.ClientID != msg.ClientID {
		t.Errorf("ClientID should be %d, got %d", msg.ClientID, loaded.ClientID)
	}

	if string(loaded.Request) != string(msg.Request) {
		t.Errorf("Request should be %s, got %s", string(msg.Request), string(loaded.Request))
	}
}

func TestMessage_EmptyRequest(t *testing.T) {
	msg := Message{
		RequestID: "req-empty",
		ClientID:  100,
		Request:   nil,
	}

	// 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded Message
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	// json.RawMessage 对于 null 值会变成 []byte("null")，而不是 nil
	if loaded.Request != nil && string(loaded.Request) != "null" {
		t.Errorf("Request should be nil or 'null', got %s", string(loaded.Request))
	}
}

func TestMessage_ComplexRequest(t *testing.T) {
	requestJSON := `{
		"method": "POST",
		"url": "http://api.example.com/users",
		"headers": {"Content-Type": "application/json"},
		"body": "eyJuYW1lIjoiam9obiJ9"
	}`

	msg := Message{
		RequestID: "req-complex",
		ClientID:  200,
		Request:   json.RawMessage(requestJSON),
	}

	// 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded Message
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	// 解析嵌套的 Request
	var request map[string]interface{}
	err = json.Unmarshal(loaded.Request, &request)
	if err != nil {
		t.Fatalf("Should be able to parse Request: %v", err)
	}

	if request["method"] != "POST" {
		t.Errorf("method should be POST, got %v", request["method"])
	}

	if request["url"] != "http://api.example.com/users" {
		t.Errorf("url should be 'http://api.example.com/users', got %v", request["url"])
	}
}

// ============================================================================
// ResponseMessage 测试
// ============================================================================

func TestResponseMessage_JSONSerialization(t *testing.T) {
	msg := ResponseMessage{
		RequestID: "resp-001",
		Response:  json.RawMessage(`{"status_code":200,"body":"OK"}`),
		Error:     "",
	}

	// 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded ResponseMessage
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	// 验证字段
	if loaded.RequestID != msg.RequestID {
		t.Errorf("RequestID should be %s, got %s", msg.RequestID, loaded.RequestID)
	}

	if string(loaded.Response) != string(msg.Response) {
		t.Errorf("Response should be %s, got %s", string(msg.Response), string(loaded.Response))
	}
}

func TestResponseMessage_WithError(t *testing.T) {
	msg := ResponseMessage{
		RequestID: "resp-error",
		Response:  nil,
		Error:     "connection refused",
	}

	// 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded ResponseMessage
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	if loaded.Error != "connection refused" {
		t.Errorf("Error should be 'connection refused', got %s", loaded.Error)
	}

	if loaded.Response != nil {
		t.Error("Response should be nil when there's an error")
	}
}

func TestResponseMessage_EmptyResponse(t *testing.T) {
	msg := ResponseMessage{
		RequestID: "resp-empty",
		Response:  nil,
		Error:     "",
	}

	// 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded ResponseMessage
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	if loaded.Response != nil {
		t.Error("Response should be nil")
	}
}

func TestResponseMessage_ComplexResponse(t *testing.T) {
	responseJSON := `{
		"status_code": 201,
		"headers": {"Location": "/users/123"},
		"body": "eyJpZCI6MTIzfQ=="
	}`

	msg := ResponseMessage{
		RequestID: "resp-complex",
		Response:  json.RawMessage(responseJSON),
		Error:     "",
	}

	// 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded ResponseMessage
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	// 解析嵌套的 Response
	var response map[string]interface{}
	err = json.Unmarshal(loaded.Response, &response)
	if err != nil {
		t.Fatalf("Should be able to parse Response: %v", err)
	}

	if response["status_code"] != float64(201) {
		t.Errorf("status_code should be 201, got %v", response["status_code"])
	}
}

// ============================================================================
// JSON 标签测试
// ============================================================================

func TestMessage_JSONTags(t *testing.T) {
	msg := Message{
		RequestID: "tag-test",
		ClientID:  999,
		Request:   json.RawMessage(`{}`),
	}

	data, _ := json.Marshal(msg)
	jsonStr := string(data)

	// 验证 JSON 字段名
	tests := []struct {
		expected string
	}{
		{`"request_id"`},
		{`"client_id"`},
		{`"request"`},
	}

	for _, tt := range tests {
		if !contains(jsonStr, tt.expected) {
			t.Errorf("JSON should contain %s, got %s", tt.expected, jsonStr)
		}
	}
}

func TestResponseMessage_JSONTags(t *testing.T) {
	msg := ResponseMessage{
		RequestID: "tag-test",
		Response:  json.RawMessage(`{}`),
		Error:     "test error",
	}

	data, _ := json.Marshal(msg)
	jsonStr := string(data)

	// 验证 JSON 字段名
	tests := []struct {
		expected string
	}{
		{`"request_id"`},
		{`"response"`},
		{`"error"`},
	}

	for _, tt := range tests {
		if !contains(jsonStr, tt.expected) {
			t.Errorf("JSON should contain %s, got %s", tt.expected, jsonStr)
		}
	}
}

func TestResponseMessage_OmitEmpty(t *testing.T) {
	msg := ResponseMessage{
		RequestID: "omit-test",
		Response:  nil,
		Error:     "",
	}

	data, _ := json.Marshal(msg)
	jsonStr := string(data)

	// 验证 omitempty 生效
	if contains(jsonStr, `"response":`) && contains(jsonStr, `"response":null`) {
		// response 可能是 null 或被省略
	}

	if contains(jsonStr, `"error":""`) {
		t.Error("empty error should be omitted")
	}
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestMessage_ZeroValues(t *testing.T) {
	var msg Message

	// 序列化零值
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded Message
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	if loaded.RequestID != "" {
		t.Error("RequestID should be empty")
	}

	if loaded.ClientID != 0 {
		t.Error("ClientID should be 0")
	}

	// json.RawMessage 对于 null 值会变成 []byte("null")，而不是 nil
	if loaded.Request != nil && string(loaded.Request) != "null" {
		t.Errorf("Request should be nil or 'null', got %s", string(loaded.Request))
	}
}

func TestResponseMessage_ZeroValues(t *testing.T) {
	var msg ResponseMessage

	// 序列化零值
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded ResponseMessage
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	if loaded.RequestID != "" {
		t.Error("RequestID should be empty")
	}

	if loaded.Response != nil {
		t.Error("Response should be nil")
	}

	if loaded.Error != "" {
		t.Error("Error should be empty")
	}
}

func TestMessage_LargeRequest(t *testing.T) {
	// 创建大型请求数据
	largeBody := make([]byte, 1024*1024) // 1MB
	for i := range largeBody {
		largeBody[i] = 'A'
	}

	requestData := map[string]interface{}{
		"method": "POST",
		"body":   string(largeBody),
	}
	requestJSON, _ := json.Marshal(requestData)

	msg := Message{
		RequestID: "large-request",
		ClientID:  100,
		Request:   json.RawMessage(requestJSON),
	}

	// 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded Message
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	if len(loaded.Request) != len(msg.Request) {
		t.Errorf("Request length should be %d, got %d", len(msg.Request), len(loaded.Request))
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
