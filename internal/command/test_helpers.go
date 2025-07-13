package command

import (
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol"
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
func (th *TestHelper) CreateMockStreamPacket(commandType packet.CommandType, body string) *protocol.StreamPacket {
	return &protocol.StreamPacket{
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
func (th *TestHelper) CreateMockCommandHandler(commandType packet.CommandType, responseType ResponseType, handleFunc func(*CommandContext) (*CommandResponse, error)) CommandHandler {
	// 这里返回一个简单的实现，实际使用时需要在测试文件中定义具体的MockCommandHandler
	return &simpleMockHandler{
		commandType:  commandType,
		responseType: responseType,
		handleFunc:   handleFunc,
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

// simpleMockHandler 简单的模拟处理器实现
type simpleMockHandler struct {
	commandType  packet.CommandType
	responseType ResponseType
	handleFunc   func(*CommandContext) (*CommandResponse, error)
}

func (s *simpleMockHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	if s.handleFunc != nil {
		return s.handleFunc(ctx)
	}
	return &CommandResponse{Success: true}, nil
}

func (s *simpleMockHandler) GetResponseType() ResponseType {
	return s.responseType
}

func (s *simpleMockHandler) GetCommandType() packet.CommandType {
	return s.commandType
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
		th.t.Errorf("%s: %v", message, err)
	}
}

// AssertError 断言有错误
func (th *TestHelper) AssertError(err error, message string) {
	if err == nil {
		th.t.Errorf("%s: expected error but got nil", message)
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
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	th.t.Errorf("%s: condition not met within %v", message, timeout)
}

// ConcurrentTest 并发测试辅助函数
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

// Run 运行并发测试
func (ct *ConcurrentTest) Run(testFunc func() error) {
	ct.wg.Add(1)
	go func() {
		defer ct.wg.Done()
		err := testFunc()
		ct.results <- err
	}()
}

// WaitAndCheck 等待所有测试完成并检查结果
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
		if scenario.Assertions != nil {
			scenario.Assertions(t)
		}

		// 清理
		if scenario.Cleanup != nil {
			scenario.Cleanup()
		}

		// 检查执行错误
		if err != nil {
			t.Errorf("Scenario execution failed: %v", err)
		}
	})
}

// BenchmarkHelper 基准测试辅助
type BenchmarkHelper struct {
	registry     *CommandRegistry
	executor     *CommandExecutor
	streamPacket *protocol.StreamPacket
}

// NewBenchmarkHelper 创建基准测试辅助对象
func NewBenchmarkHelper() *BenchmarkHelper {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &simpleMockHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true, Data: "benchmark result"}, nil
		},
	}

	registry.Register(handler)

	// 创建流数据包
	streamPacket := &protocol.StreamPacket{
		ConnectionID: "benchmark-connection",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.TcpMap,
				Token:       "benchmark-token",
				SenderId:    "benchmark-sender",
				ReceiverId:  "benchmark-receiver",
				CommandBody: `{"port": 8080}`,
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
	for i := 0; i < b.N; i++ {
		err := bh.executor.Execute(bh.streamPacket)
		if err != nil {
			b.Errorf("Execute failed: %v", err)
		}
	}
}

// TestData 测试数据结构
type TestData struct {
	CommandType     packet.CommandType
	RequestBody     string
	ExpectedSuccess bool
	ExpectedError   string
}

// TestDataSet 测试数据集
var TestDataSet = []TestData{
	{
		CommandType:     packet.TcpMap,
		RequestBody:     `{"port": 8080}`,
		ExpectedSuccess: true,
		ExpectedError:   "",
	},
	{
		CommandType:     packet.HttpMap,
		RequestBody:     `{"port": 3000}`,
		ExpectedSuccess: true,
		ExpectedError:   "",
	},
	{
		CommandType:     packet.SocksMap,
		RequestBody:     `{"port": 1080}`,
		ExpectedSuccess: true,
		ExpectedError:   "",
	},
	{
		CommandType:     packet.TcpMap,
		RequestBody:     `invalid json`,
		ExpectedSuccess: false,
		ExpectedError:   "invalid",
	},
}

// RunTestData 运行测试数据
func RunTestData(t *testing.T, testFunc func(TestData) error) {
	for _, data := range TestDataSet {
		t.Run(string(data.CommandType), func(t *testing.T) {
			err := testFunc(data)
			if err != nil {
				t.Errorf("Test data execution failed: %v", err)
			}
		})
	}
}
