package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/storages"
)

func TestOptimizedClientIDGenerator_Basic(t *testing.T) {
	storage := storages.NewMemoryStorage(context.Background())
	defer storage.Close()

	generator := generators.NewOptimizedClientIDGenerator(storage, context.Background())
	defer generator.Close()

	// 等待段数据加载完成
	time.Sleep(100 * time.Millisecond)

	t.Run("Generate Unique IDs", func(t *testing.T) {
		ids := make(map[int64]bool)
		const numIDs = 1000

		for i := 0; i < numIDs; i++ {
			id, err := generator.GenerateClientID()
			if err != nil {
				t.Fatalf("Failed to generate ID %d: %v", i, err)
			}

			// 检查ID范围
			if id < generators.ClientIDMin || id > generators.ClientIDMax {
				t.Errorf("Generated ID %d is out of range [%d, %d]", id, generators.ClientIDMin, generators.ClientIDMax)
			}

			// 检查唯一性
			if ids[id] {
				t.Errorf("Duplicate ID generated: %d", id)
			}
			ids[id] = true
		}

		if len(ids) != numIDs {
			t.Errorf("Expected %d unique IDs, got %d", numIDs, len(ids))
		}
	})

	t.Run("Release and Reuse IDs", func(t *testing.T) {
		// 生成一个ID
		id, err := generator.GenerateClientID()
		if err != nil {
			t.Fatalf("Failed to generate ID: %v", err)
		}

		// 检查ID已使用
		used, err := generator.IsClientIDUsed(id)
		if err != nil {
			t.Fatalf("Failed to check ID usage: %v", err)
		}
		if !used {
			t.Errorf("Generated ID %d should be marked as used", id)
		}

		// 释放ID
		err = generator.ReleaseClientID(id)
		if err != nil {
			t.Fatalf("Failed to release ID: %v", err)
		}

		// 检查ID已释放
		used, err = generator.IsClientIDUsed(id)
		if err != nil {
			t.Fatalf("Failed to check ID usage after release: %v", err)
		}
		if used {
			t.Errorf("Released ID %d should not be marked as used", id)
		}

		// 重新生成相同ID
		newID, err := generator.GenerateClientID()
		if err != nil {
			t.Fatalf("Failed to regenerate ID: %v", err)
		}

		// 验证重新生成的ID可能是相同的（因为随机选择）
		// 这里我们只验证ID在有效范围内
		if newID < generators.ClientIDMin || newID > generators.ClientIDMax {
			t.Errorf("Regenerated ID %d is out of range [%d, %d]", newID, generators.ClientIDMin, generators.ClientIDMax)
		}
	})
}

func TestOptimizedClientIDGenerator_Concurrency(t *testing.T) {
	storage := storages.NewMemoryStorage(context.Background())
	defer storage.Close()

	generator := generators.NewOptimizedClientIDGenerator(storage, context.Background())
	defer generator.Close()

	// 等待段数据加载完成
	time.Sleep(100 * time.Millisecond)

	t.Run("Concurrent Generation", func(t *testing.T) {
		const numGoroutines = 10
		const idsPerGoroutine = 100
		const totalIDs = numGoroutines * idsPerGoroutine

		var wg sync.WaitGroup
		ids := make(chan int64, totalIDs)
		errors := make(chan error, totalIDs)

		// 启动多个goroutine并发生成ID
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(routineID int) {
				defer wg.Done()
				for j := 0; j < idsPerGoroutine; j++ {
					id, err := generator.GenerateClientID()
					if err != nil {
						errors <- fmt.Errorf("routine %d, attempt %d: %v", routineID, j, err)
						return
					}
					ids <- id
				}
			}(i)
		}

		// 等待所有goroutine完成
		wg.Wait()
		close(ids)
		close(errors)

		// 检查错误
		for err := range errors {
			t.Errorf("Generation error: %v", err)
		}

		// 检查生成的ID
		uniqueIDs := make(map[int64]bool)
		count := 0
		for id := range ids {
			count++
			if id < generators.ClientIDMin || id > generators.ClientIDMax {
				t.Errorf("Generated ID %d is out of range [%d, %d]", id, generators.ClientIDMin, generators.ClientIDMax)
			}
			if uniqueIDs[id] {
				t.Errorf("Duplicate ID generated: %d", id)
			}
			uniqueIDs[id] = true
		}

		if count != totalIDs {
			t.Errorf("Expected %d IDs, got %d", totalIDs, count)
		}

		if len(uniqueIDs) != totalIDs {
			t.Errorf("Expected %d unique IDs, got %d", totalIDs, len(uniqueIDs))
		}
	})
}

func TestOptimizedClientIDGenerator_Performance(t *testing.T) {
	storage := storages.NewMemoryStorage(context.Background())
	defer storage.Close()

	generator := generators.NewOptimizedClientIDGenerator(storage, context.Background())
	defer generator.Close()

	// 等待段数据加载完成
	time.Sleep(100 * time.Millisecond)

	t.Run("Generation Performance", func(t *testing.T) {
		const numIDs = 10000
		start := time.Now()

		for i := 0; i < numIDs; i++ {
			_, err := generator.GenerateClientID()
			if err != nil {
				t.Fatalf("Failed to generate ID %d: %v", i, err)
			}
		}

		duration := time.Since(start)
		rate := float64(numIDs) / duration.Seconds()

		t.Logf("Generated %d IDs in %v (%.2f IDs/sec)", numIDs, duration, rate)

		// 性能基准：应该能达到每秒至少1000个ID
		if rate < 1000 {
			t.Errorf("Generation rate too low: %.2f IDs/sec, expected at least 1000", rate)
		}
	})

	t.Run("High Usage Rate Performance", func(t *testing.T) {
		// 先使用部分ID（减少数量避免测试时间过长）
		const prefillCount = 100000 // 使用约1%的ID
		t.Logf("Prefilling with %d IDs...", prefillCount)

		for i := 0; i < prefillCount; i++ {
			_, err := generator.GenerateClientID()
			if err != nil {
				t.Fatalf("Failed to prefill ID %d: %v", i, err)
			}
		}

		// 测试在中等使用率下的性能
		const testCount = 1000
		start := time.Now()

		for i := 0; i < testCount; i++ {
			_, err := generator.GenerateClientID()
			if err != nil {
				t.Fatalf("Failed to generate ID %d under medium usage: %v", i, err)
			}
		}

		duration := time.Since(start)
		rate := float64(testCount) / duration.Seconds()

		t.Logf("Generated %d IDs under medium usage in %v (%.2f IDs/sec)", testCount, duration, rate)

		// 中等使用率下性能应该很好
		if rate < 500 {
			t.Errorf("Generation rate under medium usage too low: %.2f IDs/sec, expected at least 500", rate)
		}
	})
}

func TestOptimizedClientIDGenerator_SegmentStats(t *testing.T) {
	storage := storages.NewMemoryStorage(context.Background())
	defer storage.Close()

	generator := generators.NewOptimizedClientIDGenerator(storage, context.Background())
	defer generator.Close()

	// 等待段数据加载完成
	time.Sleep(100 * time.Millisecond)

	t.Run("Segment Statistics", func(t *testing.T) {
		// 生成一些ID
		const numIDs = 10000
		for i := 0; i < numIDs; i++ {
			_, err := generator.GenerateClientID()
			if err != nil {
				t.Fatalf("Failed to generate ID %d: %v", i, err)
			}
		}

		// 获取段统计信息
		stats := generator.GetSegmentStats()
		if len(stats) == 0 {
			t.Error("Expected segment stats to be available")
		}

		// 验证统计信息
		totalUsage := 0.0
		for segmentID, usageRate := range stats {
			if usageRate < 0 || usageRate > 1 {
				t.Errorf("Invalid usage rate for segment %d: %f", segmentID, usageRate)
			}
			totalUsage += usageRate
		}

		avgUsage := totalUsage / float64(len(stats))
		t.Logf("Average segment usage: %.4f", avgUsage)

		// 验证使用计数
		usedCount := generator.GetUsedCount()
		if usedCount < numIDs {
			t.Errorf("Expected at least %d used IDs, got %d", numIDs, usedCount)
		}
	})
}

func TestOptimizedClientIDGenerator_EdgeCases(t *testing.T) {
	storage := storages.NewMemoryStorage(context.Background())
	defer storage.Close()

	generator := generators.NewOptimizedClientIDGenerator(storage, context.Background())
	defer generator.Close()

	// 等待段数据加载完成
	time.Sleep(100 * time.Millisecond)

	t.Run("Invalid ID Operations", func(t *testing.T) {
		// 测试无效ID范围
		invalidIDs := []int64{
			generators.ClientIDMin - 1,
			generators.ClientIDMax + 1,
			-1,
			0,
		}

		for _, invalidID := range invalidIDs {
			_, err := generator.IsClientIDUsed(invalidID)
			if err == nil {
				t.Errorf("Expected error for invalid ID %d", invalidID)
			}

			err = generator.ReleaseClientID(invalidID)
			if err == nil {
				t.Errorf("Expected error for releasing invalid ID %d", invalidID)
			}
		}
	})

	t.Run("Release Unused ID", func(t *testing.T) {
		// 尝试释放一个未使用的ID
		unusedID := generators.ClientIDMin + 12345
		err := generator.ReleaseClientID(unusedID)
		if err == nil {
			t.Error("Expected error when releasing unused ID")
		}
	})
}

func TestOptimizedClientIDGenerator_Persistence(t *testing.T) {
	storage := storages.NewMemoryStorage(context.Background())

	// 第一个生成器实例
	generator1 := generators.NewOptimizedClientIDGenerator(storage, context.Background())

	// 等待段数据加载完成
	time.Sleep(100 * time.Millisecond)

	// 生成一些ID
	const numIDs = 1000
	ids := make([]int64, numIDs)
	for i := 0; i < numIDs; i++ {
		id, err := generator1.GenerateClientID()
		if err != nil {
			t.Fatalf("Failed to generate ID %d: %v", i, err)
		}
		ids[i] = id
	}

	// 关闭第一个生成器
	generator1.Close()

	// 创建第二个生成器实例（使用相同的存储）
	generator2 := generators.NewOptimizedClientIDGenerator(storage, context.Background())
	defer generator2.Close()

	// 等待段数据加载完成
	time.Sleep(100 * time.Millisecond)

	// 验证之前生成的ID仍然被标记为已使用
	for _, id := range ids {
		used, err := generator2.IsClientIDUsed(id)
		if err != nil {
			t.Fatalf("Failed to check ID %d usage: %v", id, err)
		}
		if !used {
			t.Errorf("Previously generated ID %d should still be marked as used", id)
		}
	}

	// 验证使用计数
	usedCount := generator2.GetUsedCount()
	if usedCount < numIDs {
		t.Errorf("Expected at least %d used IDs after persistence, got %d", numIDs, usedCount)
	}

	storage.Close()
}
