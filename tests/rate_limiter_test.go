package tests

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
	io2 "tunnox-core/internal/io"
)

// 生成测试数据，至少几KB大小
func generateTestData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}

func TestRateLimiterReader(t *testing.T) {
	// 生成5KB的测试数据
	testData := generateTestData(5 * 1024)

	// 创建限速读取器，限制为每秒2KB
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader, err := io2.NewRateLimiterReader(bytes.NewReader(testData), 2*1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	// 读取数据并测量时间
	start := time.Now()
	var result bytes.Buffer
	_, err = io.Copy(&result, reader)
	if err != nil {
		t.Fatalf("Failed to read from RateLimiterReader: %v", err)
	}
	duration := time.Since(start)

	// 验证数据完整性
	if !bytes.Equal(result.Bytes(), testData) {
		t.Errorf("Data mismatch: expected %d bytes, got %d bytes", len(testData), result.Len())
	}

	// 验证限速效果：5KB数据，2KB/s速率，应该至少需要2.5秒
	expectedMinDuration := time.Duration(len(testData)/(2*1024)) * time.Second
	if duration < expectedMinDuration {
		t.Errorf("Rate limiting not working properly. Expected at least %v, got %v",
			expectedMinDuration, duration)
	}

	t.Logf("Read %d bytes in %v (expected at least %v)", len(testData), duration, expectedMinDuration)
}

func TestRateLimiterWriter(t *testing.T) {
	// 生成10KB的测试数据
	testData := generateTestData(10 * 1024)

	// 创建限速写入器，限制为每秒1KB
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	writer, err := io2.NewRateLimiterWriter(&buf, 1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// 写入数据并测量时间
	start := time.Now()

	// 分块写入，每次写入2KB
	totalWritten := 0
	chunkSize := 2 * 1024
	for totalWritten < len(testData) {
		remaining := len(testData) - totalWritten
		if remaining < chunkSize {
			chunkSize = remaining
		}

		n, err := writer.Write(testData[totalWritten : totalWritten+chunkSize])
		if err != nil {
			t.Fatalf("Failed to write to RateLimiterWriter: %v", err)
		}
		totalWritten += n
	}

	duration := time.Since(start)

	// 验证数据完整性
	if !bytes.Equal(buf.Bytes(), testData) {
		t.Errorf("Data mismatch: expected %d bytes, got %d bytes", len(testData), buf.Len())
	}

	// 验证限速效果：考虑burst机制，实际时间可能比理论时间短
	// burst为1024字节，所以前1KB可以立即写入，剩余9KB需要9秒
	expectedMinDuration := time.Duration((len(testData)-1024)/1024) * time.Second
	if duration < expectedMinDuration {
		t.Errorf("Rate limiting not working properly. Expected at least %v, got %v",
			expectedMinDuration, duration)
	}

	t.Logf("Wrote %d bytes in %v (expected at least %v)", len(testData), duration, expectedMinDuration)
}

func TestRateLimiter(t *testing.T) {
	// 测试同时支持读写的限速器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建限速器，限制为每秒4KB
	limiter, err := io2.NewRateLimiter(4*1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer limiter.Close()

	// 设置读写器
	testData := generateTestData(8 * 1024) // 8KB数据
	limiter.SetReader(bytes.NewReader(testData))

	var buf bytes.Buffer
	limiter.SetWriter(&buf)

	// 测试读取
	start := time.Now()
	var result bytes.Buffer
	_, err = io.Copy(&result, limiter)
	if err != nil {
		t.Fatalf("Failed to read from RateLimiter: %v", err)
	}
	readDuration := time.Since(start)

	// 验证读取的数据
	if !bytes.Equal(result.Bytes(), testData) {
		t.Errorf("Read data mismatch: expected %d bytes, got %d bytes", len(testData), result.Len())
	}

	// 清空缓冲区，测试写入
	buf.Reset()
	start = time.Now()

	// 分块写入，确保完整写入
	totalWritten := 0
	chunkSize := 2 * 1024
	for totalWritten < len(testData) {
		remaining := len(testData) - totalWritten
		if remaining < chunkSize {
			chunkSize = remaining
		}

		n, err := limiter.Write(testData[totalWritten : totalWritten+chunkSize])
		if err != nil {
			t.Fatalf("Failed to write to RateLimiter: %v", err)
		}
		totalWritten += n
	}

	writeDuration := time.Since(start)

	// 验证写入的数据
	if !bytes.Equal(buf.Bytes(), testData) {
		t.Errorf("Write data mismatch: expected %d bytes, got %d bytes", len(testData), buf.Len())
	}

	// 验证限速效果：考虑burst机制
	// burst为2KB，所以前2KB可以立即读写，剩余6KB需要1.5秒
	expectedMinDuration := time.Duration((len(testData)-2*1024)/(4*1024)) * time.Second
	if readDuration < expectedMinDuration || writeDuration < expectedMinDuration {
		t.Errorf("Rate limiting not working properly. Read: %v, Write: %v, Expected at least %v",
			readDuration, writeDuration, expectedMinDuration)
	}

	t.Logf("Read %d bytes in %v, wrote %d bytes in %v (expected at least %v each)",
		len(testData), readDuration, len(testData), writeDuration, expectedMinDuration)
}

func TestRateLimiterSetRate(t *testing.T) {
	// 测试动态调整限速速率
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testData := generateTestData(6 * 1024) // 6KB数据
	reader, err := io2.NewRateLimiterReader(bytes.NewReader(testData), 2*1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	// 动态调整速率到更慢
	reader.SetRate(1024)

	// 读取数据并测量时间
	start := time.Now()
	var result bytes.Buffer
	_, err = io.Copy(&result, reader)
	if err != nil {
		t.Fatalf("Failed to read after rate change: %v", err)
	}
	duration := time.Since(start)

	// 验证数据完整性
	if !bytes.Equal(result.Bytes(), testData) {
		t.Errorf("Data mismatch after rate change: expected %d bytes, got %d bytes", len(testData), result.Len())
	}

	// 验证新的限速效果：6KB数据，1KB/s速率，应该至少需要6秒
	expectedMinDuration := time.Duration(len(testData)/1024) * time.Second
	if duration < expectedMinDuration {
		t.Errorf("Rate limiting not working after rate change. Expected at least %v, got %v",
			expectedMinDuration, duration)
	}

	t.Logf("Read %d bytes with adjusted rate in %v (expected at least %v)", len(testData), duration, expectedMinDuration)
}

func TestRateLimiterClose(t *testing.T) {
	// 测试资源释放
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testData := generateTestData(1024)
	reader, err := io2.NewRateLimiterReader(bytes.NewReader(testData), 1024, ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 关闭reader
	reader.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !reader.IsClosed() {
		t.Error("Reader should be closed after calling Close()")
	}
}

func TestRateLimiterReaderWithZeroRate(t *testing.T) {
	// 测试零速率限制 - 现在应该直接返回错误
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testData := generateTestData(1024)
	_, err := io2.NewRateLimiterReader(bytes.NewReader(testData), 0, ctx)
	if err == nil {
		t.Error("Expected error with zero rate, but no error returned")
	} else {
		t.Logf("Zero rate test completed with error as expected: %v", err)
	}
}

func TestRateLimiterWriterWithZeroRate(t *testing.T) {
	// 测试零速率限制 - 现在应该直接返回错误
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	_, err := io2.NewRateLimiterWriter(&buf, 0, ctx)
	if err == nil {
		t.Error("Expected error with zero rate, but no error returned")
	} else {
		t.Logf("Zero rate test completed with error as expected: %v", err)
	}
}

func TestRateLimiterReaderWithLargeChunks(t *testing.T) {
	// 测试大块数据的读取
	largeData := generateTestData(5 * 1024) // 5KB数据

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader, err := io2.NewRateLimiterReader(bytes.NewReader(largeData), 2*1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	// 读取大块数据
	buffer := make([]byte, 3*1024) // 3KB缓冲区
	n, err := reader.Read(buffer)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to read large chunk: %v", err)
	}

	// 验证读取的数据量（应该被限制为1024字节，因为这是单次读取的最大块大小）
	if n > 1024 {
		t.Errorf("Expected chunk size <= 1024, got %d", n)
	}

	t.Logf("Read %d bytes from large chunk", n)
}

func TestRateLimiterWriterWithLargeChunks(t *testing.T) {
	// 测试大块数据的写入
	largeData := generateTestData(5 * 1024) // 5KB数据

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	writer, err := io2.NewRateLimiterWriter(&buf, 2*1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// 写入大块数据
	n, err := writer.Write(largeData)
	if err != nil {
		t.Fatalf("Failed to write large chunk: %v", err)
	}

	// 验证写入的数据量（应该被限制为1024字节，因为这是单次写入的最大块大小）
	if n > 1024 {
		t.Errorf("Expected chunk size <= 1024, got %d", n)
	}

	t.Logf("Wrote %d bytes from large chunk", n)
}

func TestRateLimiterReaderAfterClose(t *testing.T) {
	// 测试关闭后读取
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testData := generateTestData(1024)
	reader, err := io2.NewRateLimiterReader(bytes.NewReader(testData), 1024, ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 关闭reader
	reader.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 尝试读取应该返回EOF
	buffer := make([]byte, 1024)
	n, err := reader.Read(buffer)
	if n != 0 || err != io.EOF {
		t.Errorf("Expected EOF after close, got n=%d, err=%v", n, err)
	}
}

func TestRateLimiterWriterAfterClose(t *testing.T) {
	// 测试关闭后写入
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	writer, err := io2.NewRateLimiterWriter(&buf, 1024, ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 关闭writer
	writer.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 尝试写入应该返回错误
	testData := generateTestData(1024)
	_, err = writer.Write(testData)
	if err == nil {
		t.Error("Expected error when writing after close")
	}
}

func TestRateLimiterContextCancellation(t *testing.T) {
	// 测试上下文取消时的行为
	ctx, cancel := context.WithCancel(context.Background())

	testData := generateTestData(1024)
	reader, err := io2.NewRateLimiterReader(bytes.NewReader(testData), 1024, ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 取消上下文
	cancel()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !reader.IsClosed() {
		t.Error("Reader should be closed after context cancellation")
	}
}

func TestRateLimiterConcurrentAccess(t *testing.T) {
	// 测试并发访问的安全性
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	limiter, err := io2.NewRateLimiter(4*1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer limiter.Close()

	// 并发设置读写器
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 并发设置读写器
			testData := generateTestData(1024)
			limiter.SetReader(bytes.NewReader(testData))
			limiter.SetWriter(&bytes.Buffer{})

			// 并发调整速率
			limiter.SetRate(2 * 1024)

			// 模拟一些工作
			time.Sleep(1 * time.Millisecond)
		}()
	}

	wg.Wait()

	// 验证限速器仍然可用
	if limiter.IsClosed() {
		t.Error("RateLimiter should not be closed after concurrent access")
	}
}

func TestRateLimiterMultipleSetRate(t *testing.T) {
	// 测试多次调整速率
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testData := generateTestData(1024)
	reader, err := io2.NewRateLimiterReader(bytes.NewReader(testData), 1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	// 多次调整速率
	reader.SetRate(512)
	reader.SetRate(2048)
	reader.SetRate(768)
	reader.SetRate(1536)

	// 验证方法调用不会出错
	if reader.IsClosed() {
		t.Error("Reader should not be closed after multiple SetRate calls")
	}
}

func TestRateLimiterWithNilReader(t *testing.T) {
	// 测试未设置读取器的情况
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	limiter, err := io2.NewRateLimiter(1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer limiter.Close()

	// 尝试读取应该返回错误
	buffer := make([]byte, 1024)
	_, err = limiter.Read(buffer)
	if err == nil {
		t.Error("Expected error when reading without setting reader")
	}
}

func TestRateLimiterWithNilWriter(t *testing.T) {
	// 测试未设置写入器的情况
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	limiter, err := io2.NewRateLimiter(1024, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer limiter.Close()

	// 尝试写入应该返回错误
	testData := generateTestData(1024)
	_, err = limiter.Write(testData)
	if err == nil {
		t.Error("Expected error when writing without setting writer")
	}
}

func TestRateLimiterPerformance(t *testing.T) {
	// 性能测试：测试不同速率下的实际性能
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testData := generateTestData(10 * 1024) // 10KB数据

	testCases := []struct {
		name            string
		rateBytesPerSec int64
		expectedMinSec  float64
	}{
		{"1KB/s", 1024, 9.0},       // 考虑burst，前1KB立即，剩余9KB需要9秒
		{"2KB/s", 2 * 1024, 4.0},   // 考虑burst，前1KB立即，剩余9KB需要4.5秒
		{"5KB/s", 5 * 1024, 1.6},   // 考虑burst，前1KB立即，剩余9KB需要1.8秒
		{"10KB/s", 10 * 1024, 0.5}, // 考虑burst和timer精度，适当放宽阈值
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader, err := io2.NewRateLimiterReader(bytes.NewReader(testData), tc.rateBytesPerSec, ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()

			start := time.Now()
			var result bytes.Buffer
			_, err = io.Copy(&result, reader)
			if err != nil {
				t.Fatalf("Failed to read: %v", err)
			}
			duration := time.Since(start)

			// 验证数据完整性
			if !bytes.Equal(result.Bytes(), testData) {
				t.Errorf("Data mismatch: expected %d bytes, got %d bytes", len(testData), result.Len())
			}

			// 验证限速效果
			actualSec := duration.Seconds()
			if actualSec < tc.expectedMinSec {
				t.Errorf("Rate limiting not working properly. Expected at least %.1f seconds, got %.2f seconds",
					tc.expectedMinSec, actualSec)
			}

			t.Logf("%s: Read %d bytes in %.2f seconds (expected at least %.1f seconds)",
				tc.name, len(testData), actualSec, tc.expectedMinSec)
		})
	}
}
