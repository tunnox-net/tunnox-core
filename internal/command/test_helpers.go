package command

import (
	"fmt"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// TestHelper 测试辅助结构
type TestHelper struct {
	t *testing.T
}

// NewTestHelper 创建测试辅助对象
func NewTestHelper(t *testing.T) *TestHelper {
	return &TestHelper{t: t}
}

// CreateMockStreamPacket 创建模拟流数据包
func (th *TestHelper) CreateMockStreamPacket(commandType packet.CommandType, body string) *types.StreamPacket {
	return &types.StreamPacket{
		ConnectionID: "test-connection-123",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: commandType,
				Token:       "test-token-456",
				SenderId:    "sender-123",
				ReceiverId:  "receiver-456",
				CommandBody: body,
			},
		},
	}
}

// CreateMockCommandHandler 创建模拟命令处理器
func (th *TestHelper) CreateMockCommandHandler(commandType packet.CommandType, direction CommandDirection, handleFunc func(*CommandContext) (*CommandResponse, error)) CommandHandler {
	// 这里返回一个简单的实现，实际使用时需要在测试文件中定义具体的MockCommandHandler
	return &simpleMockHandler{
		commandType: commandType,
		direction:   direction,
		handleFunc:  handleFunc,
	}
}

// CreateMockMiddleware 创建模拟中间件
func (th *TestHelper) CreateMockMiddleware(name string, processFunc func(*CommandContext, func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error)) Middleware {
	// 这里返回一个简单的实现，实际使用时需要在测试文件中定义具体的MockMiddleware
	return &simpleMockMiddleware{
		name:        name,
		processFunc: processFunc,
	}
}

// simpleMockHandler 简单的模拟处理器
type simpleMockHandler struct {
	commandType packet.CommandType
	direction   CommandDirection // 替换 responseType
	handleFunc  func(*CommandContext) (*CommandResponse, error)
}

func newSimpleMockHandler(commandType packet.CommandType, direction CommandDirection) *simpleMockHandler {
	return &simpleMockHandler{
		commandType: commandType,
		direction:   direction,
	}
}

func (s *simpleMockHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	if s.handleFunc != nil {
		return s.handleFunc(ctx)
	}
	return &CommandResponse{Success: true}, nil
}

func (s *simpleMockHandler) GetCommandType() packet.CommandType {
	return s.commandType
}

func (s *simpleMockHandler) GetCategory() CommandCategory {
	return CategoryMapping // 默认分类
}

func (s *simpleMockHandler) GetDirection() CommandDirection {
	return DirectionOneway // 默认方向
}

// simpleMockMiddleware 简单的模拟中间件实现
type simpleMockMiddleware struct {
	name        string
	processFunc func(*CommandContext, func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error)
}

func (s *simpleMockMiddleware) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	if s.processFunc != nil {
		return s.processFunc(ctx, next)
	}
	return next(ctx)
}

// SetupTestExecutor 设置测试执行器
func (th *TestHelper) SetupTestExecutor() (*CommandRegistry, *CommandExecutor) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)
	return registry, executor
}

// AssertNoError 断言没有错误
func (th *TestHelper) AssertNoError(err error, message string) {
	if err != nil {
		th.t.Errorf("%s: unexpected error: %v", message, err)
	}
}

// AssertError 断言有错误
func (th *TestHelper) AssertError(err error, message string) {
	if err == nil {
		th.t.Errorf("%s: expected error but got none", message)
	}
}

// AssertEqual 断言相等
func (th *TestHelper) AssertEqual(expected, actual interface{}, message string) {
	if expected != actual {
		th.t.Errorf("%s: expected %v, got %v", message, expected, actual)
	}
}

// AssertTrue 断言为真
func (th *TestHelper) AssertTrue(condition bool, message string) {
	if !condition {
		th.t.Errorf("%s: expected true, got false", message)
	}
}

// AssertFalse 断言为假
func (th *TestHelper) AssertFalse(condition bool, message string) {
	if condition {
		th.t.Errorf("%s: expected false, got true", message)
	}
}

// WaitForCondition 等待条件满足
func (th *TestHelper) WaitForCondition(condition func() bool, timeout time.Duration, message string) {
	start := time.Now()
	for time.Since(start) < timeout {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	th.t.Errorf("%s: condition not met within timeout", message)
}

// ConcurrentTest 并发测试辅助结构
type ConcurrentTest struct {
	t       *testing.T
	wg      sync.WaitGroup
	results chan error
}

// NewConcurrentTest 创建并发测试对象
func NewConcurrentTest(t *testing.T, numGoroutines int) *ConcurrentTest {
	return &ConcurrentTest{
		t:       t,
		results: make(chan error, numGoroutines),
	}
}

// Run 运行测试函数
func (ct *ConcurrentTest) Run(testFunc func() error) {
	ct.wg.Add(1)
	go func() {
		defer ct.wg.Done()
		ct.results <- testFunc()
	}()
}

// WaitAndCheck 等待并检查结果
func (ct *ConcurrentTest) WaitAndCheck() {
	ct.wg.Wait()
	close(ct.results)

	for err := range ct.results {
		if err != nil {
			ct.t.Errorf("Concurrent test failed: %v", err)
		}
	}
}

// TestScenario 测试场景结构
type TestScenario struct {
	Name       string
	Setup      func() (*CommandRegistry, *CommandExecutor)
	Execute    func(*CommandExecutor) error
	Assertions func(*testing.T)
	Cleanup    func()
}

// RunTestScenario 运行测试场景
func RunTestScenario(t *testing.T, scenario TestScenario) {
	t.Run(scenario.Name, func(t *testing.T) {
		// 设置
		_, executor := scenario.Setup()

		// 执行
		err := scenario.Execute(executor)

		// 断言
		if err != nil {
			t.Errorf("Test execution failed: %v", err)
		}

		if scenario.Assertions != nil {
			scenario.Assertions(t)
		}

		// 清理
		if scenario.Cleanup != nil {
			scenario.Cleanup()
		}
	})
}

// BenchmarkHelper 基准测试辅助结构
type BenchmarkHelper struct {
	registry     *CommandRegistry
	executor     *CommandExecutor
	streamPacket *types.StreamPacket
}

// NewBenchmarkHelper 创建基准测试辅助对象
func NewBenchmarkHelper() *BenchmarkHelper {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建测试数据包
	streamPacket := &types.StreamPacket{
		ConnectionID: "benchmark-connection",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.HeartbeatCmd,
				Token:       "benchmark-token",
				SenderId:    "benchmark-sender",
				ReceiverId:  "benchmark-receiver",
				CommandBody: `{"test": "data"}`,
			},
		},
	}

	return &BenchmarkHelper{
		registry:     registry,
		executor:     executor,
		streamPacket: streamPacket,
	}
}

// BenchmarkExecute 基准测试执行
func (bh *BenchmarkHelper) BenchmarkExecute(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bh.executor.Execute(bh.streamPacket)
	}
}

// TestData 测试数据结构
type TestData struct {
	CommandType     packet.CommandType
	RequestBody     string
	ExpectedSuccess bool
	ExpectedError   string
}

// RunTestData 运行测试数据
func RunTestData(t *testing.T, testFunc func(TestData) error) {
	testCases := []TestData{
		{
			CommandType:     packet.HeartbeatCmd,
			RequestBody:     `{"test": "heartbeat"}`,
			ExpectedSuccess: true,
		},
		{
			CommandType:     packet.Connect,
			RequestBody:     `{"invalid": "json"`,
			ExpectedSuccess: false,
			ExpectedError:   "invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v", tc.CommandType), func(t *testing.T) {
			err := testFunc(tc)
			if tc.ExpectedSuccess && err != nil {
				t.Errorf("Expected success but got error: %v", err)
			}
			if !tc.ExpectedSuccess && err == nil {
				t.Errorf("Expected error but got success")
			}
			if tc.ExpectedError != "" && err != nil && !contains(err.Error(), tc.ExpectedError) {
				t.Errorf("Expected error containing '%s' but got: %v", tc.ExpectedError, err)
			}
		})
	}
}

// contains 检查字符串是否包含子字符串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || func() bool {
		for i := 1; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()))
}
