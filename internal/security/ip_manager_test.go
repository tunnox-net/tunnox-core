package security

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/core/storage"
)

func TestIPManager_Blacklist(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewIPManager(memStorage, ctx)

	ip := "192.168.1.100"

	// 添加到黑名单
	err := manager.AddToBlacklist(ip, 10*time.Minute, "test ban", "admin")
	require.NoError(t, err, "Failed to add to blacklist")

	// 检查是否被封禁
	allowed, reason := manager.IsAllowed(ip)
	assert.False(t, allowed, "IP should be blocked")
	assert.Contains(t, reason, "test ban")

	// 从黑名单移除
	manager.RemoveFromBlacklist(ip)

	// 应该允许访问
	allowed, _ = manager.IsAllowed(ip)
	assert.True(t, allowed, "IP should be allowed after removal")
}

func TestIPManager_Whitelist(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewIPManager(memStorage, ctx)

	ip := "192.168.1.100"

	// 先添加到黑名单
	err := manager.AddToBlacklist(ip, 10*time.Minute, "test ban", "admin")
	require.NoError(t, err, "Failed to add to blacklist")

	// 检查被封禁
	allowed, _ := manager.IsAllowed(ip)
	assert.False(t, allowed, "IP should be blocked")

	// 添加到白名单
	err = manager.AddToWhitelist(ip, "trusted IP", "admin")
	require.NoError(t, err, "Failed to add to whitelist")

	// 白名单优先级高于黑名单，应该允许
	allowed, _ = manager.IsAllowed(ip)
	assert.True(t, allowed, "Whitelisted IP should be allowed even if blacklisted")
}

func TestIPManager_CIDR(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewIPManager(memStorage, ctx)

	cidr := "192.168.1.0/24"

	// 添加整个网段到黑名单
	err := manager.AddToBlacklist(cidr, 0, "ban entire subnet", "admin")
	require.NoError(t, err, "Failed to add CIDR to blacklist")

	// 检查网段内的IP
	testIPs := []string{
		"192.168.1.1",
		"192.168.1.100",
		"192.168.1.255",
	}

	for _, ip := range testIPs {
		allowed, _ := manager.IsAllowed(ip)
		assert.False(t, allowed, "IP %s in subnet should be blocked", ip)
	}

	// 网段外的IP应该允许
	allowed, _ := manager.IsAllowed("192.168.2.1")
	assert.True(t, allowed, "IP outside subnet should be allowed")
}

func TestIPManager_TemporaryBlacklist(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewIPManager(memStorage, ctx)

	ip := "192.168.1.100"

	// 添加到黑名单，1秒后过期
	err := manager.AddToBlacklist(ip, 1*time.Second, "temp ban", "admin")
	require.NoError(t, err, "Failed to add to blacklist")

	// 应该被封禁
	allowed, _ := manager.IsAllowed(ip)
	assert.False(t, allowed, "IP should be blocked")

	// 等待过期
	time.Sleep(1500 * time.Millisecond)

	// 应该自动解封
	allowed, _ = manager.IsAllowed(ip)
	assert.True(t, allowed, "IP should be allowed after expiration")
}

func TestIPManager_GetBlacklist(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewIPManager(memStorage, ctx)

	// 添加多个IP到黑名单
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}
	for _, ip := range ips {
		err := manager.AddToBlacklist(ip, 10*time.Minute, "test", "admin")
		require.NoError(t, err)
	}

	// 获取黑名单
	list := manager.GetBlacklist()
	assert.Len(t, list, 3, "Should have 3 blacklist entries")

	// 验证内容
	ipSet := make(map[string]bool)
	for _, record := range list {
		ipSet[record.IP] = true
	}
	for _, ip := range ips {
		assert.True(t, ipSet[ip], "IP %s should be in blacklist", ip)
	}
}

func TestIPManager_GetWhitelist(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewIPManager(memStorage, ctx)

	// 添加多个IP到白名单
	ips := []string{"10.0.0.1", "10.0.0.2"}
	for _, ip := range ips {
		err := manager.AddToWhitelist(ip, "trusted", "admin")
		require.NoError(t, err)
	}

	// 获取白名单
	list := manager.GetWhitelist()
	assert.Len(t, list, 2, "Should have 2 whitelist entries")
}

func TestIPManager_InvalidIP(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewIPManager(memStorage, ctx)

	// 无效的IP
	err := manager.AddToBlacklist("invalid-ip", 10*time.Minute, "test", "admin")
	assert.Error(t, err, "Should reject invalid IP")

	// 无效的CIDR
	err = manager.AddToBlacklist("192.168.1.0/99", 10*time.Minute, "test", "admin")
	assert.Error(t, err, "Should reject invalid CIDR")
}

func TestIPManager_GetStats(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewIPManager(memStorage, ctx)

	// 添加一些IP
	manager.AddToBlacklist("192.168.1.1", 10*time.Minute, "test", "admin")
	manager.AddToBlacklist("192.168.1.2", 10*time.Minute, "test", "admin")
	manager.AddToWhitelist("10.0.0.1", "trusted", "admin")

	stats := manager.GetStats()
	assert.Equal(t, 2, stats.BlacklistCount, "Should have 2 blacklist entries")
	assert.Equal(t, 1, stats.WhitelistCount, "Should have 1 whitelist entry")
}
