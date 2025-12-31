package command

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"tunnox-core/internal/packet"
)

func TestNewResponseManager(t *testing.T) {
	manager := NewResponseManager()
	if manager == nil {
		t.Fatal("NewResponseManager returned nil")
	}

	if manager.pendingRequests == nil {
		t.Error("pendingRequests map should not be nil")
	}

	if manager.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", manager.timeout, 30*time.Second)
	}
}

func TestResponseManager_RegisterRequest(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	if ch == nil {
		t.Fatal("RegisterRequest returned nil channel")
	}

	// 验证已注册
	manager.mu.RLock()
	_, exists := manager.pendingRequests[commandID]
	manager.mu.RUnlock()

	if !exists {
		t.Error("Request not found in pendingRequests")
	}
}

func TestResponseManager_UnregisterRequest(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	manager.RegisterRequest(commandID)

	// 取消注册
	manager.UnregisterRequest(commandID)

	// 验证已取消注册
	manager.mu.RLock()
	_, exists := manager.pendingRequests[commandID]
	manager.mu.RUnlock()

	if exists {
		t.Error("Request should be removed from pendingRequests")
	}
}

func TestResponseManager_UnregisterRequest_NotExists(t *testing.T) {
	manager := NewResponseManager()

	// 取消注册不存在的请求（不应该 panic）
	manager.UnregisterRequest("non-existent")
}

func TestResponseManager_HandleResponse_Success(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	// 创建响应数据包
	responseData := map[string]interface{}{
		"success":    true,
		"data":       "test data",
		"command_id": commandID,
	}
	bodyBytes, _ := json.Marshal(responseData)

	pkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandId:   commandID,
			CommandBody: string(bodyBytes),
		},
	}

	// 处理响应
	handled := manager.HandleResponse(pkt)
	if !handled {
		t.Error("HandleResponse should return true")
	}

	// 验证响应已发送到通道
	select {
	case resp := <-ch:
		if resp == nil {
			t.Fatal("Response should not be nil")
		}
		if !resp.Success {
			t.Error("Response.Success should be true")
		}
		if resp.Data != "test data" {
			t.Errorf("Response.Data = %s, want 'test data'", resp.Data)
		}
	default:
		t.Error("Response not received on channel")
	}
}

func TestResponseManager_HandleResponse_WithObjectData(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	// 创建响应数据包（data 是对象）
	responseData := map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"key": "value",
		},
		"command_id": commandID,
	}
	bodyBytes, _ := json.Marshal(responseData)

	pkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandId:   commandID,
			CommandBody: string(bodyBytes),
		},
	}

	// 处理响应
	handled := manager.HandleResponse(pkt)
	if !handled {
		t.Error("HandleResponse should return true")
	}

	// 验证响应
	select {
	case resp := <-ch:
		if resp == nil {
			t.Fatal("Response should not be nil")
		}
		// data 应该被序列化为 JSON 字符串
		if resp.Data == "" {
			t.Error("Response.Data should not be empty")
		}
	default:
		t.Error("Response not received on channel")
	}
}

func TestResponseManager_HandleResponse_Error(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	// 创建错误响应数据包
	responseData := map[string]interface{}{
		"success":    false,
		"error":      "something went wrong",
		"command_id": commandID,
	}
	bodyBytes, _ := json.Marshal(responseData)

	pkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandId:   commandID,
			CommandBody: string(bodyBytes),
		},
	}

	// 处理响应
	handled := manager.HandleResponse(pkt)
	if !handled {
		t.Error("HandleResponse should return true")
	}

	// 验证响应
	select {
	case resp := <-ch:
		if resp == nil {
			t.Fatal("Response should not be nil")
		}
		if resp.Success {
			t.Error("Response.Success should be false")
		}
		if resp.Error != "something went wrong" {
			t.Errorf("Response.Error = %s, want 'something went wrong'", resp.Error)
		}
	default:
		t.Error("Response not received on channel")
	}
}

func TestResponseManager_HandleResponse_NotCommandResp(t *testing.T) {
	manager := NewResponseManager()

	// 创建非命令响应数据包
	pkt := &packet.TransferPacket{
		PacketType: packet.TunnelData,
	}

	// 处理响应
	handled := manager.HandleResponse(pkt)
	if handled {
		t.Error("HandleResponse should return false for non-command response")
	}
}

func TestResponseManager_HandleResponse_NoCommandPacket(t *testing.T) {
	manager := NewResponseManager()

	// 创建没有 CommandPacket 的数据包
	pkt := &packet.TransferPacket{
		PacketType:    packet.CommandResp,
		CommandPacket: nil,
	}

	// 处理响应
	handled := manager.HandleResponse(pkt)
	if handled {
		t.Error("HandleResponse should return false when CommandPacket is nil")
	}
}

func TestResponseManager_HandleResponse_EmptyCommandID(t *testing.T) {
	manager := NewResponseManager()

	// 创建空 CommandId 的数据包
	pkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandId: "",
		},
	}

	// 处理响应
	handled := manager.HandleResponse(pkt)
	if handled {
		t.Error("HandleResponse should return false for empty CommandId")
	}
}

func TestResponseManager_HandleResponse_NoPendingRequest(t *testing.T) {
	manager := NewResponseManager()

	// 创建没有对应等待请求的响应
	responseData := map[string]interface{}{
		"success":    true,
		"command_id": "unknown-command",
	}
	bodyBytes, _ := json.Marshal(responseData)

	pkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandId:   "unknown-command",
			CommandBody: string(bodyBytes),
		},
	}

	// 处理响应
	handled := manager.HandleResponse(pkt)
	if handled {
		t.Error("HandleResponse should return false for unknown command")
	}
}

func TestResponseManager_HandleResponse_InvalidJSON(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	// 创建无效 JSON 的数据包
	pkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandId:   commandID,
			CommandBody: "invalid json {{{",
		},
	}

	// 处理响应
	handled := manager.HandleResponse(pkt)
	if handled {
		t.Error("HandleResponse should return false for invalid JSON")
	}

	// 验证错误响应
	select {
	case resp := <-ch:
		if resp == nil {
			t.Fatal("Response should not be nil")
		}
		if resp.Success {
			t.Error("Response.Success should be false for invalid JSON")
		}
		if resp.Error == "" {
			t.Error("Response.Error should contain error message")
		}
	default:
		// 通道可能已经关闭
	}
}

func TestResponseManager_WaitForResponse_Success(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	// 在后台发送响应
	go func() {
		time.Sleep(10 * time.Millisecond)
		ch <- &Response{
			Success: true,
			Data:    "test data",
		}
	}()

	// 等待响应
	resp, err := manager.WaitForResponse(commandID, ch)
	if err != nil {
		t.Fatalf("WaitForResponse failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response should not be nil")
	}
	if !resp.Success {
		t.Error("Response.Success should be true")
	}
	if resp.Data != "test data" {
		t.Errorf("Response.Data = %s, want 'test data'", resp.Data)
	}
}

func TestResponseManager_WaitForResponse_ChannelClosed(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	// 关闭通道
	close(ch)

	// 等待响应
	_, err := manager.WaitForResponse(commandID, ch)
	if err == nil {
		t.Error("WaitForResponse should return error when channel is closed")
	}
}

func TestResponseManager_WaitForResponseWithContext_Success(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	ctx := context.Background()

	// 在后台发送响应
	go func() {
		time.Sleep(10 * time.Millisecond)
		ch <- &Response{
			Success: true,
			Data:    "test data",
		}
	}()

	// 等待响应
	resp, err := manager.WaitForResponseWithContext(ctx, commandID, ch)
	if err != nil {
		t.Fatalf("WaitForResponseWithContext failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response should not be nil")
	}
	if !resp.Success {
		t.Error("Response.Success should be true")
	}
}

func TestResponseManager_WaitForResponseWithContext_Cancelled(t *testing.T) {
	manager := NewResponseManager()

	commandID := "test-command-123"
	ch := manager.RegisterRequest(commandID)

	ctx, cancel := context.WithCancel(context.Background())

	// 在后台取消 context
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	// 等待响应
	_, err := manager.WaitForResponseWithContext(ctx, commandID, ch)
	if err == nil {
		t.Error("WaitForResponseWithContext should return error when context is cancelled")
	}
}

func TestResponseManager_Concurrent(t *testing.T) {
	manager := NewResponseManager()

	numRequests := 100
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			commandID := "command-" + itoa(id)
			ch := manager.RegisterRequest(commandID)

			// 发送响应
			go func() {
				time.Sleep(time.Duration(id%10) * time.Millisecond)
				select {
				case ch <- &Response{Success: true}:
				default:
				}
			}()

			// 等待响应（使用较短的超时）
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			manager.WaitForResponseWithContext(ctx, commandID, ch)
		}(i)
	}

	wg.Wait()
}

func TestResponse_Fields(t *testing.T) {
	rawData := map[string]interface{}{
		"key": "value",
	}

	resp := Response{
		Success:   true,
		Data:      "test data",
		Error:     "error message",
		CommandId: "command-123",
		RequestID: "request-456",
		RawData:   rawData,
	}

	if !resp.Success {
		t.Error("Success should be true")
	}
	if resp.Data != "test data" {
		t.Errorf("Data = %s, want 'test data'", resp.Data)
	}
	if resp.Error != "error message" {
		t.Errorf("Error = %s, want 'error message'", resp.Error)
	}
	if resp.CommandId != "command-123" {
		t.Errorf("CommandId = %s, want 'command-123'", resp.CommandId)
	}
	if resp.RequestID != "request-456" {
		t.Errorf("RequestID = %s, want 'request-456'", resp.RequestID)
	}
	if resp.RawData == nil {
		t.Error("RawData should not be nil")
	}
}

// itoa 简单的整数转字符串
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
