package memory

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
)

// =============================================================================
// 百万数据性能测试
// =============================================================================

// TestMemoryStore_MillionEntries 测试百万级数据存储
func TestMemoryStore_MillionEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过百万数据测试（使用 -short 标志）")
	}

	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	const count = 1_000_000

	// 记录初始内存
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 写入百万数据
	start := time.Now()
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		if err := s.Set(ctx, key, value); err != nil {
			t.Fatalf("Set failed at %d: %v", i, err)
		}
	}
	writeTime := time.Since(start)

	// 记录写入后内存
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)
	memUsed := m2.Alloc - m1.Alloc

	t.Logf("写入 %d 条数据:", count)
	t.Logf("  耗时: %v", writeTime)
	t.Logf("  平均写入: %.2f µs/op", float64(writeTime.Microseconds())/float64(count))
	t.Logf("  QPS: %.0f/s", float64(count)/writeTime.Seconds())
	t.Logf("  内存使用: %.2f MB", float64(memUsed)/(1024*1024))
	t.Logf("  平均每条: %.0f bytes", float64(memUsed)/float64(count))

	// 随机读取测试
	start = time.Now()
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("key-%d", i)
		_, err := s.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed at %d: %v", i, err)
		}
	}
	readTime := time.Since(start)

	t.Logf("读取 %d 条数据:", count)
	t.Logf("  耗时: %v", readTime)
	t.Logf("  平均读取: %.2f µs/op", float64(readTime.Microseconds())/float64(count))
	t.Logf("  QPS: %.0f/s", float64(count)/readTime.Seconds())

	// 验证性能指标
	if writeTime.Seconds() > 30 {
		t.Errorf("写入百万数据超过 30 秒: %v", writeTime)
	}
	if readTime.Seconds() > 10 {
		t.Errorf("读取百万数据超过 10 秒: %v", readTime)
	}
}

// TestMemoryStore_MillionEntriesWithTTL 测试带 TTL 的百万数据
func TestMemoryStore_MillionEntriesWithTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过百万数据测试（使用 -short 标志）")
	}

	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	const count = 1_000_000

	start := time.Now()
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		ttl := time.Duration(10+i%60) * time.Minute // 10-70 分钟的 TTL
		if err := s.SetWithTTL(ctx, key, value, ttl); err != nil {
			t.Fatalf("SetWithTTL failed at %d: %v", i, err)
		}
	}
	writeTime := time.Since(start)

	t.Logf("写入 %d 条带 TTL 数据:", count)
	t.Logf("  耗时: %v", writeTime)
	t.Logf("  平均写入: %.2f µs/op", float64(writeTime.Microseconds())/float64(count))
}

// TestMemorySetStore_LargeSet 测试大型集合
func TestMemorySetStore_LargeSet(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过大型集合测试（使用 -short 标志）")
	}

	ctx := context.Background()
	s := NewMemorySetStore[string, string]()
	defer s.Close()

	const setCount = 1000      // 1000 个集合
	const membersPerSet = 1000 // 每个集合 1000 个成员

	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 添加成员
	start := time.Now()
	for i := 0; i < setCount; i++ {
		setKey := fmt.Sprintf("set-%d", i)
		for j := 0; j < membersPerSet; j++ {
			member := fmt.Sprintf("member-%d", j)
			if err := s.Add(ctx, setKey, member); err != nil {
				t.Fatalf("Add failed: %v", err)
			}
		}
	}
	writeTime := time.Since(start)

	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)
	memUsed := m2.Alloc - m1.Alloc

	totalMembers := setCount * membersPerSet
	t.Logf("创建 %d 个集合，每个 %d 成员 (总计 %d):", setCount, membersPerSet, totalMembers)
	t.Logf("  耗时: %v", writeTime)
	t.Logf("  平均添加: %.2f µs/op", float64(writeTime.Microseconds())/float64(totalMembers))
	t.Logf("  内存使用: %.2f MB", float64(memUsed)/(1024*1024))

	// 查询集合成员
	start = time.Now()
	for i := 0; i < setCount; i++ {
		setKey := fmt.Sprintf("set-%d", i)
		members, err := s.Members(ctx, setKey)
		if err != nil {
			t.Fatalf("Members failed: %v", err)
		}
		if len(members) != membersPerSet {
			t.Errorf("expected %d members, got %d", membersPerSet, len(members))
		}
	}
	queryTime := time.Since(start)

	t.Logf("查询 %d 个集合:", setCount)
	t.Logf("  耗时: %v", queryTime)
	t.Logf("  平均查询: %.2f µs/op", float64(queryTime.Microseconds())/float64(setCount))
}

// =============================================================================
// 基准测试
// =============================================================================

func BenchmarkMemoryStore_Set(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		_ = s.Set(ctx, key, "value")
	}
}

func BenchmarkMemoryStore_Get(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// 预填充数据
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("key-%d", i)
		_ = s.Set(ctx, key, "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%10000)
		_, _ = s.Get(ctx, key)
	}
}

func BenchmarkMemoryStore_SetWithTTL(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		_ = s.SetWithTTL(ctx, key, "value", time.Hour)
	}
}

func BenchmarkMemoryStore_BatchSet(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// 准备批量数据
	items := make(map[string]string)
	for i := 0; i < 100; i++ {
		items[fmt.Sprintf("key-%d", i)] = "value"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.BatchSet(ctx, items)
	}
}

func BenchmarkMemoryStore_BatchGet(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// 预填充数据
	keys := make([]string, 100)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		keys[i] = key
		_ = s.Set(ctx, key, "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.BatchGet(ctx, keys)
	}
}

func BenchmarkMemorySetStore_Add(b *testing.B) {
	ctx := context.Background()
	s := NewMemorySetStore[string, string]()
	defer s.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		member := fmt.Sprintf("member-%d", i)
		_ = s.Add(ctx, "set", member)
	}
}

func BenchmarkMemorySetStore_Members(b *testing.B) {
	ctx := context.Background()
	s := NewMemorySetStore[string, string]()
	defer s.Close()

	// 预填充
	for i := 0; i < 1000; i++ {
		_ = s.Add(ctx, "set", fmt.Sprintf("member-%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Members(ctx, "set")
	}
}

func BenchmarkMemoryStore_Concurrent_Set(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i)
			_ = s.Set(ctx, key, "value")
			i++
		}
	})
}

func BenchmarkMemoryStore_Concurrent_Get(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// 预填充
	for i := 0; i < 10000; i++ {
		_ = s.Set(ctx, fmt.Sprintf("key-%d", i), "value")
	}

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = s.Get(ctx, fmt.Sprintf("key-%d", i%10000))
			i++
		}
	})
}
