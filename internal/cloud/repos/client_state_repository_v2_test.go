package repos

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/store/memory"
)

// =============================================================================
// 测试辅助：创建带内存存储的 ClientStateRepositoryV2
// =============================================================================

func newTestClientStateRepoV2(ctx context.Context) *ClientStateRepositoryV2 {
	stateStore := memory.NewMemoryStore[string, *models.ClientRuntimeState]()
	nodeClientIndex := memory.NewMemorySetStore[string, string]()

	return NewClientStateRepositoryV2(ClientStateRepoV2Config{
		StateStore:      stateStore,
		NodeClientIndex: nodeClientIndex,
		Ctx:             ctx,
	})
}

// =============================================================================
// ClientStateRepositoryV2 测试
// =============================================================================

func TestClientStateRepositoryV2_SetGetState(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 创建状态
	state := &models.ClientRuntimeState{
		ClientID:  1001,
		NodeID:    "node-1",
		ConnID:    "conn-1",
		Status:    models.ClientStatusOnline,
		IPAddress: "192.168.1.100",
	}
	state.Touch()

	err := repo.SetState(state)
	if err != nil {
		t.Fatalf("SetState failed: %v", err)
	}

	// 获取状态
	got, err := repo.GetState(1001)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if got == nil {
		t.Fatal("expected state, got nil")
	}
	if got.ClientID != 1001 {
		t.Errorf("expected ClientID 1001, got %d", got.ClientID)
	}
	if got.NodeID != "node-1" {
		t.Errorf("expected NodeID 'node-1', got '%s'", got.NodeID)
	}
	if got.Status != models.ClientStatusOnline {
		t.Errorf("expected Status 'online', got '%s'", got.Status)
	}
}

func TestClientStateRepositoryV2_GetStateNotFound(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 获取不存在的状态应返回 nil
	state, err := repo.GetState(9999)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state != nil {
		t.Error("expected nil for non-existent state")
	}
}

func TestClientStateRepositoryV2_DeleteState(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 创建并删除
	state := &models.ClientRuntimeState{
		ClientID:  1001,
		NodeID:    "node-1",
		ConnID:    "conn-1",
		Status:    models.ClientStatusOnline,
		IPAddress: "192.168.1.100",
	}
	state.Touch()
	_ = repo.SetState(state)

	err := repo.DeleteState(1001)
	if err != nil {
		t.Fatalf("DeleteState failed: %v", err)
	}

	// 验证删除
	got, _ := repo.GetState(1001)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestClientStateRepositoryV2_TouchState(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 创建状态
	state := &models.ClientRuntimeState{
		ClientID:  1001,
		NodeID:    "node-1",
		ConnID:    "conn-1",
		Status:    models.ClientStatusOnline,
		IPAddress: "192.168.1.100",
	}
	state.Touch()
	_ = repo.SetState(state)

	// 记录原始心跳时间
	original, _ := repo.GetState(1001)
	originalTime := original.LastSeen

	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)

	// Touch
	err := repo.TouchState(1001)
	if err != nil {
		t.Fatalf("TouchState failed: %v", err)
	}

	// 验证心跳时间更新
	updated, _ := repo.GetState(1001)
	if !updated.LastSeen.After(originalTime) {
		t.Error("expected LastSeen to be updated")
	}
}

func TestClientStateRepositoryV2_NodeClients(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 添加多个客户端到节点
	for i := 1; i <= 5; i++ {
		err := repo.AddToNodeClients("node-1", int64(1000+i))
		if err != nil {
			t.Fatalf("AddToNodeClients failed: %v", err)
		}
	}

	// 添加到另一个节点
	_ = repo.AddToNodeClients("node-2", 2001)
	_ = repo.AddToNodeClients("node-2", 2002)

	// 获取 node-1 的客户端
	clients, err := repo.GetNodeClients("node-1")
	if err != nil {
		t.Fatalf("GetNodeClients failed: %v", err)
	}

	if len(clients) != 5 {
		t.Errorf("expected 5 clients for node-1, got %d", len(clients))
	}

	// 获取 node-2 的客户端
	clients, err = repo.GetNodeClients("node-2")
	if err != nil {
		t.Fatalf("GetNodeClients failed: %v", err)
	}

	if len(clients) != 2 {
		t.Errorf("expected 2 clients for node-2, got %d", len(clients))
	}
}

func TestClientStateRepositoryV2_RemoveFromNodeClients(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 添加客户端
	_ = repo.AddToNodeClients("node-1", 1001)
	_ = repo.AddToNodeClients("node-1", 1002)
	_ = repo.AddToNodeClients("node-1", 1003)

	// 移除一个
	err := repo.RemoveFromNodeClients("node-1", 1002)
	if err != nil {
		t.Fatalf("RemoveFromNodeClients failed: %v", err)
	}

	// 验证
	clients, _ := repo.GetNodeClients("node-1")
	if len(clients) != 2 {
		t.Errorf("expected 2 clients after remove, got %d", len(clients))
	}

	// 验证 1002 被移除
	for _, c := range clients {
		if c == 1002 {
			t.Error("expected 1002 to be removed")
		}
	}
}

func TestClientStateRepositoryV2_CountNodeClients(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 添加客户端
	for i := 1; i <= 10; i++ {
		_ = repo.AddToNodeClients("node-1", int64(1000+i))
	}

	count, err := repo.CountNodeClients("node-1")
	if err != nil {
		t.Fatalf("CountNodeClients failed: %v", err)
	}

	if count != 10 {
		t.Errorf("expected count 10, got %d", count)
	}

	// 不存在的节点应返回 0
	count, err = repo.CountNodeClients("node-nonexistent")
	if err != nil {
		t.Fatalf("CountNodeClients failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 for non-existent node, got %d", count)
	}
}

func TestClientStateRepositoryV2_BatchGetStates(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 创建多个状态
	for i := 1; i <= 5; i++ {
		state := &models.ClientRuntimeState{
			ClientID:  int64(1000 + i),
			NodeID:    "node-1",
			ConnID:    "conn-" + string(rune('0'+i)),
			Status:    models.ClientStatusOnline,
			IPAddress: "192.168.1." + string(rune('0'+i)),
		}
		state.Touch()
		_ = repo.SetState(state)
	}

	// 批量获取
	states, err := repo.BatchGetStates([]int64{1001, 1003, 1005, 9999})
	if err != nil {
		t.Fatalf("BatchGetStates failed: %v", err)
	}

	if len(states) != 3 {
		t.Errorf("expected 3 states, got %d", len(states))
	}

	// 验证
	if _, ok := states[1001]; !ok {
		t.Error("expected state 1001 in result")
	}
	if _, ok := states[1003]; !ok {
		t.Error("expected state 1003 in result")
	}
	if _, ok := states[1005]; !ok {
		t.Error("expected state 1005 in result")
	}
}

func TestClientStateRepositoryV2_GetOnlineClientsForNode(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 创建状态并添加到节点
	for i := 1; i <= 3; i++ {
		state := &models.ClientRuntimeState{
			ClientID:  int64(1000 + i),
			NodeID:    "node-1",
			ConnID:    "conn-" + string(rune('0'+i)),
			Status:    models.ClientStatusOnline,
			IPAddress: "192.168.1." + string(rune('0'+i)),
		}
		state.Touch()
		_ = repo.SetState(state)
		_ = repo.AddToNodeClients("node-1", int64(1000+i))
	}

	// 获取在线客户端
	states, err := repo.GetOnlineClientsForNode("node-1")
	if err != nil {
		t.Fatalf("GetOnlineClientsForNode failed: %v", err)
	}

	if len(states) != 3 {
		t.Errorf("expected 3 online clients, got %d", len(states))
	}
}

func TestClientStateRepositoryV2_EmptyNodeClients(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 获取空节点的客户端
	clients, err := repo.GetNodeClients("empty-node")
	if err != nil {
		t.Fatalf("GetNodeClients failed: %v", err)
	}

	if len(clients) != 0 {
		t.Errorf("expected 0 clients for empty node, got %d", len(clients))
	}
}

func TestClientStateRepositoryV2_ValidationError(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 无效状态（ClientID 为 0）
	state := &models.ClientRuntimeState{
		ClientID:  0,
		NodeID:    "node-1",
		ConnID:    "conn-test",
		Status:    models.ClientStatusOnline,
		IPAddress: "192.168.1.100",
	}

	err := repo.SetState(state)
	if err == nil {
		t.Error("expected validation error for ClientID=0")
	}
}

func TestClientStateRepositoryV2_NilState(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	err := repo.SetState(nil)
	if err == nil {
		t.Error("expected error for nil state")
	}
}

func TestClientStateRepositoryV2_EmptyNodeID(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientStateRepoV2(ctx)

	// 空 nodeID 应返回错误
	err := repo.AddToNodeClients("", 1001)
	if err == nil {
		t.Error("expected error for empty nodeID")
	}

	// 空 nodeID 的移除应该不报错（静默忽略）
	err = repo.RemoveFromNodeClients("", 1001)
	if err != nil {
		t.Errorf("RemoveFromNodeClients with empty nodeID should not error: %v", err)
	}
}
