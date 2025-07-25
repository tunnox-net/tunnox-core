package command

import (
	"reflect"
	"testing"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

func TestHandlerTypeInfo(t *testing.T) {
	// 测试连接处理器
	connectHandler := NewConnectHandler()

	// 验证类型信息
	if connectHandler.GetCommandType() != packet.Connect {
		t.Errorf("Expected command type %v, got %v", packet.Connect, connectHandler.GetCommandType())
	}

	if connectHandler.GetDirection() != DirectionDuplex {
		t.Errorf("Expected direction %v, got %v", DirectionDuplex, connectHandler.GetDirection())
	}

	if connectHandler.GetCategory() != CategoryConnection {
		t.Errorf("Expected category %v, got %v", CategoryConnection, connectHandler.GetCategory())
	}

	// 验证请求和响应类型
	requestType := connectHandler.GetRequestType()
	if requestType == nil {
		t.Error("Expected request type to be non-nil for ConnectHandler")
	} else {
		expectedType := reflect.TypeOf(ConnectRequest{})
		if requestType != expectedType {
			t.Errorf("Expected request type %v, got %v", expectedType, requestType)
		}
	}

	responseType := connectHandler.GetResponseType()
	if responseType == nil {
		t.Error("Expected response type to be non-nil for ConnectHandler")
	} else {
		expectedType := reflect.TypeOf(ConnectResponse{})
		if responseType != expectedType {
			t.Errorf("Expected response type %v, got %v", expectedType, responseType)
		}
	}
}

func TestHeartbeatHandlerTypeInfo(t *testing.T) {
	// 测试心跳处理器
	heartbeatHandler := NewHeartbeatHandler()

	// 验证类型信息
	if heartbeatHandler.GetCommandType() != packet.HeartbeatCmd {
		t.Errorf("Expected command type %v, got %v", packet.HeartbeatCmd, heartbeatHandler.GetCommandType())
	}

	if heartbeatHandler.GetDirection() != DirectionOneway {
		t.Errorf("Expected direction %v, got %v", DirectionOneway, heartbeatHandler.GetDirection())
	}

	// 验证请求类型（应该有）和响应类型（应该为nil）
	requestType := heartbeatHandler.GetRequestType()
	if requestType == nil {
		t.Error("Expected request type to be non-nil for HeartbeatHandler")
	} else {
		expectedType := reflect.TypeOf(HeartbeatRequest{})
		if requestType != expectedType {
			t.Errorf("Expected request type %v, got %v", expectedType, requestType)
		}
	}

	responseType := heartbeatHandler.GetResponseType()
	if responseType != nil {
		t.Errorf("Expected response type to be nil for HeartbeatHandler, got %v", responseType)
	}
}

func TestDisconnectHandlerTypeInfo(t *testing.T) {
	// 测试断开连接处理器
	disconnectHandler := NewDisconnectHandlerV2()

	// 验证类型信息
	if disconnectHandler.GetCommandType() != packet.Disconnect {
		t.Errorf("Expected command type %v, got %v", packet.Disconnect, disconnectHandler.GetCommandType())
	}

	if disconnectHandler.GetDirection() != DirectionOneway {
		t.Errorf("Expected direction %v, got %v", DirectionOneway, disconnectHandler.GetDirection())
	}

	// 验证请求类型（应该有）和响应类型（应该为nil）
	requestType := disconnectHandler.GetRequestType()
	if requestType == nil {
		t.Error("Expected request type to be non-nil for DisconnectHandlerV2")
	} else {
		expectedType := reflect.TypeOf(DisconnectRequest{})
		if requestType != expectedType {
			t.Errorf("Expected request type %v, got %v", expectedType, requestType)
		}
	}

	responseType := disconnectHandler.GetResponseType()
	if responseType != nil {
		t.Errorf("Expected response type to be nil for DisconnectHandlerV2, got %v", responseType)
	}
}

func TestProcessCommandBody(t *testing.T) {
	// 测试命令体处理
	connectHandler := NewConnectHandler()

	// 测试有效的JSON
	validJSON := `{"client_id": 12345, "client_name": "test_client", "protocol": "tcp"}`
	data, err := ProcessCommandBody(connectHandler, validJSON)
	if err != nil {
		t.Errorf("Failed to process valid JSON: %v", err)
	}

	if data == nil {
		t.Error("Expected data to be non-nil")
	} else {
		// 验证解析后的数据类型
		request, ok := data.(*ConnectRequest)
		if !ok {
			t.Errorf("Expected *ConnectRequest, got %T", data)
		} else {
			if request.ClientID != 12345 {
				t.Errorf("Expected ClientID 12345, got %d", request.ClientID)
			}
			if request.ClientName != "test_client" {
				t.Errorf("Expected ClientName 'test_client', got '%s'", request.ClientName)
			}
		}
	}

	// 测试无效的JSON
	invalidJSON := `{"client_id": "invalid", "client_name": "test_client"}`
	_, err = ProcessCommandBody(connectHandler, invalidJSON)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestCreateTypedResponse(t *testing.T) {
	// 测试类型安全的响应创建
	connectHandler := NewConnectHandler()

	// 测试有效的响应数据
	responseData := ConnectResponse{
		Success:    true,
		SessionID:  "session_123",
		ServerTime: 1640995200,
	}

	response, err := CreateTypedResponse(connectHandler, responseData)
	if err != nil {
		t.Errorf("Failed to create typed response: %v", err)
	}

	if response == nil {
		t.Error("Expected response to be non-nil")
	} else {
		if !response.Success {
			t.Error("Expected response to be successful")
		}
		if response.Data == "" {
			t.Error("Expected response data to be non-empty")
		}
	}

	// 测试类型不匹配的响应数据
	wrongType := "wrong type"
	_, err = CreateTypedResponse(connectHandler, wrongType)
	if err == nil {
		t.Error("Expected error for type mismatch")
	}

	// 测试无响应体的处理器
	heartbeatHandler := NewHeartbeatHandler()
	response, err = CreateTypedResponse(heartbeatHandler, nil)
	if err != nil {
		t.Errorf("Failed to create response for handler without response body: %v", err)
	}

	if response == nil {
		t.Error("Expected response to be non-nil")
	} else {
		if !response.Success {
			t.Error("Expected response to be successful")
		}
		if response.Data != "" {
			t.Error("Expected response data to be empty for handler without response body")
		}
	}
}

func TestLegacyHandlerCompatibility(t *testing.T) {
	// 测试向后兼容性
	legacyHandler := NewDisconnectHandler()

	// 验证接口实现
	var handler types.CommandHandler = legacyHandler

	// 验证基本方法
	if handler.GetCommandType() != packet.Disconnect {
		t.Errorf("Expected command type %v, got %v", packet.Disconnect, handler.GetCommandType())
	}

	if handler.GetDirection() != DirectionOneway {
		t.Errorf("Expected direction %v, got %v", DirectionOneway, handler.GetDirection())
	}

	if handler.GetCategory() != CategoryConnection {
		t.Errorf("Expected category %v, got %v", CategoryConnection, handler.GetCategory())
	}

	// 验证新增的类型信息方法（应该返回nil）
	if handler.GetRequestType() != nil {
		t.Errorf("Expected request type to be nil for legacy handler, got %v", handler.GetRequestType())
	}

	if handler.GetResponseType() != nil {
		t.Errorf("Expected response type to be nil for legacy handler, got %v", handler.GetResponseType())
	}
}

// 基准测试：类型信息获取性能
func BenchmarkGetHandlerTypeInfo(b *testing.B) {
	connectHandler := NewConnectHandler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		connectHandler.GetCommandType()
		connectHandler.GetDirection()
		connectHandler.GetCategory()
		connectHandler.GetRequestType()
		connectHandler.GetResponseType()
	}
}

// 基准测试：命令体处理性能
func BenchmarkProcessCommandBody(b *testing.B) {
	connectHandler := NewConnectHandler()
	validJSON := `{"client_id": 12345, "client_name": "test_client", "protocol": "tcp"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ProcessCommandBody(connectHandler, validJSON)
		if err != nil {
			b.Fatalf("Failed to process command body: %v", err)
		}
	}
}
