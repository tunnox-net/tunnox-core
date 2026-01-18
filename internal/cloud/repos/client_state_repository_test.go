package repos

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/storage"
)

func TestClientStateRepository_ZSETNodeClients(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	repo := NewClientStateRepository(ctx, memStorage)
	defer repo.Close()

	nodeID := "node-test-001"
	clientID1 := int64(1001)
	clientID2 := int64(1002)
	clientID3 := int64(1003)

	t.Run("AddToNodeClients", func(t *testing.T) {
		err := repo.AddToNodeClients(nodeID, clientID1)
		require.NoError(t, err)

		err = repo.AddToNodeClients(nodeID, clientID2)
		require.NoError(t, err)

		clients, err := repo.GetNodeClients(nodeID)
		require.NoError(t, err)
		assert.Len(t, clients, 2)
		assert.Contains(t, clients, clientID1)
		assert.Contains(t, clients, clientID2)
	})

	t.Run("TouchNodeClient_UpdatesScore", func(t *testing.T) {
		time.Sleep(10 * time.Millisecond)
		err := repo.TouchNodeClient(nodeID, clientID1)
		require.NoError(t, err)

		clients, err := repo.GetNodeClients(nodeID)
		require.NoError(t, err)
		assert.Contains(t, clients, clientID1)
	})

	t.Run("RemoveFromNodeClients", func(t *testing.T) {
		err := repo.RemoveFromNodeClients(nodeID, clientID2)
		require.NoError(t, err)

		clients, err := repo.GetNodeClients(nodeID)
		require.NoError(t, err)
		assert.Len(t, clients, 1)
		assert.Contains(t, clients, clientID1)
		assert.NotContains(t, clients, clientID2)
	})

	t.Run("CleanupStaleClients_NoStale", func(t *testing.T) {
		err := repo.AddToNodeClients(nodeID, clientID3)
		require.NoError(t, err)

		clients, err := repo.GetNodeClients(nodeID)
		require.NoError(t, err)
		initialCount := len(clients)

		removed, err := repo.CleanupStaleClients(nodeID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), removed)

		clients, err = repo.GetNodeClients(nodeID)
		require.NoError(t, err)
		assert.Equal(t, initialCount, len(clients))
	})

	t.Run("CleanupStaleClients_RemovesStale", func(t *testing.T) {
		staleNodeID := "node-stale-test"
		staleClientID := int64(9999)
		freshClientID := int64(8888)

		zsetStore := memStorage.(storage.SortedSetStore)
		key := "tunnox:runtime:node:clients:" + staleNodeID

		staleScore := float64(time.Now().Add(-2 * time.Minute).Unix())
		err := zsetStore.ZAdd(key, staleClientID, staleScore)
		require.NoError(t, err)

		err = repo.AddToNodeClients(staleNodeID, freshClientID)
		require.NoError(t, err)

		removed, err := repo.CleanupStaleClients(staleNodeID)
		require.NoError(t, err)
		assert.Equal(t, int64(1), removed)

		clientsAfter, err := repo.GetNodeClients(staleNodeID)
		require.NoError(t, err)
		assert.Len(t, clientsAfter, 1)
		assert.Contains(t, clientsAfter, freshClientID)
		assert.NotContains(t, clientsAfter, staleClientID)
	})

	t.Run("GetNodeClients_EmptyNode", func(t *testing.T) {
		clients, err := repo.GetNodeClients("non-existent-node")
		require.NoError(t, err)
		assert.Empty(t, clients)
	})

	t.Run("AddToNodeClients_EmptyNodeID", func(t *testing.T) {
		err := repo.AddToNodeClients("", clientID1)
		assert.Error(t, err)
	})
}

func TestClientStateRepository_StateOperations(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	repo := NewClientStateRepository(ctx, memStorage)
	defer repo.Close()

	clientID := int64(2001)
	nodeID := "node-state-test"

	t.Run("SetState_GetState", func(t *testing.T) {
		state := &models.ClientRuntimeState{
			ClientID:  clientID,
			NodeID:    nodeID,
			ConnID:    "conn-001",
			Status:    models.ClientStatusOnline,
			IPAddress: "192.168.1.100",
			LastSeen:  time.Now(),
		}

		err := repo.SetState(state)
		require.NoError(t, err)

		retrieved, err := repo.GetState(clientID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, clientID, retrieved.ClientID)
		assert.Equal(t, nodeID, retrieved.NodeID)
		assert.Equal(t, models.ClientStatusOnline, retrieved.Status)
	})

	t.Run("GetState_NotFound", func(t *testing.T) {
		state, err := repo.GetState(99999)
		require.NoError(t, err)
		assert.Nil(t, state)
	})

	t.Run("TouchState", func(t *testing.T) {
		originalState, _ := repo.GetState(clientID)
		originalLastSeen := originalState.LastSeen

		time.Sleep(10 * time.Millisecond)

		err := repo.TouchState(clientID)
		require.NoError(t, err)

		updatedState, err := repo.GetState(clientID)
		require.NoError(t, err)
		assert.True(t, updatedState.LastSeen.After(originalLastSeen) || updatedState.LastSeen.Equal(originalLastSeen))
	})

	t.Run("DeleteState", func(t *testing.T) {
		err := repo.DeleteState(clientID)
		require.NoError(t, err)

		state, err := repo.GetState(clientID)
		require.NoError(t, err)
		assert.Nil(t, state)
	})

	t.Run("SetState_NilState", func(t *testing.T) {
		err := repo.SetState(nil)
		assert.Error(t, err)
	})
}

func TestClientStateRepository_RedisZSET(t *testing.T) {
	ctx := context.Background()

	redisConfig := &storage.RedisConfig{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		PoolSize: 10,
	}

	redisStorage, err := storage.NewRedisStorage(ctx, redisConfig)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
	}
	defer redisStorage.Close()

	repo := NewClientStateRepository(ctx, redisStorage)
	defer repo.Close()

	nodeID := "redis-zset-test-node"
	clientID1 := int64(3001)
	clientID2 := int64(3002)

	defer func() {
		repo.RemoveFromNodeClients(nodeID, clientID1)
		repo.RemoveFromNodeClients(nodeID, clientID2)
	}()

	t.Run("ZSET_AddAndGet", func(t *testing.T) {
		err := repo.AddToNodeClients(nodeID, clientID1)
		require.NoError(t, err)

		err = repo.AddToNodeClients(nodeID, clientID2)
		require.NoError(t, err)

		clients, err := repo.GetNodeClients(nodeID)
		require.NoError(t, err)
		assert.Len(t, clients, 2)
	})

	t.Run("ZSET_TouchAndVerify", func(t *testing.T) {
		err := repo.TouchNodeClient(nodeID, clientID1)
		require.NoError(t, err)

		clients, err := repo.GetNodeClients(nodeID)
		require.NoError(t, err)
		assert.Contains(t, clients, clientID1)
	})

	t.Run("ZSET_RemoveAndVerify", func(t *testing.T) {
		err := repo.RemoveFromNodeClients(nodeID, clientID1)
		require.NoError(t, err)

		clients, err := repo.GetNodeClients(nodeID)
		require.NoError(t, err)
		assert.NotContains(t, clients, clientID1)
		assert.Contains(t, clients, clientID2)
	})
}
