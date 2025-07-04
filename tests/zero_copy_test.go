package tests

import (
	"bytes"
	"context"
	"testing"
	"time"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

func TestZeroCopyBuffer_Basic(t *testing.T) {
	// 创建内存池
	pool := utils.NewBufferPool()

	// 分配缓冲区
	data := []byte("hello world")
	buffer := pool.Get(len(data))
	copy(buffer, data)

	// 创建零拷贝缓冲区
	zcb := utils.NewZeroCopyBuffer(buffer, pool)

	// 验证数据
	if zcb.Length() != len(data) {
		t.Errorf("Expected length %d, got %d", len(data), zcb.Length())
	}

	if !bytes.Equal(zcb.Data(), data) {
		t.Errorf("Expected data %v, got %v", data, zcb.Data())
	}

	// 关闭缓冲区，归还内存
	zcb.Close()

	// 验证缓冲区已关闭（通过尝试再次关闭来验证）
	zcb.Close() // 第二次关闭应该不会出错
}

func TestZeroCopyBuffer_Copy(t *testing.T) {
	// 创建内存池
	pool := utils.NewBufferPool()

	// 分配缓冲区
	data := []byte("hello world")
	buffer := pool.Get(len(data))
	copy(buffer, data)

	// 创建零拷贝缓冲区
	zcb := utils.NewZeroCopyBuffer(buffer, pool)

	// 创建副本
	copyData := zcb.Copy()

	// 修改原始数据
	zcb.Data()[0] = 'H'

	// 验证副本不受影响
	if copyData[0] != 'h' {
		t.Errorf("Expected copy to be unaffected, got %c", copyData[0])
	}

	// 关闭缓冲区
	zcb.Close()
}

func TestZeroCopyBuffer_MultipleClose(t *testing.T) {
	// 创建内存池
	pool := utils.NewBufferPool()

	// 分配缓冲区
	data := []byte("test")
	buffer := pool.Get(len(data))
	copy(buffer, data)

	// 创建零拷贝缓冲区
	zcb := utils.NewZeroCopyBuffer(buffer, pool)

	// 多次关闭应该不会出错
	zcb.Close()
	zcb.Close()
	zcb.Close()
}

func TestPackageStream_ReadExactZeroCopy(t *testing.T) {
	// 创建测试数据
	testData := []byte("hello world")
	reader := bytes.NewReader(testData)

	// 创建PackageStream
	ctx := context.Background()
	ps := stream.NewPackageStream(reader, nil, ctx)
	defer ps.Close()

	// 使用零拷贝读取
	zcb, err := ps.ReadExactZeroCopy(len(testData))
	if err != nil {
		t.Fatalf("Failed to read with zero copy: %v", err)
	}
	defer zcb.Close()

	// 验证数据
	if zcb.Length() != len(testData) {
		t.Errorf("Expected length %d, got %d", len(testData), zcb.Length())
	}

	if !bytes.Equal(zcb.Data(), testData) {
		t.Errorf("Expected data %v, got %v", testData, zcb.Data())
	}
}

func TestPackageStream_ReadExactZeroCopy_LargeData(t *testing.T) {
	// 创建大量测试数据
	testData := make([]byte, 1024*1024) // 1MB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	reader := bytes.NewReader(testData)

	// 创建PackageStream
	ctx := context.Background()
	ps := stream.NewPackageStream(reader, nil, ctx)
	defer ps.Close()

	// 分块读取
	chunkSize := 64 * 1024 // 64KB
	totalRead := 0

	for totalRead < len(testData) {
		remaining := len(testData) - totalRead
		if remaining < chunkSize {
			chunkSize = remaining
		}

		zcb, err := ps.ReadExactZeroCopy(chunkSize)
		if err != nil {
			t.Fatalf("Failed to read chunk at offset %d: %v", totalRead, err)
		}

		// 验证数据
		expectedChunk := testData[totalRead : totalRead+chunkSize]
		if !bytes.Equal(zcb.Data(), expectedChunk) {
			t.Errorf("Data mismatch at offset %d", totalRead)
		}

		zcb.Close()
		totalRead += chunkSize
	}
}

func TestTokenBucket_Basic(t *testing.T) {
	ctx := context.Background()

	// 创建令牌桶，速率1000字节/秒
	tb, err := stream.NewTokenBucket(1000, ctx)
	if err != nil {
		t.Fatalf("Failed to create token bucket: %v", err)
	}

	// 等待100字节的令牌
	start := time.Now()
	err = tb.WaitForTokens(100)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to wait for tokens: %v", err)
	}

	// 验证等待时间合理（初始令牌数为0，需要等待产生）
	if duration < 50*time.Millisecond {
		t.Errorf("Expected some wait time, took %v", duration)
	}
}

func TestTokenBucket_RateLimit(t *testing.T) {
	ctx := context.Background()

	// 创建令牌桶，速率100字节/秒
	tb, err := stream.NewTokenBucket(100, ctx)
	if err != nil {
		t.Fatalf("Failed to create token bucket: %v", err)
	}

	// 等待200字节的令牌（超过初始令牌数）
	start := time.Now()
	err = tb.WaitForTokens(200)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to wait for tokens: %v", err)
	}

	// 验证等待时间（应该至少1秒）
	if duration < 900*time.Millisecond {
		t.Errorf("Expected at least 900ms wait, took %v", duration)
	}
}

func TestTokenBucket_SetRate(t *testing.T) {
	ctx := context.Background()

	// 创建令牌桶，初始速率100字节/秒
	tb, err := stream.NewTokenBucket(100, ctx)
	if err != nil {
		t.Fatalf("Failed to create token bucket: %v", err)
	}

	// 验证初始速率
	if tb.GetRate() != 100 {
		t.Errorf("Expected rate 100, got %d", tb.GetRate())
	}

	// 设置新速率
	err = tb.SetRate(200)
	if err != nil {
		t.Fatalf("Failed to set rate: %v", err)
	}

	// 验证新速率
	if tb.GetRate() != 200 {
		t.Errorf("Expected rate 200, got %d", tb.GetRate())
	}

	// 验证突发大小也相应调整
	expectedBurst := int(float64(200) / float64(2)) // DefaultBurstRatio = 2
	actualBurst := tb.GetBurstSize()
	if actualBurst != expectedBurst {
		t.Errorf("Expected burst size %d, got %d", expectedBurst, actualBurst)
	}
}

func TestTokenBucket_InvalidRate(t *testing.T) {
	ctx := context.Background()

	// 测试零速率
	_, err := stream.NewTokenBucket(0, ctx)
	if err == nil {
		t.Error("Expected error for zero rate")
	}

	// 测试负速率
	_, err = stream.NewTokenBucket(-100, ctx)
	if err == nil {
		t.Error("Expected error for negative rate")
	}
}

func TestTokenBucket_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// 创建令牌桶
	tb, err := stream.NewTokenBucket(100, ctx)
	if err != nil {
		t.Fatalf("Failed to create token bucket: %v", err)
	}

	// 启动goroutine等待大量令牌
	errCh := make(chan error, 1)
	go func() {
		errCh <- tb.WaitForTokens(1000)
	}()

	// 取消上下文
	cancel()

	// 等待错误
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Expected error due to context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for context cancellation")
	}
}

func BenchmarkZeroCopyBuffer_Allocation(b *testing.B) {
	pool := utils.NewBufferPool()
	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := pool.Get(len(data))
		zcb := utils.NewZeroCopyBuffer(buffer, pool)
		zcb.Close()
	}
}

func BenchmarkTokenBucket_WaitForTokens(b *testing.B) {
	ctx := context.Background()
	tb, _ := stream.NewTokenBucket(1000000, ctx) // 1MB/s

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.WaitForTokens(1000)
	}
}
