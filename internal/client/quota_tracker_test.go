package client

import (
	"sync"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
)

// TestGetUserQuota_Cache 测试配额缓存机制
func TestGetUserQuota_Cache(t *testing.T) {
	client := &TunnoxClient{
		quotaCacheMu:    sync.RWMutex{},
		cachedQuota:     nil,
		quotaLastRefresh: time.Time{},
	}

	// 第一次获取 - 应该创建默认配额
	quota1, err := client.GetUserQuota()
	if err != nil {
		t.Fatalf("GetUserQuota failed: %v", err)
	}

	if quota1 == nil {
		t.Fatal("Expected non-nil quota")
	}

	if quota1.MaxConnections != 100 {
		t.Errorf("Expected MaxConnections=100, got %d", quota1.MaxConnections)
	}

	// 第二次获取 - 应该使用缓存（5分钟内）
	quota2, err := client.GetUserQuota()
	if err != nil {
		t.Fatalf("GetUserQuota failed: %v", err)
	}

	// 验证返回的是相同的配额对象（缓存生效）
	if quota1 != quota2 {
		t.Error("Expected cached quota to be returned")
	}
}

// TestGetUserQuota_CacheExpiration 测试配额缓存过期
func TestGetUserQuota_CacheExpiration(t *testing.T) {
	client := &TunnoxClient{
		quotaCacheMu:     sync.RWMutex{},
		cachedQuota:      &models.UserQuota{MaxConnections: 50},
		quotaLastRefresh: time.Now().Add(-6 * time.Minute), // 6分钟前，已过期
	}

	// 获取配额 - 应该刷新缓存
	quota, err := client.GetUserQuota()
	if err != nil {
		t.Fatalf("GetUserQuota failed: %v", err)
	}

	// 验证配额已刷新（默认值）
	if quota.MaxConnections != 100 {
		t.Errorf("Expected refreshed MaxConnections=100, got %d", quota.MaxConnections)
	}
}

// TestTrackTraffic 测试流量追踪
func TestTrackTraffic(t *testing.T) {
	client := &TunnoxClient{
		trafficStatsMu:    sync.RWMutex{},
		localTrafficStats: make(map[string]*localMappingStats),
	}

	mappingID := "test-mapping-1"

	// 第一次追踪流量
	err := client.TrackTraffic(mappingID, 1000, 2000)
	if err != nil {
		t.Fatalf("TrackTraffic failed: %v", err)
	}

	// 验证统计数据
	sent, received := client.GetLocalTrafficStats(mappingID)
	if sent != 1000 {
		t.Errorf("Expected sent=1000, got %d", sent)
	}
	if received != 2000 {
		t.Errorf("Expected received=2000, got %d", received)
	}

	// 第二次追踪流量 - 应该累加
	err = client.TrackTraffic(mappingID, 500, 800)
	if err != nil {
		t.Fatalf("TrackTraffic failed: %v", err)
	}

	sent, received = client.GetLocalTrafficStats(mappingID)
	if sent != 1500 {
		t.Errorf("Expected sent=1500, got %d", sent)
	}
	if received != 2800 {
		t.Errorf("Expected received=2800, got %d", received)
	}
}

// TestTrackTraffic_ZeroTraffic 测试零流量情况
func TestTrackTraffic_ZeroTraffic(t *testing.T) {
	client := &TunnoxClient{
		trafficStatsMu:    sync.RWMutex{},
		localTrafficStats: make(map[string]*localMappingStats),
	}

	mappingID := "test-mapping-2"

	// 追踪零流量
	err := client.TrackTraffic(mappingID, 0, 0)
	if err != nil {
		t.Fatalf("TrackTraffic failed: %v", err)
	}

	// 验证没有创建统计记录
	sent, received := client.GetLocalTrafficStats(mappingID)
	if sent != 0 || received != 0 {
		t.Errorf("Expected zero traffic, got sent=%d, received=%d", sent, received)
	}
}

// TestTrackTraffic_MultipleMapping 测试多个映射的流量追踪
func TestTrackTraffic_MultipleMapping(t *testing.T) {
	client := &TunnoxClient{
		trafficStatsMu:    sync.RWMutex{},
		localTrafficStats: make(map[string]*localMappingStats),
	}

	// 追踪多个映射的流量
	client.TrackTraffic("mapping-1", 1000, 500)
	client.TrackTraffic("mapping-2", 2000, 1000)
	client.TrackTraffic("mapping-3", 3000, 1500)

	// 验证各自独立
	sent1, recv1 := client.GetLocalTrafficStats("mapping-1")
	sent2, recv2 := client.GetLocalTrafficStats("mapping-2")
	sent3, recv3 := client.GetLocalTrafficStats("mapping-3")

	if sent1 != 1000 || recv1 != 500 {
		t.Errorf("mapping-1: expected 1000/500, got %d/%d", sent1, recv1)
	}
	if sent2 != 2000 || recv2 != 1000 {
		t.Errorf("mapping-2: expected 2000/1000, got %d/%d", sent2, recv2)
	}
	if sent3 != 3000 || recv3 != 1500 {
		t.Errorf("mapping-3: expected 3000/1500, got %d/%d", sent3, recv3)
	}
}

// TestGetLocalTrafficStats_NonExistent 测试获取不存在的映射统计
func TestGetLocalTrafficStats_NonExistent(t *testing.T) {
	client := &TunnoxClient{
		trafficStatsMu:    sync.RWMutex{},
		localTrafficStats: make(map[string]*localMappingStats),
	}

	sent, received := client.GetLocalTrafficStats("non-existent")
	if sent != 0 || received != 0 {
		t.Errorf("Expected zero for non-existent mapping, got sent=%d, received=%d", sent, received)
	}
}

// TestTrackTraffic_Concurrent 测试并发流量追踪
func TestTrackTraffic_Concurrent(t *testing.T) {
	client := &TunnoxClient{
		trafficStatsMu:    sync.RWMutex{},
		localTrafficStats: make(map[string]*localMappingStats),
	}

	mappingID := "concurrent-mapping"
	goroutines := 100
	bytesPerGoroutine := int64(100)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// 并发追踪流量
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			client.TrackTraffic(mappingID, bytesPerGoroutine, bytesPerGoroutine)
		}()
	}

	wg.Wait()

	// 验证总流量正确
	sent, received := client.GetLocalTrafficStats(mappingID)
	expected := int64(goroutines) * bytesPerGoroutine

	if sent != expected {
		t.Errorf("Expected sent=%d, got %d", expected, sent)
	}
	if received != expected {
		t.Errorf("Expected received=%d, got %d", expected, received)
	}
}

