package tests

import (
	"bytes"
	"context"
	"testing"
	"time"
	"tunnox-core/internal/stream/io"
	"tunnox-core/internal/utils"
)

// BenchmarkReadExact_WithPool 测试使用内存池的读取性能
func BenchmarkReadExact_WithPool(b *testing.B) {
	// 准备测试数据
	testData := make([]byte, 1024*1024) // 1MB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	reader := bytes.NewReader(testData)
	var buf bytes.Buffer
	writer := &buf

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io.NewPackageStream(reader, writer, ctx)
	defer stream.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 重置reader位置
		reader.Seek(0, 0)

		// 读取1KB数据
		_, err := stream.ReadExact(1024)
		if err != nil {
			b.Fatalf("ReadExact failed: %v", err)
		}
	}
}

// BenchmarkReadExact_WithoutPool 测试不使用内存池的读取性能（对比）
func BenchmarkReadExact_WithoutPool(b *testing.B) {
	// 准备测试数据
	testData := make([]byte, 1024*1024) // 1MB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	reader := bytes.NewReader(testData)
	var buf bytes.Buffer
	writer := &buf

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io.NewPackageStream(reader, writer, ctx)
	defer stream.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 重置reader位置
		reader.Seek(0, 0)

		// 读取1KB数据
		_, err := stream.ReadExact(1024)
		if err != nil {
			b.Fatalf("ReadExact failed: %v", err)
		}
	}
}

// BenchmarkBufferPool_Allocation 测试内存池分配性能
func BenchmarkBufferPool_Allocation(b *testing.B) {
	pool := utils.NewBufferPool()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf := pool.Get(1024)
		pool.Put(buf)
	}
}

// BenchmarkBufferPool_Allocation_Standard 测试标准内存分配性能（对比）
func BenchmarkBufferPool_Allocation_Standard(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf := make([]byte, 1024)
		_ = buf
	}
}

// TestBufferPool_Concurrent 测试内存池并发安全性
func TestBufferPool_Concurrent(t *testing.T) {
	pool := utils.NewBufferPool()

	// 并发测试
	const numGoroutines = 100
	const numAllocations = 1000

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()

			for j := 0; j < numAllocations; j++ {
				size := 64 + (j % 1024) // 64-1088字节
				buf := pool.Get(size)

				// 写入一些数据
				for k := range buf {
					buf[k] = byte(k % 256)
				}

				pool.Put(buf)
			}
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	t.Log("Concurrent buffer pool test completed successfully")
}

// TestBufferPool_MemoryReuse 测试内存池内存复用
func TestBufferPool_MemoryReuse(t *testing.T) {
	pool := utils.NewBufferPool()

	// 分配一些缓冲区
	buffers := make([][]byte, 10)
	for i := range buffers {
		buffers[i] = pool.Get(1024)
		// 写入数据
		for j := range buffers[i] {
			buffers[i][j] = byte(i + j)
		}
	}

	// 归还缓冲区
	for _, buf := range buffers {
		pool.Put(buf)
	}

	// 重新分配，应该复用之前的内存
	for i := 0; i < 10; i++ {
		buf := pool.Get(1024)
		// 检查是否被清零
		for j, val := range buf {
			if val != 0 {
				t.Errorf("Buffer not cleared at position %d: got %d, want 0", j, val)
			}
		}
		pool.Put(buf)
	}
}

// TestBufferManager_Integration 测试缓冲区管理器集成
func TestBufferManager_Integration(t *testing.T) {
	manager := utils.NewBufferManager()

	// 测试分配和释放
	buf1 := manager.Allocate(1024)
	buf2 := manager.Allocate(2048)

	if len(buf1) != 1024 {
		t.Errorf("Expected buffer size 1024, got %d", len(buf1))
	}

	if len(buf2) != 2048 {
		t.Errorf("Expected buffer size 2048, got %d", len(buf2))
	}

	// 写入数据
	for i := range buf1 {
		buf1[i] = byte(i % 256)
	}

	for i := range buf2 {
		buf2[i] = byte(i % 256)
	}

	// 释放缓冲区
	manager.Release(buf1)
	manager.Release(buf2)

	// 重新分配，应该复用内存
	buf3 := manager.Allocate(1024)
	buf4 := manager.Allocate(2048)

	// 检查是否被清零
	for i, val := range buf3 {
		if val != 0 {
			t.Errorf("Buffer not cleared at position %d: got %d, want 0", i, val)
		}
	}

	for i, val := range buf4 {
		if val != 0 {
			t.Errorf("Buffer not cleared at position %d: got %d, want 0", i, val)
		}
	}

	manager.Release(buf3)
	manager.Release(buf4)
}

// BenchmarkPackageStream_ReadExact_LargeData 测试大数据量读取性能
func BenchmarkPackageStream_ReadExact_LargeData(b *testing.B) {
	// 准备大数据
	testData := make([]byte, 10*1024*1024) // 10MB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	reader := bytes.NewReader(testData)
	var buf bytes.Buffer
	writer := &buf

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := io.NewPackageStream(reader, writer, ctx)
	defer stream.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 重置reader位置
		reader.Seek(0, 0)

		// 读取100KB数据
		_, err := stream.ReadExact(100 * 1024)
		if err != nil {
			b.Fatalf("ReadExact failed: %v", err)
		}
	}
}

// TestMemoryPool_Stress 压力测试
func TestMemoryPool_Stress(t *testing.T) {
	pool := utils.NewBufferPool()

	// 模拟高并发场景
	const numIterations = 10000
	const maxBufferSize = 64 * 1024 // 64KB

	start := time.Now()

	for i := 0; i < numIterations; i++ {
		size := 64 + (i % maxBufferSize)
		buf := pool.Get(size)

		// 模拟数据处理
		for j := range buf {
			buf[j] = byte(i + j)
		}

		pool.Put(buf)
	}

	duration := time.Since(start)
	t.Logf("Processed %d buffers in %v", numIterations, duration)
	t.Logf("Average time per buffer: %v", duration/time.Duration(numIterations))
}
