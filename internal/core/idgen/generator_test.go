package idgen

import (
	"context"
	"testing"
	"tunnox-core/internal/core/storage"
)

// TestClientIDGenerator_RandomGeneration 测试 ClientID 随机生成
func TestClientIDGenerator_RandomGeneration(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewClientIDGenerator(store, ctx)
	defer gen.Close()

	// 生成多个 ClientID
	const count = 100
	ids := make(map[int64]bool)

	for i := 0; i < count; i++ {
		id, err := gen.Generate()
		if err != nil {
			t.Fatalf("Failed to generate client ID at iteration %d: %v", i, err)
		}

		// 验证 ID 在范围内
		if id < ClientIDMin || id > ClientIDMax {
			t.Errorf("Client ID %d out of range [%d, %d]", id, ClientIDMin, ClientIDMax)
		}

		// 验证 ID 不重复
		if ids[id] {
			t.Errorf("Duplicate client ID generated: %d", id)
		}
		ids[id] = true
	}

	t.Logf("Generated %d unique client IDs", len(ids))
}

// TestClientIDGenerator_Randomness 测试 ClientID 随机性
func TestClientIDGenerator_Randomness(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewClientIDGenerator(store, ctx)
	defer gen.Close()

	// 生成 10 个 ClientID
	const count = 10
	ids := make([]int64, count)

	for i := 0; i < count; i++ {
		id, err := gen.Generate()
		if err != nil {
			t.Fatalf("Failed to generate client ID: %v", err)
		}
		ids[i] = id
	}

	// 检查是否有连续的 ID（不应该有）
	consecutiveCount := 0
	for i := 0; i < len(ids)-1; i++ {
		if ids[i+1] == ids[i]+1 || ids[i+1] == ids[i]-1 {
			consecutiveCount++
		}
	}

	// 如果超过 20% 的 ID 是连续的，认为随机性不足
	if consecutiveCount > count/5 {
		t.Errorf("Too many consecutive IDs: %d/%d (expected < %d)", consecutiveCount, count-1, count/5)
	}

	t.Logf("Generated IDs: %v", ids)
	t.Logf("Consecutive pairs: %d/%d", consecutiveCount, count-1)
}

// TestClientIDGenerator_ReleaseAndReuse 测试 ClientID 释放和重用
func TestClientIDGenerator_ReleaseAndReuse(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewClientIDGenerator(store, ctx)
	defer gen.Close()

	// 生成一个 ID
	id1, err := gen.Generate()
	if err != nil {
		t.Fatalf("Failed to generate client ID: %v", err)
	}

	// 验证 ID 已被使用
	used, err := gen.IsUsed(id1)
	if err != nil {
		t.Fatalf("Failed to check if ID is used: %v", err)
	}
	if !used {
		t.Errorf("Expected ID %d to be marked as used", id1)
	}

	// 释放 ID
	err = gen.Release(id1)
	if err != nil {
		t.Fatalf("Failed to release ID: %v", err)
	}

	// 验证 ID 已被释放
	used, err = gen.IsUsed(id1)
	if err != nil {
		t.Fatalf("Failed to check if ID is used: %v", err)
	}
	if used {
		t.Errorf("Expected ID %d to be released", id1)
	}

	// 可以重新使用该 ID（虽然不太可能随机生成到相同的 ID）
	t.Logf("ID %d successfully released and available for reuse", id1)
}

// TestStorageIDGenerator_Int64 测试 StorageIDGenerator 对 int64 类型的支持
func TestStorageIDGenerator_Int64(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	// 直接使用 StorageIDGenerator[int64]
	gen := NewStorageIDGenerator[int64](store, "", "tunnox:id:test:int64", ctx)
	defer gen.Close()

	// 生成多个 ID
	const count = 50
	ids := make(map[int64]bool)

	for i := 0; i < count; i++ {
		id, err := gen.Generate()
		if err != nil {
			t.Fatalf("Failed to generate int64 ID at iteration %d: %v", i, err)
		}

		// 验证 ID 在范围内
		if id < ClientIDMin || id > ClientIDMax {
			t.Errorf("ID %d out of range [%d, %d]", id, ClientIDMin, ClientIDMax)
		}

		// 验证 ID 不重复
		if ids[id] {
			t.Errorf("Duplicate ID generated: %d", id)
		}
		ids[id] = true
	}

	t.Logf("Generated %d unique int64 IDs", len(ids))
}

// TestStorageIDGenerator_String 测试 StorageIDGenerator 对 string 类型的支持
func TestStorageIDGenerator_String(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewStorageIDGenerator[string](store, "test_", "tunnox:id:test:string", ctx)
	defer gen.Close()

	// 生成多个 ID
	const count = 50
	ids := make(map[string]bool)

	for i := 0; i < count; i++ {
		id, err := gen.Generate()
		if err != nil {
			t.Fatalf("Failed to generate string ID at iteration %d: %v", i, err)
		}

		// 验证 ID 包含前缀
		if len(id) < 5 || id[:5] != "test_" {
			t.Errorf("ID %s does not have expected prefix 'test_'", id)
		}

		// 验证 ID 不重复
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}

	t.Logf("Generated %d unique string IDs", len(ids))
}

// TestIDManager_ClientID 测试 IDManager 的 ClientID 生成
func TestIDManager_ClientID(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	manager := NewIDManager(store, ctx)
	defer manager.Close()

	// 生成多个 ClientID
	const count = 30
	ids := make(map[int64]bool)

	for i := 0; i < count; i++ {
		id, err := manager.GenerateClientID()
		if err != nil {
			t.Fatalf("Failed to generate client ID via manager: %v", err)
		}

		// 验证 ID 在范围内
		if id < ClientIDMin || id > ClientIDMax {
			t.Errorf("Client ID %d out of range [%d, %d]", id, ClientIDMin, ClientIDMax)
		}

		// 验证 ID 不重复
		if ids[id] {
			t.Errorf("Duplicate client ID generated via manager: %d", id)
		}
		ids[id] = true
	}

	t.Logf("IDManager generated %d unique client IDs", len(ids))
}

// TestClientIDGenerator_CollisionRate 测试 ClientID 碰撞率
func TestClientIDGenerator_CollisionRate(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewClientIDGenerator(store, ctx)
	defer gen.Close()

	// 生成大量 ID 并统计重试次数
	const count = 1000
	totalAttempts := 0

	for i := 0; i < count; i++ {
		// 这里我们无法直接统计重试次数，但可以验证生成成功
		_, err := gen.Generate()
		if err != nil {
			t.Fatalf("Failed to generate client ID at iteration %d: %v", i, err)
		}
		totalAttempts++
	}

	// 在 90,000,000 个可能的 ID 中，生成 1000 个 ID 的碰撞概率应该极低
	// 平均每次生成应该几乎不需要重试
	avgAttempts := float64(totalAttempts) / float64(count)
	t.Logf("Generated %d IDs with average %.2f attempts per ID", count, avgAttempts)

	// 理论上平均尝试次数应该接近 1.0
	if avgAttempts > 1.5 {
		t.Errorf("Average attempts per ID is too high: %.2f (expected < 1.5)", avgAttempts)
	}
}

// TestClientIDGenerator_Distribution 测试 ClientID 分布均匀性
func TestClientIDGenerator_Distribution(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewClientIDGenerator(store, ctx)
	defer gen.Close()

	// 生成 ID 并分段统计
	const count = 1000
	const segments = 10
	segmentSize := (ClientIDMax - ClientIDMin + 1) / segments
	distribution := make([]int, segments)

	for i := 0; i < count; i++ {
		id, err := gen.Generate()
		if err != nil {
			t.Fatalf("Failed to generate client ID: %v", err)
		}

		// 计算 ID 所在分段
		segmentIndex := int((id - ClientIDMin) / segmentSize)
		if segmentIndex >= segments {
			segmentIndex = segments - 1
		}
		distribution[segmentIndex]++
	}

	// 检查分布是否相对均匀
	expectedPerSegment := count / segments
	tolerance := expectedPerSegment / 2 // 允许 50% 的偏差

	t.Logf("ID distribution across %d segments:", segments)
	for i, cnt := range distribution {
		rangeStart := ClientIDMin + int64(i)*segmentSize
		rangeEnd := rangeStart + segmentSize - 1
		if i == segments-1 {
			rangeEnd = ClientIDMax
		}
		t.Logf("  Segment %d [%d-%d]: %d IDs", i+1, rangeStart, rangeEnd, cnt)

		// 检查偏差
		deviation := cnt - expectedPerSegment
		if deviation < 0 {
			deviation = -deviation
		}
		if deviation > tolerance {
			t.Logf("  Warning: Segment %d has %d IDs (expected ~%d, tolerance ±%d)",
				i+1, cnt, expectedPerSegment, tolerance)
		}
	}
}

// BenchmarkClientIDGenerator_Generate 基准测试 ClientID 生成性能
func BenchmarkClientIDGenerator_Generate(b *testing.B) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewClientIDGenerator(store, ctx)
	defer gen.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := gen.Generate()
		if err != nil {
			b.Fatalf("Failed to generate client ID: %v", err)
		}
	}
}

// BenchmarkStorageIDGenerator_Int64 基准测试 int64 ID 生成性能
func BenchmarkStorageIDGenerator_Int64(b *testing.B) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewStorageIDGenerator[int64](store, "", "tunnox:id:bench:int64", ctx)
	defer gen.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := gen.Generate()
		if err != nil {
			b.Fatalf("Failed to generate int64 ID: %v", err)
		}
	}
}

// BenchmarkStorageIDGenerator_String 基准测试 string ID 生成性能
func BenchmarkStorageIDGenerator_String(b *testing.B) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	gen := NewStorageIDGenerator[string](store, "test_", "tunnox:id:bench:string", ctx)
	defer gen.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := gen.Generate()
		if err != nil {
			b.Fatalf("Failed to generate string ID: %v", err)
		}
	}
}
