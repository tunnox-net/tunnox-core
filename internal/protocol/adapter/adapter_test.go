package adapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
)

// mockReadWriteCloser 模拟读写关闭器
type mockReadWriteCloser struct {
	readData   []byte
	readIndex  int
	writeData  []byte
	closed     bool
	readErr    error
	writeErr   error
	closeErr   error
	persistent bool
	mu         sync.Mutex
}

func newMockReadWriteCloser() *mockReadWriteCloser {
	return &mockReadWriteCloser{
		readData:  make([]byte, 0),
		writeData: make([]byte, 0),
	}
}

func (m *mockReadWriteCloser) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.readIndex >= len(m.readData) {
		return 0, io.EOF
	}
	n := copy(p, m.readData[m.readIndex:])
	m.readIndex += n
	return n, nil
}

func (m *mockReadWriteCloser) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.writeData = append(m.writeData, p...)
	return len(p), nil
}

func (m *mockReadWriteCloser) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closeErr != nil {
		return m.closeErr
	}
	m.closed = true
	return nil
}

func (m *mockReadWriteCloser) IsPersistent() bool {
	return m.persistent
}

// mockProtocolAdapter 模拟协议适配器
type mockProtocolAdapter struct {
	BaseAdapter
	dialFunc   func(addr string) (io.ReadWriteCloser, error)
	listenFunc func(addr string) error
	acceptFunc func() (io.ReadWriteCloser, error)
	connType   string
}

func newMockProtocolAdapter(parentCtx context.Context) *mockProtocolAdapter {
	m := &mockProtocolAdapter{
		connType: "mock",
	}
	m.BaseAdapter = BaseAdapter{}
	m.SetName("mock")
	m.SetCtx(parentCtx, m.onClose)
	m.SetProtocolAdapter(m)
	return m
}

func (m *mockProtocolAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	if m.dialFunc != nil {
		return m.dialFunc(addr)
	}
	return newMockReadWriteCloser(), nil
}

func (m *mockProtocolAdapter) Listen(addr string) error {
	if m.listenFunc != nil {
		return m.listenFunc(addr)
	}
	return nil
}

func (m *mockProtocolAdapter) Accept() (io.ReadWriteCloser, error) {
	if m.acceptFunc != nil {
		return m.acceptFunc()
	}
	return newMockReadWriteCloser(), nil
}

func (m *mockProtocolAdapter) getConnectionType() string {
	return m.connType
}

func (m *mockProtocolAdapter) onClose() error {
	return m.BaseAdapter.onClose()
}

// TestBaseAdapterGettersSetters 测试基础的 getter/setter 方法
func TestBaseAdapterGettersSetters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)

	// 测试名称
	if adapter.Name() != "mock" {
		t.Errorf("Expected name 'mock', got '%s'", adapter.Name())
	}

	adapter.SetName("new-mock")
	if adapter.Name() != "new-mock" {
		t.Errorf("Expected name 'new-mock', got '%s'", adapter.Name())
	}

	// 测试地址
	testAddr := "localhost:8080"
	adapter.SetAddr(testAddr)

	if adapter.GetAddr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.GetAddr())
	}

	if adapter.Addr() != testAddr {
		t.Errorf("Expected Addr() '%s', got '%s'", testAddr, adapter.Addr())
	}
}

// TestBaseAdapterConnectTo 测试 ConnectTo 方法
func TestBaseAdapterConnectTo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)

	// 测试成功连接
	err := adapter.ConnectTo("localhost:8080")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// 测试已连接时再次连接
	err = adapter.ConnectTo("localhost:8081")
	if err == nil {
		t.Error("Expected error for already connected")
	}
	if !strings.Contains(err.Error(), "already connected") {
		t.Errorf("Expected error containing 'already connected', got: %v", err)
	}

	adapter.Close()
}

// TestBaseAdapterConnectToDialError 测试 ConnectTo 拨号错误
func TestBaseAdapterConnectToDialError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)
	adapter.dialFunc = func(addr string) (io.ReadWriteCloser, error) {
		return nil, errors.New("dial failed")
	}

	err := adapter.ConnectTo("localhost:8080")
	if err == nil {
		t.Error("Expected error")
	}
	if !strings.Contains(err.Error(), "failed to connect to mock server") {
		t.Errorf("Expected error containing 'failed to connect to mock server', got: %v", err)
	}

	adapter.Close()
}

// TestBaseAdapterConnectToNoProtocol 测试无协议适配器的 ConnectTo
func TestBaseAdapterConnectToNoProtocol(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &BaseAdapter{}
	adapter.SetCtx(ctx, nil)

	err := adapter.ConnectTo("localhost:8080")
	if err == nil {
		t.Error("Expected error for protocol adapter not set")
	}
	if !strings.Contains(err.Error(), "protocol adapter not set") {
		t.Errorf("Expected error containing 'protocol adapter not set', got: %v", err)
	}
}

// TestBaseAdapterListenFrom 测试 ListenFrom 方法
func TestBaseAdapterListenFrom(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建一个自定义的 accept 函数，在关闭时返回错误
	closed := make(chan struct{})
	adapter := newMockProtocolAdapter(ctx)
	adapter.acceptFunc = func() (io.ReadWriteCloser, error) {
		<-closed
		return nil, errors.New("closed")
	}

	err := adapter.ListenFrom("localhost:8080")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// 验证地址设置
	if adapter.Addr() != "localhost:8080" {
		t.Errorf("Expected address 'localhost:8080', got '%s'", adapter.Addr())
	}

	// 关闭触发 accept 退出
	close(closed)
	adapter.Close()
}

// TestBaseAdapterListenFromEmptyAddress 测试空地址的 ListenFrom
func TestBaseAdapterListenFromEmptyAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)

	err := adapter.ListenFrom("")
	if err == nil {
		t.Error("Expected error for empty address")
	}
	if !strings.Contains(err.Error(), "address not set") {
		t.Errorf("Expected error containing 'address not set', got: %v", err)
	}

	adapter.Close()
}

// TestBaseAdapterListenFromNoProtocol 测试无协议适配器的 ListenFrom
func TestBaseAdapterListenFromNoProtocol(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &BaseAdapter{}
	adapter.SetCtx(ctx, nil)

	err := adapter.ListenFrom("localhost:8080")
	if err == nil {
		t.Error("Expected error for protocol adapter not set")
	}
	if !strings.Contains(err.Error(), "protocol adapter not set") {
		t.Errorf("Expected error containing 'protocol adapter not set', got: %v", err)
	}
}

// TestBaseAdapterListenFromListenError 测试 Listen 错误
func TestBaseAdapterListenFromListenError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)
	adapter.listenFunc = func(addr string) error {
		return errors.New("listen failed")
	}

	err := adapter.ListenFrom("localhost:8080")
	if err == nil {
		t.Error("Expected error")
	}
	if !strings.Contains(err.Error(), "failed to listen on mock") {
		t.Errorf("Expected error containing 'failed to listen on mock', got: %v", err)
	}

	adapter.Close()
}

// TestBaseAdapterGetReaderWriter 测试 GetReader 和 GetWriter
func TestBaseAdapterGetReaderWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)

	// 未连接时应返回 nil
	if adapter.GetReader() != nil {
		t.Error("Expected nil reader before connection")
	}
	if adapter.GetWriter() != nil {
		t.Error("Expected nil writer before connection")
	}

	// 连接后应返回非 nil
	err := adapter.ConnectTo("localhost:8080")
	if err != nil {
		t.Fatalf("Connection failed: %v", err)
	}

	if adapter.GetReader() == nil {
		t.Error("Expected non-nil reader after connection")
	}
	if adapter.GetWriter() == nil {
		t.Error("Expected non-nil writer after connection")
	}

	adapter.Close()
}

// TestBaseAdapterClose 测试 Close 方法
func TestBaseAdapterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)

	err := adapter.ConnectTo("localhost:8080")
	if err != nil {
		t.Fatalf("Connection failed: %v", err)
	}

	err = adapter.Close()
	if err != nil {
		t.Errorf("Expected no error on close, got: %v", err)
	}
}

// TestBaseAdapterSession 测试 Session 相关方法
func TestBaseAdapterSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)

	// 初始应为 nil
	if adapter.GetSession() != nil {
		t.Error("Expected nil session initially")
	}

	// SetSession 已在 mock 实现中设置为 nil
	// 这里只测试基本功能
	adapter.SetSession(nil)
	if adapter.GetSession() != nil {
		t.Error("Expected nil session after SetSession(nil)")
	}

	adapter.Close()
}

// TestIsIgnorableError 测试可忽略错误检测
func TestIsIgnorableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "normal error",
			err:      errors.New("normal error"),
			expected: false,
		},
		{
			name:     "timeout error code",
			err:      coreerrors.New(coreerrors.CodeTimeout, "timeout"),
			expected: true,
		},
		{
			name:     "network timeout error",
			err:      &mockNetError{timeout: true},
			expected: true,
		},
		{
			name:     "network non-timeout error",
			err:      &mockNetError{timeout: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIgnorableError(tt.err)
			if result != tt.expected {
				t.Errorf("isIgnorableError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// mockNetError 模拟网络错误
type mockNetError struct {
	timeout   bool
	temporary bool
}

func (e *mockNetError) Error() string {
	return "mock network error"
}

func (e *mockNetError) Timeout() bool {
	return e.timeout
}

func (e *mockNetError) Temporary() bool {
	return e.temporary
}

// 确保 mockNetError 实现 net.Error 接口
var _ net.Error = (*mockNetError)(nil)

// TestIsTimeoutError 测试超时错误检测
func TestIsTimeoutError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &BaseAdapter{}
	adapter.SetCtx(ctx, nil)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "normal error",
			err:      errors.New("normal error"),
			expected: false,
		},
		{
			name:     "timeout and temporary error",
			err:      &mockTimeoutError{timeout: true, temporary: true},
			expected: true,
		},
		{
			name:     "timeout but not temporary",
			err:      &mockTimeoutError{timeout: true, temporary: false},
			expected: false,
		},
		{
			name:     "wrapped timeout error",
			err:      fmt.Errorf("wrapped: %w", &mockTimeoutError{timeout: true, temporary: true}),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.isTimeoutError(tt.err)
			if result != tt.expected {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// mockTimeoutError 模拟超时错误
type mockTimeoutError struct {
	timeout   bool
	temporary bool
}

func (e *mockTimeoutError) Error() string {
	return "mock timeout error"
}

func (e *mockTimeoutError) Timeout() bool {
	return e.timeout
}

func (e *mockTimeoutError) Temporary() bool {
	return e.temporary
}

// TestIsTunnelModeSwitch 测试隧道模式切换检测
func TestIsTunnelModeSwitch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &BaseAdapter{}
	adapter.SetCtx(ctx, nil)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "tunnel mode switch error",
			err:      coreerrors.New(coreerrors.CodeTunnelModeSwitch, "switched"),
			expected: true,
		},
		{
			name:     "other error",
			err:      coreerrors.New(coreerrors.CodeNetworkError, "network error"),
			expected: false,
		},
		{
			name:     "plain error",
			err:      errors.New("plain error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.isTunnelModeSwitch(tt.err)
			if result != tt.expected {
				t.Errorf("isTunnelModeSwitch(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestAcceptLoopWithTimeout 测试带超时的 accept 循环
func TestAcceptLoopWithTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	acceptCount := 0
	adapter := newMockProtocolAdapter(ctx)
	adapter.acceptFunc = func() (io.ReadWriteCloser, error) {
		acceptCount++
		if acceptCount < 3 {
			// 返回超时错误，应该继续循环
			return nil, coreerrors.New(coreerrors.CodeTimeout, "timeout")
		}
		// 返回真实错误，应该退出循环
		return nil, errors.New("real error")
	}

	// 启动监听
	err := adapter.ListenFrom("localhost:8080")
	if err != nil {
		t.Fatalf("ListenFrom failed: %v", err)
	}

	// 等待循环执行几次
	time.Sleep(100 * time.Millisecond)

	// 关闭适配器
	cancel()
	adapter.Close()

	// 验证超时被忽略，循环继续执行
	if acceptCount < 3 {
		t.Errorf("Expected at least 3 accept calls, got %d", acceptCount)
	}
}

// TestCheckAndHandleStreamMode 测试流模式检查
func TestCheckAndHandleStreamMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &BaseAdapter{}
	adapter.SetCtx(ctx, nil)

	// 测试 nil streamConn
	state := &connectionState{streamConn: nil}
	result := adapter.checkAndHandleStreamMode(state)
	if result {
		t.Error("Expected false for nil streamConn")
	}
}

// TestConnectionStateCleanup 测试连接状态清理
func TestConnectionStateCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &BaseAdapter{}
	adapter.SetCtx(ctx, nil)

	mockConn := newMockReadWriteCloser()
	state := &connectionState{
		streamConn:      nil,
		shouldCloseConn: true,
	}

	// 测试正常关闭
	adapter.cleanupConnection(state, mockConn)

	if !mockConn.closed {
		t.Error("Expected connection to be closed")
	}
}

// TestConnectionStatePersistent 测试持久连接不关闭
func TestConnectionStatePersistent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &BaseAdapter{}
	adapter.SetCtx(ctx, nil)

	mockConn := newMockReadWriteCloser()
	state := &connectionState{
		streamConn:      nil,
		shouldCloseConn: false, // 持久连接
	}

	adapter.cleanupConnection(state, mockConn)

	if mockConn.closed {
		t.Error("Expected persistent connection to NOT be closed")
	}
}

// TestConcurrentAccess 测试并发访问
func TestConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := newMockProtocolAdapter(ctx)

	// 连接
	err := adapter.ConnectTo("localhost:8080")
	if err != nil {
		t.Fatalf("Connection failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = adapter.GetReader()
			_ = adapter.GetWriter()
			_ = adapter.Name()
			_ = adapter.GetAddr()
		}()
	}

	wg.Wait()
	adapter.Close()
}
