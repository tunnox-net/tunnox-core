package testutils

import (
	"context"
	"testing"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/errors"
)

// TestHelper 通用测试辅助工具
type TestHelper struct {
	t *testing.T
}

// NewTestHelper 创建新的测试辅助工具
func NewTestHelper(t *testing.T) *TestHelper {
	return &TestHelper{
		t: t,
	}
}

// AssertNoError 断言没有错误
func (h *TestHelper) AssertNoError(err error, message string) {
	if err != nil {
		h.t.Fatalf("%s: %v", message, err)
	}
}

// AssertError 断言有错误
func (h *TestHelper) AssertError(err error, message string) {
	if err == nil {
		h.t.Fatalf("%s: expected error but got nil", message)
	}
}

// AssertEqual 断言相等
func (h *TestHelper) AssertEqual(expected, actual interface{}, message string) {
	if expected != actual {
		h.t.Fatalf("%s: expected %v, got %v", message, expected, actual)
	}
}

// AssertNotEqual 断言不相等
func (h *TestHelper) AssertNotEqual(expected, actual interface{}, message string) {
	if expected == actual {
		h.t.Fatalf("%s: expected not equal to %v, got %v", message, expected, actual)
	}
}

// AssertTrue 断言为真
func (h *TestHelper) AssertTrue(condition bool, message string) {
	if !condition {
		h.t.Fatalf("%s: expected true, got false", message)
	}
}

// AssertFalse 断言为假
func (h *TestHelper) AssertFalse(condition bool, message string) {
	if condition {
		h.t.Fatalf("%s: expected false, got true", message)
	}
}

// AssertNil 断言为nil
func (h *TestHelper) AssertNil(value interface{}, message string) {
	if value != nil {
		h.t.Fatalf("%s: expected nil, got %v", message, value)
	}
}

// AssertNotNil 断言不为nil
func (h *TestHelper) AssertNotNil(value interface{}, message string) {
	if value == nil {
		h.t.Fatalf("%s: expected not nil, got nil", message)
	}
}

// MockResource 模拟资源
type MockResource struct {
	*dispose.ResourceBase
	closed bool
}

// NewMockResource 创建新的模拟资源
func NewMockResource(name string) *MockResource {
	return &MockResource{
		ResourceBase: dispose.NewResourceBase(name),
		closed:       false,
	}
}

// IsClosed 检查是否已关闭
func (m *MockResource) IsClosed() bool {
	return m.closed
}

// Close 关闭资源
func (m *MockResource) Close() error {
	m.closed = true
	return m.ResourceBase.Close()
}

// MockService 模拟服务
type MockService struct {
	*dispose.ResourceBase
	name   string
	closed bool
}

// NewMockService 创建新的模拟服务
func NewMockService(name string) *MockService {
	return &MockService{
		ResourceBase: dispose.NewResourceBase(name),
		name:         name,
		closed:       false,
	}
}

// GetName 获取服务名称
func (m *MockService) GetName() string {
	return m.name
}

// IsClosed 检查是否已关闭
func (m *MockService) IsClosed() bool {
	return m.closed
}

// Close 关闭服务
func (m *MockService) Close() error {
	m.closed = true
	return m.ResourceBase.Close()
}

// ConcurrentTest 并发测试工具
type ConcurrentTest struct {
	t             *testing.T
	numGoroutines int
	results       chan error
	timeout       time.Duration
}

// NewConcurrentTest 创建新的并发测试工具
func NewConcurrentTest(t *testing.T, numGoroutines int) *ConcurrentTest {
	return &ConcurrentTest{
		t:             t,
		numGoroutines: numGoroutines,
		results:       make(chan error, numGoroutines),
		timeout:       30 * time.Second,
	}
}

// SetTimeout 设置超时时间
func (ct *ConcurrentTest) SetTimeout(timeout time.Duration) {
	ct.timeout = timeout
}

// RunConcurrent 运行并发测试
func (ct *ConcurrentTest) RunConcurrent(testFunc func() error) {
	ctx, cancel := context.WithTimeout(context.Background(), ct.timeout)
	defer cancel()

	// 启动并发测试
	for i := 0; i < ct.numGoroutines; i++ {
		go func() {
			select {
			case ct.results <- testFunc():
			case <-ctx.Done():
				ct.results <- errors.NewStandardErrorWithCause(
					errors.ErrCodeTimeout,
					"concurrent test timeout",
					ctx.Err(),
				)
			}
		}()
	}

	// 收集结果
	for i := 0; i < ct.numGoroutines; i++ {
		select {
		case err := <-ct.results:
			if err != nil {
				ct.t.Errorf("Concurrent test failed: %v", err)
			}
		case <-ctx.Done():
			ct.t.Fatalf("Concurrent test timeout after %v", ct.timeout)
		}
	}
}

// BenchmarkHelper 基准测试辅助工具
type BenchmarkHelper struct {
	iterations int
	timeout    time.Duration
}

// NewBenchmarkHelper 创建新的基准测试辅助工具
func NewBenchmarkHelper() *BenchmarkHelper {
	return &BenchmarkHelper{
		iterations: 1000,
		timeout:    10 * time.Second,
	}
}

// SetIterations 设置迭代次数
func (bh *BenchmarkHelper) SetIterations(iterations int) {
	bh.iterations = iterations
}

// SetTimeout 设置超时时间
func (bh *BenchmarkHelper) SetTimeout(timeout time.Duration) {
	bh.timeout = timeout
}

// RunBenchmark 运行基准测试
func (bh *BenchmarkHelper) RunBenchmark(b *testing.B, benchmarkFunc func() error) {
	ctx, cancel := context.WithTimeout(context.Background(), bh.timeout)
	defer cancel()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < bh.iterations; i++ {
		select {
		case <-ctx.Done():
			b.Fatalf("Benchmark timeout after %v", bh.timeout)
		default:
			if err := benchmarkFunc(); err != nil {
				b.Errorf("Benchmark failed: %v", err)
			}
		}
	}
}

// TestContext 测试上下文
type TestContext struct {
	context.Context
	helper *TestHelper
}

// NewTestContext 创建新的测试上下文
func NewTestContext(t *testing.T) *TestContext {
	return &TestContext{
		Context: context.Background(),
		helper:  NewTestHelper(t),
	}
}

// GetHelper 获取测试辅助工具
func (tc *TestContext) GetHelper() *TestHelper {
	return tc.helper
}

// WithTimeout 设置超时
func (tc *TestContext) WithTimeout(timeout time.Duration) *TestContext {
	ctx, _ := context.WithTimeout(tc.Context, timeout)
	return &TestContext{
		Context: ctx,
		helper:  tc.helper,
	}
}

// WithCancel 设置取消
func (tc *TestContext) WithCancel() (*TestContext, context.CancelFunc) {
	ctx, cancel := context.WithCancel(tc.Context)
	return &TestContext{
		Context: ctx,
		helper:  tc.helper,
	}, cancel
}
