package utils

import (
	"bytes"
	"io"
	"testing"
)

// mockReadWriteCloser 模拟 ReadWriteCloser
type mockReadWriteCloser struct {
	reader *bytes.Reader
	writer *bytes.Buffer
}

func newMockReadWriteCloser(data []byte) *mockReadWriteCloser {
	return &mockReadWriteCloser{
		reader: bytes.NewReader(data),
		writer: &bytes.Buffer{},
	}
}

func (m *mockReadWriteCloser) Read(p []byte) (int, error) {
	return m.reader.Read(p)
}

func (m *mockReadWriteCloser) Write(p []byte) (int, error) {
	return m.writer.Write(p)
}

func (m *mockReadWriteCloser) Close() error {
	return nil
}

func (m *mockReadWriteCloser) getWrittenData() []byte {
	return m.writer.Bytes()
}

// TestDefaultCopyStrategy 测试默认拷贝策略
func TestDefaultCopyStrategy(t *testing.T) {
	strategy := NewDefaultCopyStrategy()
	
	// 创建测试数据
	testData := []byte("test data")
	connA := newMockReadWriteCloser(testData)
	connB := newMockReadWriteCloser(nil)
	
	// 执行拷贝
	options := &BidirectionalCopyOptions{
		LogPrefix: "TestDefaultCopyStrategy",
	}
	result := strategy.Copy(connA, connB, options)
	
	// 验证结果
	if result.SendError != nil {
		t.Errorf("Expected no send error, got: %v", result.SendError)
	}
	if result.BytesSent != int64(len(testData)) {
		t.Errorf("Expected bytes sent %d, got %d", len(testData), result.BytesSent)
	}
}

// TestHTTPPollCopyStrategy 测试 HTTP-poll 拷贝策略
func TestHTTPPollCopyStrategy(t *testing.T) {
	strategy := NewHTTPPollCopyStrategy()
	
	// 创建测试数据
	testData := []byte("test data")
	connA := newMockReadWriteCloser(testData)
	connB := newMockReadWriteCloser(nil)
	
	// 执行拷贝
	options := &BidirectionalCopyOptions{
		LogPrefix: "TestHTTPPollCopyStrategy",
	}
	result := strategy.Copy(connA, connB, options)
	
	// 验证结果
	if result.SendError != nil {
		t.Errorf("Expected no send error, got: %v", result.SendError)
	}
	if result.BytesSent != int64(len(testData)) {
		t.Errorf("Expected bytes sent %d, got %d", len(testData), result.BytesSent)
	}
}

// TestCopyStrategyFactory 测试拷贝策略工厂
func TestCopyStrategyFactory(t *testing.T) {
	factory := NewCopyStrategyFactory()
	
	// 测试默认策略
	strategy := factory.CreateStrategy("tcp")
	if _, ok := strategy.(*DefaultCopyStrategy); !ok {
		t.Errorf("Expected DefaultCopyStrategy for 'tcp', got %T", strategy)
	}
	
	// 测试 HTTP-poll 策略
	strategy = factory.CreateStrategy("httppoll")
	if _, ok := strategy.(*HTTPPollCopyStrategy); !ok {
		t.Errorf("Expected HTTPPollCopyStrategy for 'httppoll', got %T", strategy)
	}
	
	// 测试 HTTP-long-polling 策略
	strategy = factory.CreateStrategy("http-long-polling")
	if _, ok := strategy.(*HTTPPollCopyStrategy); !ok {
		t.Errorf("Expected HTTPPollCopyStrategy for 'http-long-polling', got %T", strategy)
	}
	
	// 测试 httplp 策略
	strategy = factory.CreateStrategy("httplp")
	if _, ok := strategy.(*HTTPPollCopyStrategy); !ok {
		t.Errorf("Expected HTTPPollCopyStrategy for 'httplp', got %T", strategy)
	}
	
	// 测试未知协议（应该返回默认策略）
	strategy = factory.CreateStrategy("unknown")
	if _, ok := strategy.(*DefaultCopyStrategy); !ok {
		t.Errorf("Expected DefaultCopyStrategy for 'unknown', got %T", strategy)
	}
}

// TestCopyStrategyWithEmptyData 测试空数据拷贝
func TestCopyStrategyWithEmptyData(t *testing.T) {
	strategy := NewDefaultCopyStrategy()
	
	// 创建空数据连接
	connA := newMockReadWriteCloser(nil)
	connB := newMockReadWriteCloser(nil)
	
	// 执行拷贝
	options := &BidirectionalCopyOptions{
		LogPrefix: "TestCopyStrategyWithEmptyData",
	}
	result := strategy.Copy(connA, connB, options)
	
	// 验证结果（空数据应该成功完成）
	if result.SendError != nil && result.SendError != io.EOF {
		t.Errorf("Expected EOF or nil error, got: %v", result.SendError)
	}
}

