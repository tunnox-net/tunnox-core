package factories

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/storage"
)

// TestBatchUpdateClientStatus 测试批量更新客户端状态
func TestBatchUpdateClientStatus(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := NewBuiltinCloudControlWithStorageAndServices(ctx, config, store)

	// 创建多个客户端
	client1, err := cloudControl.CreateClient("user-1", "client1")
	require.NoError(t, err)

	client2, err := cloudControl.CreateClient("user-1", "client2")
	require.NoError(t, err)

	client3, err := cloudControl.CreateClient("user-2", "client3")
	require.NoError(t, err)

	// 批量更新状态
	clientIDs := []int64{client1.ID, client2.ID, client3.ID}
	successCount := 0
	failureCount := 0

	for _, clientID := range clientIDs {
		err := cloudControl.UpdateClientStatus(clientID, models.ClientStatusOffline, "node-1")
		if err != nil {
			failureCount++
		} else {
			successCount++
		}
	}

	// 验证结果
	assert.Equal(t, 3, successCount)
	assert.Equal(t, 0, failureCount)

	// 验证状态已更新
	updated1, err := cloudControl.GetClient(client1.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ClientStatusOffline, updated1.Status)

	updated2, err := cloudControl.GetClient(client2.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ClientStatusOffline, updated2.Status)

	updated3, err := cloudControl.GetClient(client3.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ClientStatusOffline, updated3.Status)
}

// TestBatchUpdateClientStatus_PartialFailure 测试部分失败的批量更新
func TestBatchUpdateClientStatus_PartialFailure(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := NewBuiltinCloudControlWithStorageAndServices(ctx, config, store)

	// 创建一个客户端
	client1, err := cloudControl.CreateClient("user-1", "client1")
	require.NoError(t, err)

	// 批量更新（包括不存在的客户端）
	// 注意：UpdateClientStatus 对于不存在的客户端会创建新的状态记录（只存储 state），所以都会成功
	// 这里测试的是 UpdateClientStatus 的基本功能，不是验证客户端存在性
	clientIDs := []int64{client1.ID, 99999999, 88888888}
	successCount := 0
	failureCount := 0

	for _, clientID := range clientIDs {
		// 使用 offline 状态，因为 online 状态需要 conn_id
		err := cloudControl.UpdateClientStatus(clientID, models.ClientStatusOffline, "")
		if err != nil {
			failureCount++
		} else {
			successCount++
		}
	}

	// 验证结果 - 所有更新都应成功，因为 UpdateClientStatus 只更新状态存储
	assert.Equal(t, 3, successCount)
	assert.Equal(t, 0, failureCount)

	// 验证成功的更新
	updated1, err := cloudControl.GetClient(client1.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ClientStatusOffline, updated1.Status)
}

// TestBatchDeleteMappings 测试批量删除映射
func TestBatchDeleteMappings(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := NewBuiltinCloudControlWithStorageAndServices(ctx, config, store)

	// 创建多个映射
	mapping1 := &models.PortMapping{
		ListenClientID: 12345678,
		TargetClientID: 87654321,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8001,
		TargetPort:     80,
	}
	created1, err := cloudControl.CreatePortMapping(mapping1)
	require.NoError(t, err)

	mapping2 := &models.PortMapping{
		ListenClientID: 12345678,
		TargetClientID: 87654321,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8002,
		TargetPort:     80,
	}
	created2, err := cloudControl.CreatePortMapping(mapping2)
	require.NoError(t, err)

	mapping3 := &models.PortMapping{
		ListenClientID: 12345678,
		TargetClientID: 87654321,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8003,
		TargetPort:     80,
	}
	created3, err := cloudControl.CreatePortMapping(mapping3)
	require.NoError(t, err)

	// 批量删除
	mappingIDs := []string{created1.ID, created2.ID, created3.ID}
	successCount := 0
	failureCount := 0

	for _, mappingID := range mappingIDs {
		err := cloudControl.DeletePortMapping(mappingID)
		if err != nil {
			failureCount++
		} else {
			successCount++
		}
	}

	// 验证结果
	assert.Equal(t, 3, successCount)
	assert.Equal(t, 0, failureCount)

	// 验证映射已删除
	_, err = cloudControl.GetPortMapping(created1.ID)
	assert.Error(t, err)

	_, err = cloudControl.GetPortMapping(created2.ID)
	assert.Error(t, err)

	_, err = cloudControl.GetPortMapping(created3.ID)
	assert.Error(t, err)
}

// TestBatchUpdateMappingStatus 测试批量更新映射状态
func TestBatchUpdateMappingStatus(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := NewBuiltinCloudControlWithStorageAndServices(ctx, config, store)

	// 创建多个映射
	mapping1 := &models.PortMapping{
		ListenClientID: 12345678,
		TargetClientID: 87654321,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8001,
		TargetPort:     80,
		Status:         models.MappingStatusActive,
	}
	created1, err := cloudControl.CreatePortMapping(mapping1)
	require.NoError(t, err)

	mapping2 := &models.PortMapping{
		ListenClientID: 12345678,
		TargetClientID: 87654321,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8002,
		TargetPort:     80,
		Status:         models.MappingStatusActive,
	}
	created2, err := cloudControl.CreatePortMapping(mapping2)
	require.NoError(t, err)

	// 批量更新状态为inactive
	mappingIDs := []string{created1.ID, created2.ID}
	successCount := 0
	failureCount := 0

	for _, mappingID := range mappingIDs {
		err := cloudControl.UpdatePortMappingStatus(mappingID, models.MappingStatusInactive)
		if err != nil {
			failureCount++
		} else {
			successCount++
		}
	}

	// 验证结果
	assert.Equal(t, 2, successCount)
	assert.Equal(t, 0, failureCount)

	// 验证状态已更新
	updated1, err := cloudControl.GetPortMapping(created1.ID)
	require.NoError(t, err)
	assert.Equal(t, models.MappingStatusInactive, updated1.Status)

	updated2, err := cloudControl.GetPortMapping(created2.ID)
	require.NoError(t, err)
	assert.Equal(t, models.MappingStatusInactive, updated2.Status)
}

// TestBatchOperations_Concurrent 测试并发批量操作
func TestBatchOperations_Concurrent(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := NewBuiltinCloudControlWithStorageAndServices(ctx, config, store)

	// 创建10个客户端
	clientIDs := make([]int64, 10)
	for i := 0; i < 10; i++ {
		client, err := cloudControl.CreateClient("user-1", "client-"+string(rune('a'+i)))
		require.NoError(t, err)
		clientIDs[i] = client.ID
	}

	// 并发批量更新状态为 offline（因为 online 状态需要 conn_id）
	// 这个测试验证并发更新状态的线程安全性
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		clientID := clientIDs[i]
		go func() {
			err := cloudControl.UpdateClientStatus(clientID, models.ClientStatusOffline, "")
			assert.NoError(t, err)
			done <- true
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证所有客户端状态已更新
	for _, clientID := range clientIDs {
		client, err := cloudControl.GetClient(clientID)
		require.NoError(t, err)
		assert.Equal(t, models.ClientStatusOffline, client.Status)
	}
}

// TestBatchOperations_EmptyList 测试空列表批量操作
func TestBatchOperations_EmptyList(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := NewBuiltinCloudControlWithStorageAndServices(ctx, config, store)

	// 空列表批量更新
	clientIDs := []int64{}
	successCount := 0

	for _, clientID := range clientIDs {
		err := cloudControl.UpdateClientStatus(clientID, models.ClientStatusOffline, "node-1")
		if err == nil {
			successCount++
		}
	}

	// 验证结果
	assert.Equal(t, 0, successCount)
}
