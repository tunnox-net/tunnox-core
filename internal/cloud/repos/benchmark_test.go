package repos

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/storage"
)

func newTestClientConfigRepo() *ClientConfigRepository {
	ctx := context.Background()
	stor := storage.NewMemoryStorage(ctx)
	repo := NewRepository(stor)
	return NewClientConfigRepository(repo)
}

func newTestClientStateRepo() *ClientStateRepository {
	ctx := context.Background()
	stor := storage.NewMemoryStorage(ctx)
	return NewClientStateRepository(ctx, stor)
}

func newTestPortMappingRepo() *PortMappingRepo {
	ctx := context.Background()
	stor := storage.NewMemoryStorage(ctx)
	repo := NewRepository(stor)
	return NewPortMappingRepo(repo)
}

func TestClientConfigRepository_MillionConfigs(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过大规模数据测试")
	}

	repo := newTestClientConfigRepo()

	const count = 100_000
	const usersCount = 1000
	const configsPerUser = count / usersCount

	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	start := time.Now()
	for i := 0; i < count; i++ {
		userID := fmt.Sprintf("user-%d", i%usersCount)
		config := &models.ClientConfig{
			ID:       int64(i + 1),
			UserID:   userID,
			Name:     fmt.Sprintf("client-%d", i),
			AuthCode: fmt.Sprintf("auth-%d", i),
			Type:     models.ClientTypeRegistered,
		}
		if err := repo.CreateConfig(config); err != nil {
			t.Fatalf("CreateConfig failed at %d: %v", i, err)
		}
	}
	createTime := time.Since(start)

	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)
	memUsed := m2.Alloc - m1.Alloc

	t.Logf("创建 %d 个客户端配置 (%d 用户):", count, usersCount)
	t.Logf("  耗时: %v", createTime)
	t.Logf("  平均创建: %.2f µs/op", float64(createTime.Microseconds())/float64(count))
	t.Logf("  QPS: %.0f/s", float64(count)/createTime.Seconds())
	t.Logf("  内存使用: %.2f MB", float64(memUsed)/(1024*1024))

	start = time.Now()
	for i := 0; i < count; i++ {
		_, err := repo.GetConfig(int64(i + 1))
		if err != nil {
			t.Fatalf("GetConfig failed at %d: %v", i, err)
		}
	}
	getTime := time.Since(start)

	t.Logf("按 ID 获取 %d 个配置:", count)
	t.Logf("  耗时: %v", getTime)
	t.Logf("  平均获取: %.2f µs/op", float64(getTime.Microseconds())/float64(count))
	t.Logf("  QPS: %.0f/s", float64(count)/getTime.Seconds())

	start = time.Now()
	for i := 0; i < usersCount; i++ {
		userID := fmt.Sprintf("user-%d", i)
		configs, err := repo.ListUserConfigs(userID)
		if err != nil {
			t.Fatalf("ListUserConfigs failed: %v", err)
		}
		if len(configs) != configsPerUser {
			t.Errorf("expected %d configs for user-%d, got %d", configsPerUser, i, len(configs))
		}
	}
	listTime := time.Since(start)

	t.Logf("按用户列出 %d 次 (每次 %d 条):", usersCount, configsPerUser)
	t.Logf("  耗时: %v", listTime)
	t.Logf("  平均列出: %.2f µs/op", float64(listTime.Microseconds())/float64(usersCount))
}

func TestClientStateRepository_TenThousandStates(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过大规模测试")
	}

	repo := newTestClientStateRepo()

	const count = 10_000
	const nodesCount = 10

	start := time.Now()
	for i := 0; i < count; i++ {
		nodeID := fmt.Sprintf("node-%d", i%nodesCount)
		state := &models.ClientRuntimeState{
			ClientID:  int64(i + 1),
			NodeID:    nodeID,
			ConnID:    fmt.Sprintf("conn-%d", i),
			Status:    models.ClientStatusOnline,
			IPAddress: fmt.Sprintf("192.168.%d.%d", i/256, i%256),
		}
		state.Touch()
		if err := repo.SetState(state); err != nil {
			t.Fatalf("SetState failed at %d: %v", i, err)
		}
		if err := repo.AddToNodeClients(nodeID, int64(i+1)); err != nil {
			t.Fatalf("AddToNodeClients failed: %v", err)
		}
	}
	setTime := time.Since(start)

	t.Logf("创建 %d 个客户端状态 (%d 节点):", count, nodesCount)
	t.Logf("  耗时: %v", setTime)
	t.Logf("  平均设置: %.2f µs/op", float64(setTime.Microseconds())/float64(count))

	start = time.Now()
	for i := 0; i < nodesCount; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		clients, err := repo.GetNodeClients(nodeID)
		if err != nil {
			t.Fatalf("GetNodeClients failed: %v", err)
		}
		expected := count / nodesCount
		if len(clients) != expected {
			t.Errorf("expected %d clients for %s, got %d", expected, nodeID, len(clients))
		}
	}
	nodeQueryTime := time.Since(start)

	t.Logf("按节点查询 %d 次:", nodesCount)
	t.Logf("  耗时: %v", nodeQueryTime)
	t.Logf("  平均查询: %.2f µs/op", float64(nodeQueryTime.Microseconds())/float64(nodesCount))

	start = time.Now()
	for i := 0; i < count; i++ {
		if err := repo.TouchState(int64(i + 1)); err != nil {
			t.Fatalf("TouchState failed: %v", err)
		}
	}
	touchTime := time.Since(start)

	t.Logf("Touch %d 个状态:", count)
	t.Logf("  耗时: %v", touchTime)
	t.Logf("  平均 Touch: %.2f µs/op", float64(touchTime.Microseconds())/float64(count))
}

func TestPortMappingRepository_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过大规模测试")
	}

	repo := newTestPortMappingRepo()

	const count = 50_000
	const usersCount = 500

	start := time.Now()
	for i := 0; i < count; i++ {
		userID := fmt.Sprintf("user-%d", i%usersCount)
		mapping := &models.PortMapping{
			ID:             fmt.Sprintf("mapping-%d", i),
			UserID:         userID,
			ListenClientID: int64(i*2 + 1),
			TargetClientID: int64(i*2 + 2),
			Protocol:       models.ProtocolTCP,
			SourcePort:     8000 + i%1000,
			TargetAddress:  fmt.Sprintf("127.0.0.1:%d", 80+i%100),
			Status:         models.MappingStatusActive,
		}
		if err := repo.CreatePortMapping(mapping); err != nil {
			t.Fatalf("CreatePortMapping failed at %d: %v", i, err)
		}
	}
	createTime := time.Since(start)

	t.Logf("创建 %d 个端口映射 (%d 用户):", count, usersCount)
	t.Logf("  耗时: %v", createTime)
	t.Logf("  平均创建: %.2f µs/op", float64(createTime.Microseconds())/float64(count))

	start = time.Now()
	for i := 0; i < usersCount; i++ {
		userID := fmt.Sprintf("user-%d", i)
		mappings, err := repo.GetUserPortMappings(userID)
		if err != nil {
			t.Fatalf("GetUserPortMappings failed: %v", err)
		}
		expected := count / usersCount
		if len(mappings) != expected {
			t.Errorf("expected %d mappings for %s, got %d", expected, userID, len(mappings))
		}
	}
	queryTime := time.Since(start)

	t.Logf("按用户查询 %d 次:", usersCount)
	t.Logf("  耗时: %v", queryTime)
	t.Logf("  平均查询: %.2f µs/op", float64(queryTime.Microseconds())/float64(usersCount))
}

func BenchmarkClientConfigRepository_Create(b *testing.B) {
	repo := newTestClientConfigRepo()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config := &models.ClientConfig{
			ID:       int64(i + 1),
			UserID:   fmt.Sprintf("user-%d", i%100),
			Name:     fmt.Sprintf("client-%d", i),
			AuthCode: fmt.Sprintf("auth-%d", i),
			Type:     models.ClientTypeRegistered,
		}
		_ = repo.CreateConfig(config)
	}
}

func BenchmarkClientConfigRepository_Get(b *testing.B) {
	repo := newTestClientConfigRepo()

	for i := 0; i < 10000; i++ {
		config := &models.ClientConfig{
			ID:       int64(i + 1),
			UserID:   fmt.Sprintf("user-%d", i%100),
			Name:     fmt.Sprintf("client-%d", i),
			AuthCode: fmt.Sprintf("auth-%d", i),
			Type:     models.ClientTypeRegistered,
		}
		_ = repo.CreateConfig(config)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetConfig(int64(i%10000 + 1))
	}
}

func BenchmarkClientConfigRepository_ListUser(b *testing.B) {
	repo := newTestClientConfigRepo()

	for i := 0; i < 10000; i++ {
		config := &models.ClientConfig{
			ID:       int64(i + 1),
			UserID:   fmt.Sprintf("user-%d", i%100),
			Name:     fmt.Sprintf("client-%d", i),
			AuthCode: fmt.Sprintf("auth-%d", i),
			Type:     models.ClientTypeRegistered,
		}
		_ = repo.CreateConfig(config)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.ListUserConfigs(fmt.Sprintf("user-%d", i%100))
	}
}

func BenchmarkClientStateRepository_Set(b *testing.B) {
	repo := newTestClientStateRepo()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := &models.ClientRuntimeState{
			ClientID:  int64(i + 1),
			NodeID:    "node-1",
			ConnID:    fmt.Sprintf("conn-%d", i),
			Status:    models.ClientStatusOnline,
			IPAddress: "192.168.1.100",
		}
		state.Touch()
		_ = repo.SetState(state)
	}
}

func BenchmarkClientStateRepository_Touch(b *testing.B) {
	repo := newTestClientStateRepo()

	for i := 0; i < 10000; i++ {
		state := &models.ClientRuntimeState{
			ClientID:  int64(i + 1),
			NodeID:    "node-1",
			ConnID:    fmt.Sprintf("conn-%d", i),
			Status:    models.ClientStatusOnline,
			IPAddress: "192.168.1.100",
		}
		state.Touch()
		_ = repo.SetState(state)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = repo.TouchState(int64(i%10000 + 1))
	}
}
