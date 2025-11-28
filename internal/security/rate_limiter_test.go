package security

import (
	"context"
	"testing"
	"time"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_IPRateLimit(t *testing.T) {
	config := &RateLimitConfig{
		Rate:  2,                // 每秒2个连接
		Burst: 5,                // 最多突发5个
		TTL:   5 * time.Minute,
	}
	
	limiter := NewRateLimiter(config, nil, context.Background())
	
	ip := "192.168.1.100"
	
	// 前5个应该成功（突发容量）
	for i := 0; i < 5; i++ {
		allowed := limiter.AllowIP(ip)
		assert.True(t, allowed, "Connection %d should be allowed (burst)", i+1)
	}
	
	// 第6个应该失败（超过突发）
	allowed := limiter.AllowIP(ip)
	assert.False(t, allowed, "Connection 6 should be blocked (exceeded burst)")
	
	// 等待0.5秒，应该补充1个令牌
	time.Sleep(500 * time.Millisecond)
	allowed = limiter.AllowIP(ip)
	assert.True(t, allowed, "Should allow after refill")
}

func TestRateLimiter_TunnelRateLimit(t *testing.T) {
	config := &RateLimitConfig{
		Rate:  1000,             // 每秒1000字节
		Burst: 2000,             // 最多突发2000字节
		TTL:   5 * time.Minute,
	}
	
	limiter := NewRateLimiter(nil, config, context.Background())
	
	tunnelID := "tunnel-123"
	
	// 传输1500字节，应该成功
	allowed := limiter.AllowTunnel(tunnelID, 1500)
	assert.True(t, allowed, "Should allow 1500 bytes")
	
	// 再传输1000字节，应该失败（超过突发）
	allowed = limiter.AllowTunnel(tunnelID, 1000)
	assert.False(t, allowed, "Should block 1000 bytes (exceeded burst)")
	
	// 等待1秒，应该补充1000字节令牌
	time.Sleep(1 * time.Second)
	allowed = limiter.AllowTunnel(tunnelID, 1000)
	assert.True(t, allowed, "Should allow 1000 bytes after refill")
}

func TestRateLimiter_MultipleIPs(t *testing.T) {
	config := &RateLimitConfig{
		Rate:  2,
		Burst: 3,
		TTL:   5 * time.Minute,
	}
	
	limiter := NewRateLimiter(config, nil, context.Background())
	
	ip1 := "192.168.1.100"
	ip2 := "192.168.1.101"
	
	// IP1消耗3个令牌
	for i := 0; i < 3; i++ {
		allowed := limiter.AllowIP(ip1)
		assert.True(t, allowed, "IP1 connection %d should be allowed", i+1)
	}
	
	// IP2应该有自己独立的限制
	for i := 0; i < 3; i++ {
		allowed := limiter.AllowIP(ip2)
		assert.True(t, allowed, "IP2 connection %d should be allowed", i+1)
	}
	
	// IP1和IP2都应该被限制
	assert.False(t, limiter.AllowIP(ip1), "IP1 should be blocked")
	assert.False(t, limiter.AllowIP(ip2), "IP2 should be blocked")
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	config := &RateLimitConfig{
		Rate:  10,               // 每秒10个令牌
		Burst: 10,
		TTL:   5 * time.Minute,
	}
	
	limiter := NewRateLimiter(config, nil, context.Background())
	
	ip := "192.168.1.100"
	
	// 消耗所有令牌
	for i := 0; i < 10; i++ {
		limiter.AllowIP(ip)
	}
	
	// 应该被限制
	assert.False(t, limiter.AllowIP(ip), "Should be blocked after consuming all tokens")
	
	// 等待0.5秒，应该补充5个令牌
	time.Sleep(500 * time.Millisecond)
	
	// 应该允许5个连接
	successCount := 0
	for i := 0; i < 10; i++ {
		if limiter.AllowIP(ip) {
			successCount++
		}
	}
	
	// 应该成功5个左右（允许一些时间误差）
	assert.GreaterOrEqual(t, successCount, 4, "Should allow at least 4 connections after refill")
	assert.LessOrEqual(t, successCount, 6, "Should not allow more than 6 connections")
}

func TestRateLimiter_AllowIPBurst(t *testing.T) {
	config := &RateLimitConfig{
		Rate:  5,
		Burst: 10,
		TTL:   5 * time.Minute,
	}
	
	limiter := NewRateLimiter(config, nil, context.Background())
	
	ip := "192.168.1.100"
	
	// 一次性请求5个令牌
	allowed := limiter.AllowIPBurst(ip, 5)
	assert.True(t, allowed, "Should allow burst of 5")
	
	// 再请求10个，应该失败（只剩5个）
	allowed = limiter.AllowIPBurst(ip, 10)
	assert.False(t, allowed, "Should block burst of 10 (only 5 remaining)")
	
	// 请求5个，应该成功
	allowed = limiter.AllowIPBurst(ip, 5)
	assert.True(t, allowed, "Should allow burst of 5 (last 5 tokens)")
	
	// 再请求1个，应该失败
	allowed = limiter.AllowIP(ip)
	assert.False(t, allowed, "Should block after all tokens consumed")
}

func TestRateLimiter_SetIPRateLimit(t *testing.T) {
	config := &RateLimitConfig{
		Rate:  2,
		Burst: 5,
		TTL:   5 * time.Minute,
	}
	
	limiter := NewRateLimiter(config, nil, context.Background())
	
	ip := "192.168.1.100"
	
	// 消耗5个令牌
	for i := 0; i < 5; i++ {
		limiter.AllowIP(ip)
	}
	
	// 动态调整为更高的限制
	limiter.SetIPRateLimit(10, 15)
	
	// 应该有新的15个令牌
	for i := 0; i < 15; i++ {
		allowed := limiter.AllowIP(ip)
		assert.True(t, allowed, "Connection %d should be allowed after rate limit increase", i+1)
	}
}

func TestRateLimiter_GetStats(t *testing.T) {
	limiter := NewRateLimiter(nil, nil, context.Background())
	
	// 添加一些bucket
	limiter.AllowIP("192.168.1.100")
	limiter.AllowIP("192.168.1.101")
	limiter.AllowTunnel("tunnel-1", 100)
	
	stats := limiter.GetStats()
	assert.Equal(t, 2, stats.IPBucketCount, "Should have 2 IP buckets")
	assert.Equal(t, 1, stats.TunnelBucketCount, "Should have 1 tunnel bucket")
}

func TestRateLimiter_Reset(t *testing.T) {
	limiter := NewRateLimiter(nil, nil, context.Background())
	
	// 添加一些bucket
	limiter.AllowIP("192.168.1.100")
	limiter.AllowTunnel("tunnel-1", 100)
	
	stats := limiter.GetStats()
	require.Greater(t, stats.IPBucketCount+stats.TunnelBucketCount, 0)
	
	// 重置
	limiter.Reset()
	
	stats = limiter.GetStats()
	assert.Equal(t, 0, stats.IPBucketCount, "IP buckets should be reset")
	assert.Equal(t, 0, stats.TunnelBucketCount, "Tunnel buckets should be reset")
}

func TestRateLimiter_WaitIP(t *testing.T) {
	config := &RateLimitConfig{
		Rate:  10,               // 每秒10个
		Burst: 5,
		TTL:   5 * time.Minute,
	}
	
	limiter := NewRateLimiter(config, nil, context.Background())
	
	ip := "192.168.1.100"
	
	// 消耗所有令牌
	for i := 0; i < 5; i++ {
		limiter.AllowIP(ip)
	}
	
	// 等待令牌补充
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	start := time.Now()
	err := limiter.WaitIP(ctx, ip)
	elapsed := time.Since(start)
	
	require.NoError(t, err, "WaitIP should not return error")
	assert.Greater(t, elapsed, 50*time.Millisecond, "Should wait for refill")
}

func TestRateLimiter_WaitTunnel(t *testing.T) {
	config := &RateLimitConfig{
		Rate:  1000,
		Burst: 500,
		TTL:   5 * time.Minute,
	}
	
	limiter := NewRateLimiter(nil, config, context.Background())
	
	tunnelID := "tunnel-123"
	
	// 消耗所有令牌
	limiter.AllowTunnel(tunnelID, 500)
	
	// 等待令牌补充
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	start := time.Now()
	err := limiter.WaitTunnel(ctx, tunnelID, 500)
	elapsed := time.Since(start)
	
	require.NoError(t, err, "WaitTunnel should not return error")
	assert.Greater(t, elapsed, 400*time.Millisecond, "Should wait for refill")
}

