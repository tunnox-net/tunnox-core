package security

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBruteForceProtector_RecordFailure(t *testing.T) {
	config := &BruteForceConfig{
		MaxFailures:     3,
		TimeWindow:      5 * time.Minute,
		BanDuration:     10 * time.Minute,
		PermanentBanAt:  10,
		CleanupInterval: 1 * time.Minute,
	}

	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	ip := "192.168.1.100"

	// 第1次失败，不应封禁
	shouldBan := protector.RecordFailure(ip)
	assert.False(t, shouldBan, "First failure should not ban")
	assert.Equal(t, 1, protector.GetFailureCount(ip))

	// 第2次失败，不应封禁
	shouldBan = protector.RecordFailure(ip)
	assert.False(t, shouldBan, "Second failure should not ban")
	assert.Equal(t, 2, protector.GetFailureCount(ip))

	// 第3次失败，应该封禁
	shouldBan = protector.RecordFailure(ip)
	assert.True(t, shouldBan, "Third failure should ban")

	// 检查是否被封禁
	banned, reason := protector.IsBanned(ip)
	assert.True(t, banned, "IP should be banned")
	assert.Contains(t, reason, "失败 3 次")
}

func TestBruteForceProtector_RecordSuccess(t *testing.T) {
	config := DefaultBruteForceConfig()
	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	ip := "192.168.1.100"

	// 记录2次失败
	protector.RecordFailure(ip)
	protector.RecordFailure(ip)
	assert.Equal(t, 2, protector.GetFailureCount(ip))

	// 记录成功，应该清除失败记录
	protector.RecordSuccess(ip)
	assert.Equal(t, 0, protector.GetFailureCount(ip))
}

func TestBruteForceProtector_PermanentBan(t *testing.T) {
	config := &BruteForceConfig{
		MaxFailures:     3,
		TimeWindow:      5 * time.Minute,
		BanDuration:     10 * time.Minute,
		PermanentBanAt:  5, // 5次永久封禁
		CleanupInterval: 1 * time.Minute,
	}

	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	ip := "192.168.1.100"

	// 记录5次失败，应该永久封禁
	for i := 0; i < 5; i++ {
		protector.RecordFailure(ip)

		// 每3次会触发临时封禁，清除临时封禁继续测试
		if (i+1)%3 == 0 && i < 4 {
			protector.UnbanIP(ip)
		}
	}

	// 检查是否永久封禁
	banned, reason := protector.IsBanned(ip)
	assert.True(t, banned, "IP should be permanently banned")
	assert.Contains(t, reason, "累计失败")

	// 获取封禁记录
	bannedIPs := protector.GetBannedIPs()
	require.Len(t, bannedIPs, 1)
	assert.True(t, bannedIPs[0].ExpiresAt.IsZero(), "Should be permanent ban")
}

func TestBruteForceProtector_TemporaryBan(t *testing.T) {
	config := &BruteForceConfig{
		MaxFailures:     3,
		TimeWindow:      5 * time.Minute,
		BanDuration:     1 * time.Second, // 1秒后解封
		PermanentBanAt:  10,
		CleanupInterval: 500 * time.Millisecond,
	}

	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	ip := "192.168.1.100"

	// 记录3次失败，触发临时封禁
	for i := 0; i < 3; i++ {
		protector.RecordFailure(ip)
	}

	// 应该被封禁
	banned, _ := protector.IsBanned(ip)
	assert.True(t, banned, "IP should be banned")

	// 等待封禁过期
	time.Sleep(1500 * time.Millisecond)

	// 应该自动解封
	banned, _ = protector.IsBanned(ip)
	assert.False(t, banned, "IP should be unbanned after expiration")
}

func TestBruteForceProtector_TimeWindow(t *testing.T) {
	config := &BruteForceConfig{
		MaxFailures:     3,
		TimeWindow:      1 * time.Second, // 1秒时间窗口
		BanDuration:     10 * time.Minute,
		PermanentBanAt:  10,
		CleanupInterval: 500 * time.Millisecond,
	}

	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	ip := "192.168.1.100"

	// 记录2次失败
	protector.RecordFailure(ip)
	protector.RecordFailure(ip)
	assert.Equal(t, 2, protector.GetFailureCount(ip))

	// 等待时间窗口过期
	time.Sleep(1500 * time.Millisecond)

	// 触发清理（通过记录新失败）
	protector.RecordFailure(ip)

	// 由于前2次已过期，现在只有1次失败
	assert.Equal(t, 1, protector.GetFailureCount(ip))
}

func TestBruteForceProtector_ManualBanUnban(t *testing.T) {
	config := DefaultBruteForceConfig()
	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	ip := "192.168.1.100"

	// 手动封禁
	protector.BanIP(ip, 10*time.Minute, "manual ban for testing")

	banned, reason := protector.IsBanned(ip)
	assert.True(t, banned, "IP should be banned")
	assert.Contains(t, reason, "manual ban")

	// 手动解封
	protector.UnbanIP(ip)

	banned, _ = protector.IsBanned(ip)
	assert.False(t, banned, "IP should be unbanned")
}

func TestBruteForceProtector_GetStats(t *testing.T) {
	config := &BruteForceConfig{
		MaxFailures:     3,
		TimeWindow:      5 * time.Minute,
		BanDuration:     10 * time.Minute,
		PermanentBanAt:  5,
		CleanupInterval: 1 * time.Minute,
	}

	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	// 添加一些失败记录
	protector.RecordFailure("192.168.1.100")
	protector.RecordFailure("192.168.1.101")

	// 封禁一些IP
	protector.BanIP("192.168.1.200", 10*time.Minute, "temp ban")
	protector.BanIP("192.168.1.201", 0, "permanent ban")

	stats := protector.GetStats()
	assert.Equal(t, 2, stats.TotalFailureRecords, "Should have 2 failure records")
	assert.Equal(t, 2, stats.TotalBannedIPs, "Should have 2 banned IPs")
	assert.Equal(t, 1, stats.TemporaryBans, "Should have 1 temporary ban")
	assert.Equal(t, 1, stats.PermanentBans, "Should have 1 permanent ban")
}

func TestBruteForceProtector_ConcurrentAccess(t *testing.T) {
	config := DefaultBruteForceConfig()
	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	// 并发记录失败
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			ip := "192.168.1.100"
			protector.RecordFailure(ip)
			protector.IsBanned(ip)
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 应该没有panic，且有失败记录
	count := protector.GetFailureCount("192.168.1.100")
	assert.Greater(t, count, 0, "Should have failure records")
}

func TestBruteForceProtector_Reset(t *testing.T) {
	config := DefaultBruteForceConfig()
	protector := NewBruteForceProtector(config, context.Background())
	// No explicit disposal needed for test

	ip := "192.168.1.100"

	// 添加失败记录和封禁
	protector.RecordFailure(ip)
	protector.BanIP("192.168.1.200", 10*time.Minute, "test")

	assert.Equal(t, 1, protector.GetFailureCount(ip))
	assert.Equal(t, 1, len(protector.GetBannedIPs()))

	// 重置
	protector.Reset()

	assert.Equal(t, 0, protector.GetFailureCount(ip))
	assert.Equal(t, 0, len(protector.GetBannedIPs()))
}
