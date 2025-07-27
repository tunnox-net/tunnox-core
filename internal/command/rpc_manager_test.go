package command

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewRPCManager(t *testing.T) {
	rm := NewRPCManager(context.Background())

	if rm == nil {
		t.Fatal("NewRPCManager returned nil")
	}

	if rm.pendingRequests == nil {
		t.Error("pendingRequests map should be initialized")
	}

	if rm.timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", rm.timeout)
	}
}

func TestRPCManager_RegisterAndGetRequest(t *testing.T) {
	rm := NewRPCManager(context.Background())
	requestID := "test-request-123"
	responseChan := make(chan *CommandResponse, 1)

	// 注册请求
	rm.RegisterRequest(requestID, responseChan)

	// 获取请求
	retrievedChan, exists := rm.GetRequest(requestID)
	if !exists {
		t.Error("Request should exist after registration")
	}

	if retrievedChan != responseChan {
		t.Error("Retrieved channel should be the same as registered channel")
	}

	// 测试不存在的请求
	_, exists = rm.GetRequest("non-existent")
	if exists {
		t.Error("Non-existent request should not exist")
	}
}

func TestRPCManager_UnregisterRequest(t *testing.T) {
	rm := NewRPCManager(context.Background())
	requestID := "test-request-456"
	responseChan := make(chan *CommandResponse, 1)

	// 注册请求
	rm.RegisterRequest(requestID, responseChan)

	// 验证请求存在
	_, exists := rm.GetRequest(requestID)
	if !exists {
		t.Error("Request should exist after registration")
	}

	// 注销请求
	rm.UnregisterRequest(requestID)

	// 验证请求不存在
	_, exists = rm.GetRequest(requestID)
	if exists {
		t.Error("Request should not exist after unregistration")
	}

	// 测试注销不存在的请求（应该不会panic）
	rm.UnregisterRequest("non-existent")
}

func TestRPCManager_SetAndGetTimeout(t *testing.T) {
	rm := NewRPCManager(context.Background())

	// 测试默认超时
	defaultTimeout := rm.GetTimeout()
	if defaultTimeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", defaultTimeout)
	}

	// 设置新超时
	newTimeout := 60 * time.Second
	rm.SetTimeout(newTimeout)

	// 验证新超时
	retrievedTimeout := rm.GetTimeout()
	if retrievedTimeout != newTimeout {
		t.Errorf("Expected timeout %v, got %v", newTimeout, retrievedTimeout)
	}
}

func TestRPCManager_GetPendingRequestCount(t *testing.T) {
	rm := NewRPCManager(context.Background())

	// 初始数量应该为0
	count := rm.GetPendingRequestCount()
	if count != 0 {
		t.Errorf("Expected initial count 0, got %d", count)
	}

	// 注册一些请求
	requestIDs := []string{"req1", "req2", "req3"}
	for _, id := range requestIDs {
		responseChan := make(chan *CommandResponse, 1)
		rm.RegisterRequest(id, responseChan)
	}

	// 验证数量
	count = rm.GetPendingRequestCount()
	if count != len(requestIDs) {
		t.Errorf("Expected count %d, got %d", len(requestIDs), count)
	}

	// 注销一个请求
	rm.UnregisterRequest("req1")

	// 验证数量
	count = rm.GetPendingRequestCount()
	if count != len(requestIDs)-1 {
		t.Errorf("Expected count %d, got %d", len(requestIDs)-1, count)
	}
}

func TestRPCManager_ConcurrentAccess(t *testing.T) {
	rm := NewRPCManager(context.Background())
	done := make(chan bool, 10)

	// 并发注册和注销请求
	for i := 0; i < 10; i++ {
		go func(id int) {
			requestID := fmt.Sprintf("concurrent-req-%d", id)
			responseChan := make(chan *CommandResponse, 1)

			// 注册请求
			rm.RegisterRequest(requestID, responseChan)

			// 验证注册成功
			_, exists := rm.GetRequest(requestID)
			if !exists {
				t.Errorf("Request %s should exist", requestID)
			}

			// 注销请求
			rm.UnregisterRequest(requestID)

			// 验证注销成功
			_, exists = rm.GetRequest(requestID)
			if exists {
				t.Errorf("Request %s should not exist after unregistration", requestID)
			}

			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证最终数量为0
	count := rm.GetPendingRequestCount()
	if count != 0 {
		t.Errorf("Expected final count 0, got %d", count)
	}
}

func TestRPCManager_TimeoutHandling(t *testing.T) {
	rm := NewRPCManager(context.Background())

	// 设置较短的超时时间用于测试
	rm.SetTimeout(100 * time.Millisecond)

	requestID := "timeout-test"
	responseChan := make(chan *CommandResponse, 1)

	// 注册请求
	rm.RegisterRequest(requestID, responseChan)

	// 等待超时
	time.Sleep(200 * time.Millisecond)

	// 验证请求仍然存在（因为没有自动清理机制）
	_, exists := rm.GetRequest(requestID)
	if !exists {
		t.Error("Request should still exist since there's no automatic cleanup")
	}

	// 验证数量为1
	count := rm.GetPendingRequestCount()
	if count != 1 {
		t.Errorf("Expected count 1 after timeout, got %d", count)
	}

	// 手动清理
	rm.UnregisterRequest(requestID)

	// 验证清理后数量为0
	count = rm.GetPendingRequestCount()
	if count != 0 {
		t.Errorf("Expected count 0 after manual cleanup, got %d", count)
	}
}

func TestRPCManager_ResponseChannelCommunication(t *testing.T) {
	rm := NewRPCManager(context.Background())
	requestID := "response-test"
	responseChan := make(chan *CommandResponse, 1)

	// 注册请求
	rm.RegisterRequest(requestID, responseChan)

	// 模拟发送响应
	expectedResponse := &CommandResponse{
		Success: true,
		Data:    "test response",
	}

	// 在goroutine中发送响应
	go func() {
		time.Sleep(10 * time.Millisecond)
		responseChan <- expectedResponse
	}()

	// 等待接收响应
	select {
	case response := <-responseChan:
		if response != expectedResponse {
			t.Error("Received response should match expected response")
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for response")
	}

	// 清理
	rm.UnregisterRequest(requestID)
}
