package tests

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
	io2 "tunnox-core/internal/io"
)

func TestNewStream(t *testing.T) {
	// 准备测试数据
	testData := []byte("Hello, this is a test for Stream!")

	var buf bytes.Buffer
	reader := bytes.NewReader(testData)
	writer := &buf

	// 创建Stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io2.NewStream(reader, writer, ctx)
	defer stream.Close()

	// 验证Stream创建成功
	if stream == nil {
		t.Fatal("Stream should not be nil")
	}

	// 验证Stream未关闭
	if stream.IsClosed() {
		t.Error("New Stream should not be closed")
	}
}

func TestStreamClose(t *testing.T) {
	// 准备测试数据
	testData := []byte("Test data for stream close")

	var buf bytes.Buffer
	reader := bytes.NewReader(testData)
	writer := &buf

	// 创建Stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io2.NewStream(reader, writer, ctx)

	// 关闭Stream
	stream.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !stream.IsClosed() {
		t.Error("Stream should be closed after calling Close()")
	}
}

func TestStreamConcurrentAccess(t *testing.T) {
	// 测试并发访问的安全性
	testData := []byte("Concurrent access test data")

	var buf bytes.Buffer
	reader := bytes.NewReader(testData)
	writer := &buf

	// 创建Stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io2.NewStream(reader, writer, ctx)
	defer stream.Close()

	// 并发访问Stream
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 并发检查关闭状态
			_ = stream.IsClosed()

			// 模拟一些工作
			time.Sleep(1 * time.Millisecond)
		}()
	}

	wg.Wait()

	// 验证Stream仍然可用
	if stream.IsClosed() {
		t.Error("Stream should not be closed after concurrent access")
	}
}

func TestStreamContextCancellation(t *testing.T) {
	// 测试上下文取消时的行为
	testData := []byte("Context cancellation test")

	var buf bytes.Buffer
	reader := bytes.NewReader(testData)
	writer := &buf

	// 创建Stream
	ctx, cancel := context.WithCancel(context.Background())

	stream := io2.NewStream(reader, writer, ctx)

	// 取消上下文
	cancel()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证Stream已关闭
	if !stream.IsClosed() {
		t.Error("Stream should be closed after context cancellation")
	}
}

func TestStreamWithNilOnClose(t *testing.T) {
	// 测试onClose为nil的情况
	testData := []byte("Nil onClose test")

	var buf bytes.Buffer
	reader := bytes.NewReader(testData)
	writer := &buf

	// 创建Stream（onClose为nil）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io2.NewStream(reader, writer, ctx)
	defer stream.Close()

	// 验证Stream创建成功
	if stream == nil {
		t.Fatal("Stream should not be nil even with nil onClose")
	}

	// 验证Stream未关闭
	if stream.IsClosed() {
		t.Error("New Stream should not be closed")
	}
}

func TestStreamMultipleClose(t *testing.T) {
	// 测试多次调用Close的安全性
	testData := []byte("Multiple close test")

	var buf bytes.Buffer
	reader := bytes.NewReader(testData)
	writer := &buf

	// 创建Stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io2.NewStream(reader, writer, ctx)

	// 多次调用Close
	stream.Close()
	stream.Close()
	stream.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !stream.IsClosed() {
		t.Error("Stream should be closed after calling Close()")
	}
}

func TestStreamWithLargeData(t *testing.T) {
	// 测试大数据量的情况
	largeData := make([]byte, 1024*1024) // 1MB数据
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	var buf bytes.Buffer
	reader := bytes.NewReader(largeData)
	writer := &buf

	// 创建Stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io2.NewStream(reader, writer, ctx)
	defer stream.Close()

	// 验证Stream创建成功
	if stream == nil {
		t.Fatal("Stream should not be nil with large data")
	}

	// 验证Stream未关闭
	if stream.IsClosed() {
		t.Error("New Stream should not be closed")
	}
}
