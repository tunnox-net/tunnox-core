package dispose

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/utils"
)

// MockResource 模拟资源，用于测试
type MockResource struct {
	name         string
	disposed     bool
	mu           sync.Mutex
	disposeCount int
}

func NewMockResource(name string) *MockResource {
	return &MockResource{
		name: name,
	}
}

func (mr *MockResource) Dispose() error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.disposed {
		return fmt.Errorf("resource %s already disposed", mr.name)
	}

	mr.disposed = true
	mr.disposeCount++
	utils.Infof("MockResource %s disposed", mr.name)
	return nil
}

func (mr *MockResource) IsDisposed() bool {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return mr.disposed
}

func (mr *MockResource) GetDisposeCount() int {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return mr.disposeCount
}

// SlowMockResource 慢速模拟资源
type SlowMockResource struct {
	name     string
	disposed bool
	mu       sync.Mutex
}

func (smr *SlowMockResource) Dispose() error {
	smr.mu.Lock()
	defer smr.mu.Unlock()

	if smr.disposed {
		return fmt.Errorf("resource %s already disposed", smr.name)
	}

	time.Sleep(2 * time.Second) // 模拟慢速释放
	smr.disposed = true
	utils.Infof("SlowMockResource %s disposed", smr.name)
	return nil
}

// ErrorMockResource 会出错的模拟资源
type ErrorMockResource struct {
	name     string
	disposed bool
	mu       sync.Mutex
}

func (emr *ErrorMockResource) Dispose() error {
	emr.mu.Lock()
	defer emr.mu.Unlock()

	if emr.disposed {
		return fmt.Errorf("resource %s already disposed", emr.name)
	}

	emr.disposed = true
	utils.Infof("ErrorMockResource %s disposed", emr.name)
	return fmt.Errorf("simulated disposal error for %s", emr.name)
}

// OrderTrackingResource 用于跟踪释放顺序的资源包装器
type OrderTrackingResource struct {
	resource *MockResource
	name     string
	order    *[]string
	mu       *sync.Mutex
}

func (otr *OrderTrackingResource) Dispose() error {
	otr.mu.Lock()
	*otr.order = append(*otr.order, otr.name)
	otr.mu.Unlock()
	return otr.resource.Dispose()
}

// TestDisposeIntegration 测试所有组件的Dispose集成
func TestDisposeIntegration(t *testing.T) {
	// 创建资源管理器
	resourceMgr := utils.NewResourceManager()

	// 创建模拟资源
	resources := []*MockResource{
		NewMockResource("resource-1"),
		NewMockResource("resource-2"),
		NewMockResource("resource-3"),
	}

	// 注册资源
	for _, resource := range resources {
		if err := resourceMgr.Register(resource.name, resource); err != nil {
			t.Fatalf("Failed to register resource %s: %v", resource.name, err)
		}
	}

	// 验证资源数量
	if count := resourceMgr.GetResourceCount(); count != 3 {
		t.Errorf("Expected 3 resources, got %d", count)
	}

	// 释放所有资源
	result := resourceMgr.DisposeAll()

	// 验证释放结果
	if result.HasErrors() {
		t.Errorf("Resource disposal failed: %v", result.Error())
	}

	// 验证所有资源都被释放
	for _, resource := range resources {
		if !resource.IsDisposed() {
			t.Errorf("Resource %s was not disposed", resource.name)
		}
	}

	// 验证资源列表已清空
	if count := resourceMgr.GetResourceCount(); count != 0 {
		t.Errorf("Expected 0 resources after disposal, got %d", count)
	}
}

// TestDisposeOrder 测试资源释放顺序
func TestDisposeOrder(t *testing.T) {
	resourceMgr := utils.NewResourceManager()

	// 记录释放顺序
	var disposeOrder []string
	var mu sync.Mutex

	// 创建带顺序记录的模拟资源
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("resource-%d", i)
		resource := NewMockResource(name)

		// 创建包装器来记录顺序
		wrapper := &OrderTrackingResource{
			resource: resource,
			name:     name,
			order:    &disposeOrder,
			mu:       &mu,
		}

		if err := resourceMgr.Register(name, wrapper); err != nil {
			t.Fatalf("Failed to register resource %s: %v", name, err)
		}
	}

	// 释放资源
	result := resourceMgr.DisposeAll()
	if result.HasErrors() {
		t.Errorf("Resource disposal failed: %v", result.Error())
	}

	// 验证释放顺序（应该与注册顺序相反）
	expectedOrder := []string{"resource-5", "resource-4", "resource-3", "resource-2", "resource-1"}
	if len(disposeOrder) != len(expectedOrder) {
		t.Errorf("Expected %d disposed resources, got %d", len(expectedOrder), len(disposeOrder))
	}

	for i, expected := range expectedOrder {
		if i >= len(disposeOrder) || disposeOrder[i] != expected {
			t.Errorf("Expected dispose order[%d] = %s, got %s", i, expected, disposeOrder[i])
		}
	}
}

// TestDisposeTimeout 测试带超时的资源释放
func TestDisposeTimeout(t *testing.T) {
	resourceMgr := utils.NewResourceManager()

	// 创建慢速资源
	slowResource := &SlowMockResource{name: "slow-resource"}

	if err := resourceMgr.Register("slow-resource", slowResource); err != nil {
		t.Fatalf("Failed to register slow resource: %v", err)
	}

	// 使用短超时时间
	result := resourceMgr.DisposeWithTimeout(500 * time.Millisecond)

	// 应该因为超时而失败
	if !result.HasErrors() {
		t.Error("Expected timeout error, but disposal succeeded")
	}

	// 验证错误信息包含超时
	if len(result.Errors) == 0 || result.Errors[0].ResourceName != "timeout" {
		t.Error("Expected timeout error in result")
	}
}

// TestConcurrentDispose 测试并发资源释放
func TestConcurrentDispose(t *testing.T) {
	resourceMgr := utils.NewResourceManager()

	// 创建多个资源
	resources := make([]*MockResource, 10)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("concurrent-resource-%d", i)
		resources[i] = NewMockResource(name)
		if err := resourceMgr.Register(name, resources[i]); err != nil {
			t.Fatalf("Failed to register resource %s: %v", name, err)
		}
	}

	// 并发释放资源
	var wg sync.WaitGroup
	results := make([]*utils.DisposeResult, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = resourceMgr.DisposeAll()
			t.Logf("Goroutine %d: result has %d errors, resource count: %d",
				index, len(results[index].Errors), resourceMgr.GetResourceCount())
		}(i)
	}

	wg.Wait()

	// 验证只有一个结果成功，其他应该返回空结果（因为资源已经被释放）
	successCount := 0
	for i, result := range results {
		// 成功的释放应该没有错误，并且应该实际释放了资源
		if len(result.Errors) == 0 && result.ActualDisposal {
			successCount++
			t.Logf("Result %d: SUCCESS (actually disposed resources)", i)
		} else if len(result.Errors) == 0 && !result.ActualDisposal {
			t.Logf("Result %d: EMPTY (no errors, but no resources disposed)", i)
		} else {
			t.Logf("Result %d: FAILED (errors: %d)", i, len(result.Errors))
		}
	}

	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful disposal, got %d", successCount)
	}
}

// TestServiceManagerDispose 测试服务管理器的资源释放
func TestServiceManagerDispose(t *testing.T) {
	// 创建服务配置
	config := utils.DefaultServiceConfig()
	config.GracefulShutdownTimeout = 1 * time.Second
	config.ResourceDisposeTimeout = 1 * time.Second

	// 创建服务管理器
	serviceMgr := utils.NewServiceManager(config)

	// 注册资源
	resources := []*MockResource{
		NewMockResource("server-resource-1"),
		NewMockResource("server-resource-2"),
	}

	for _, resource := range resources {
		if err := serviceMgr.RegisterResource(resource.name, resource); err != nil {
			t.Fatalf("Failed to register resource %s: %v", resource.name, err)
		}
	}

	// 创建HTTP服务
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	httpService := utils.NewHTTPService(":0", handler)
	serviceMgr.RegisterService(httpService)

	// 启动服务（在goroutine中）
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		if err := serviceMgr.RunWithContext(ctx); err != nil {
			t.Errorf("Service error: %v", err)
		}
	}()

	// 等待上下文取消
	<-ctx.Done()

	// 验证资源被释放
	result := serviceMgr.GetDisposeResult()
	if result != nil && result.HasErrors() {
		t.Errorf("Service resource disposal failed: %v", result.Error())
	}
}

// TestDisposeWithErrors 测试包含错误的资源释放
func TestDisposeWithErrors(t *testing.T) {
	resourceMgr := utils.NewResourceManager()

	// 创建正常资源
	normalResource := NewMockResource("normal-resource")
	if err := resourceMgr.Register("normal-resource", normalResource); err != nil {
		t.Fatalf("Failed to register normal resource: %v", err)
	}

	// 创建会出错的资源
	errorResource := &ErrorMockResource{name: "error-resource"}

	if err := resourceMgr.Register("error-resource", errorResource); err != nil {
		t.Fatalf("Failed to register error resource: %v", err)
	}

	// 释放资源
	result := resourceMgr.DisposeAll()

	// 应该包含错误
	if !result.HasErrors() {
		t.Error("Expected disposal errors, but none occurred")
	}

	// 验证错误信息
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}

	if result.Errors[0].ResourceName != "error-resource" {
		t.Errorf("Expected error from error-resource, got %s", result.Errors[0].ResourceName)
	}

	// 验证正常资源也被释放了
	if !normalResource.IsDisposed() {
		t.Error("Normal resource was not disposed")
	}
}

// TestDisposeReentrancy 测试资源释放的可重入性
func TestDisposeReentrancy(t *testing.T) {
	resourceMgr := utils.NewResourceManager()

	// 创建资源
	resource := NewMockResource("reentrant-resource")
	if err := resourceMgr.Register("reentrant-resource", resource); err != nil {
		t.Fatalf("Failed to register resource: %v", err)
	}

	// 第一次释放
	result1 := resourceMgr.DisposeAll()
	if result1.HasErrors() {
		t.Errorf("First disposal failed: %v", result1.Error())
	}

	// 第二次释放（应该返回空结果）
	result2 := resourceMgr.DisposeAll()
	if len(result2.Errors) != 0 {
		t.Errorf("Second disposal should return empty result, got %d errors", len(result2.Errors))
	}

	// 验证资源只被释放一次
	if resource.GetDisposeCount() != 1 {
		t.Errorf("Expected resource to be disposed once, got %d times", resource.GetDisposeCount())
	}
}

// TestDisposeStress 压力测试
func TestDisposeStress(t *testing.T) {
	resourceMgr := utils.NewResourceManager()

	// 创建大量资源
	const numResources = 1000
	resources := make([]*MockResource, numResources)

	for i := 0; i < numResources; i++ {
		name := fmt.Sprintf("stress-resource-%d", i)
		resources[i] = NewMockResource(name)
		if err := resourceMgr.Register(name, resources[i]); err != nil {
			t.Fatalf("Failed to register resource %s: %v", name, err)
		}
	}

	// 验证资源数量
	if count := resourceMgr.GetResourceCount(); count != numResources {
		t.Errorf("Expected %d resources, got %d", numResources, count)
	}

	// 释放所有资源
	start := time.Now()
	result := resourceMgr.DisposeAll()
	duration := time.Since(start)

	// 验证释放结果
	if result.HasErrors() {
		t.Errorf("Stress test disposal failed: %v", result.Error())
	}

	// 验证所有资源都被释放
	for i, resource := range resources {
		if !resource.IsDisposed() {
			t.Errorf("Resource %d was not disposed", i)
		}
	}

	// 验证性能（释放1000个资源应该在合理时间内完成）
	if duration > 5*time.Second {
		t.Errorf("Disposal took too long: %v", duration)
	}

	t.Logf("Disposed %d resources in %v", numResources, duration)
}

// TestGlobalResourceManager 测试全局资源管理器
func TestGlobalResourceManager(t *testing.T) {
	// 注册全局资源
	resource1 := NewMockResource("global-resource-1")
	resource2 := NewMockResource("global-resource-2")

	if err := utils.RegisterGlobalResource("global-resource-1", resource1); err != nil {
		t.Fatalf("Failed to register global resource 1: %v", err)
	}

	if err := utils.RegisterGlobalResource("global-resource-2", resource2); err != nil {
		t.Fatalf("Failed to register global resource 2: %v", err)
	}

	// 释放所有全局资源
	result := utils.DisposeAllGlobalResources()

	// 验证释放结果
	if result.HasErrors() {
		t.Errorf("Global resource disposal failed: %v", result.Error())
	}

	// 验证所有资源都被释放
	if !resource1.IsDisposed() {
		t.Error("Global resource 1 was not disposed")
	}

	if !resource2.IsDisposed() {
		t.Error("Global resource 2 was not disposed")
	}
}

// TestDisposeWithContext 测试带上下文的资源释放
func TestDisposeWithContext(t *testing.T) {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 创建资源管理器
	resourceMgr := utils.NewResourceManager()

	// 创建慢速资源
	slowResource := &SlowMockResource{name: "context-slow-resource"}
	if err := resourceMgr.Register("context-slow-resource", slowResource); err != nil {
		t.Fatalf("Failed to register slow resource: %v", err)
	}

	// 在goroutine中释放资源
	resultChan := make(chan *utils.DisposeResult, 1)
	go func() {
		resultChan <- resourceMgr.DisposeAll()
	}()

	// 等待上下文取消或资源释放完成
	select {
	case result := <-resultChan:
		if result.HasErrors() {
			t.Errorf("Resource disposal failed: %v", result.Error())
		}
	case <-ctx.Done():
		t.Log("Context cancelled, resource disposal may still be in progress")
	}
}
