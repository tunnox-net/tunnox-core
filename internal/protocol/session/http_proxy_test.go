package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/protocol/httptypes"
)

func TestHTTPProxyManager_RegisterAndUnregister(t *testing.T) {
	manager := NewHTTPProxyManager()

	// 注册请求
	requestID := "test-request-123"
	ch := manager.RegisterPendingRequest(requestID)
	assert.NotNil(t, ch)

	// 通过发送响应来验证请求已注册
	resp := &httptypes.HTTPProxyResponse{
		RequestID:  requestID,
		StatusCode: 200,
	}
	go manager.HandleResponse(resp)

	// 应该能收到响应
	select {
	case received := <-ch:
		assert.Equal(t, requestID, received.RequestID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response, request may not be registered")
	}

	// 注销请求
	manager.UnregisterPendingRequest(requestID)

	// 验证请求已注销 - 重新注册应该得到新的 channel
	ch2 := manager.RegisterPendingRequest(requestID)
	assert.NotNil(t, ch2)
	// 清理
	manager.UnregisterPendingRequest(requestID)
}

func TestHTTPProxyManager_HandleResponse(t *testing.T) {
	manager := NewHTTPProxyManager()

	// 注册请求
	requestID := "test-request-456"
	ch := manager.RegisterPendingRequest(requestID)
	defer manager.UnregisterPendingRequest(requestID)

	// 发送响应
	resp := &httptypes.HTTPProxyResponse{
		RequestID:  requestID,
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"status":"ok"}`),
	}

	go manager.HandleResponse(resp)

	// 等待响应
	select {
	case received := <-ch:
		assert.Equal(t, requestID, received.RequestID)
		assert.Equal(t, 200, received.StatusCode)
		assert.Equal(t, "application/json", received.Headers["Content-Type"])
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestHTTPProxyManager_HandleResponseNoRequest(t *testing.T) {
	manager := NewHTTPProxyManager()

	// 发送响应（没有注册请求）
	resp := &httptypes.HTTPProxyResponse{
		RequestID:  "non-existent-request",
		StatusCode: 200,
	}

	// 不应该 panic
	manager.HandleResponse(resp)
}

func TestHTTPProxyManager_WaitForResponse_Success(t *testing.T) {
	manager := NewHTTPProxyManager()
	ctx := context.Background()

	requestID := "test-request-789"

	// 在后台发送响应
	go func() {
		time.Sleep(50 * time.Millisecond)
		resp := &httptypes.HTTPProxyResponse{
			RequestID:  requestID,
			StatusCode: 201,
		}
		manager.HandleResponse(resp)
	}()

	// 等待响应
	resp, err := manager.WaitForResponse(ctx, requestID, time.Second)
	require.NoError(t, err)
	assert.Equal(t, requestID, resp.RequestID)
	assert.Equal(t, 201, resp.StatusCode)
}

func TestHTTPProxyManager_WaitForResponse_Timeout(t *testing.T) {
	manager := NewHTTPProxyManager()
	ctx := context.Background()

	requestID := "test-request-timeout"

	// 等待响应（不发送响应）
	resp, err := manager.WaitForResponse(ctx, requestID, 100*time.Millisecond)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "timeout")
}

func TestHTTPProxyManager_WaitForResponse_ContextCancelled(t *testing.T) {
	manager := NewHTTPProxyManager()
	ctx, cancel := context.WithCancel(context.Background())

	requestID := "test-request-cancel"

	// 在后台取消上下文
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// 等待响应
	resp, err := manager.WaitForResponse(ctx, requestID, time.Second)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetHTTPProxyManager_Singleton(t *testing.T) {
	// 获取两次，应该是同一个实例
	manager1 := getHTTPProxyManager()
	manager2 := getHTTPProxyManager()

	assert.Same(t, manager1, manager2)
}
