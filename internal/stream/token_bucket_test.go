package stream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTokenBucket 测试创建令牌桶
func TestNewTokenBucket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rate    int64
		wantErr bool
	}{
		{
			name:    "valid rate",
			rate:    1024,
			wantErr: false,
		},
		{
			name:    "high rate",
			rate:    100 * 1024 * 1024,
			wantErr: false,
		},
		{
			name:    "minimum rate",
			rate:    1,
			wantErr: false,
		},
		{
			name:    "zero rate",
			rate:    0,
			wantErr: true,
		},
		{
			name:    "negative rate",
			rate:    -100,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tb, err := NewTokenBucket(tc.rate, ctx)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tb)
			} else {
				require.NoError(t, err)
				require.NotNil(t, tb)
				tb.Close()
			}
		})
	}
}

// TestTokenBucket_GetRate 测试获取速率
func TestTokenBucket_GetRate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rate := int64(1024)
	tb, err := NewTokenBucket(rate, ctx)
	require.NoError(t, err)
	defer tb.Close()

	gotRate := tb.GetRate()
	assert.Equal(t, rate, gotRate)
}

// TestTokenBucket_SetRate 测试设置速率
func TestTokenBucket_SetRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		initialRate int64
		newRate     int64
		wantErr     bool
	}{
		{
			name:        "increase rate",
			initialRate: 1024,
			newRate:     2048,
			wantErr:     false,
		},
		{
			name:        "decrease rate",
			initialRate: 2048,
			newRate:     1024,
			wantErr:     false,
		},
		{
			name:        "set to zero",
			initialRate: 1024,
			newRate:     0,
			wantErr:     true,
		},
		{
			name:        "set to negative",
			initialRate: 1024,
			newRate:     -100,
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tb, err := NewTokenBucket(tc.initialRate, ctx)
			require.NoError(t, err)
			defer tb.Close()

			err = tb.SetRate(tc.newRate)

			if tc.wantErr {
				assert.Error(t, err)
				// 速率应该保持不变
				assert.Equal(t, tc.initialRate, tb.GetRate())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.newRate, tb.GetRate())
			}
		})
	}
}

// TestTokenBucket_GetBurstSize 测试获取突发大小
func TestTokenBucket_GetBurstSize(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rate := int64(1024)
	tb, err := NewTokenBucket(rate, ctx)
	require.NoError(t, err)
	defer tb.Close()

	burstSize := tb.GetBurstSize()
	assert.Greater(t, burstSize, 0)
	assert.LessOrEqual(t, int64(burstSize), rate)
}

// TestTokenBucket_GetTokens 测试获取当前令牌数
func TestTokenBucket_GetTokens(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tb, err := NewTokenBucket(1024, ctx)
	require.NoError(t, err)
	defer tb.Close()

	// 初始令牌数应该是0
	tokens := tb.GetTokens()
	assert.Equal(t, 0, tokens)
}

// TestTokenBucket_WaitForTokens 测试等待令牌
func TestTokenBucket_WaitForTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		rate         int64
		tokensNeeded int
		wantErr      bool
	}{
		{
			name:         "small request with high rate",
			rate:         1024 * 1024,
			tokensNeeded: 100,
			wantErr:      false,
		},
		{
			name:         "zero tokens needed",
			rate:         1024,
			tokensNeeded: 0,
			wantErr:      false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tb, err := NewTokenBucket(tc.rate, ctx)
			require.NoError(t, err)
			defer tb.Close()

			err = tb.WaitForTokens(tc.tokensNeeded)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTokenBucket_WaitForTokens_ContextCancelled 测试 context 取消
func TestTokenBucket_WaitForTokens_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	// 使用非常低的速率
	tb, err := NewTokenBucket(1, ctx)
	require.NoError(t, err)
	defer tb.Close()

	// 取消 context
	cancel()

	// 请求大量令牌应该因为 context 取消而失败
	err = tb.WaitForTokens(1000)
	assert.Error(t, err)
}

// TestTokenBucket_WaitForTokens_Concurrent 测试并发等待令牌
func TestTokenBucket_WaitForTokens_Concurrent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 使用高速率以避免测试超时
	tb, err := NewTokenBucket(10*1024*1024, ctx)
	require.NoError(t, err)
	defer tb.Close()

	const numGoroutines = 10
	const tokensPerRequest = 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := tb.WaitForTokens(tokensPerRequest); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// 不应该有错误
	for err := range errors {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestTokenBucket_TokenRefill 测试令牌补充
func TestTokenBucket_TokenRefill(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用较高的速率
	rate := int64(10000)
	tb, err := NewTokenBucket(rate, ctx)
	require.NoError(t, err)
	defer tb.Close()

	// 等待一段时间让令牌产生
	time.Sleep(100 * time.Millisecond)

	// 消耗一些令牌
	err = tb.WaitForTokens(100)
	require.NoError(t, err)
}

// TestTokenBucket_BurstCapacity 测试突发容量
func TestTokenBucket_BurstCapacity(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rate := int64(10000)
	tb, err := NewTokenBucket(rate, ctx)
	require.NoError(t, err)
	defer tb.Close()

	burstSize := tb.GetBurstSize()
	assert.Greater(t, burstSize, 0)

	// 突发大小不应超过速率
	assert.LessOrEqual(t, int64(burstSize), rate)
}

// TestTokenBucket_SetRate_AdjustsBurstSize 测试设置速率时调整突发大小
func TestTokenBucket_SetRate_AdjustsBurstSize(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tb, err := NewTokenBucket(1024, ctx)
	require.NoError(t, err)
	defer tb.Close()

	originalBurstSize := tb.GetBurstSize()

	// 增加速率
	err = tb.SetRate(10240)
	require.NoError(t, err)

	newBurstSize := tb.GetBurstSize()
	// 新的突发大小应该随速率调整
	assert.NotEqual(t, originalBurstSize, newBurstSize)
}

// TestTokenBucket_SetRate_AdjustsTokens 测试设置速率时调整令牌数
func TestTokenBucket_SetRate_AdjustsTokens(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用较高的初始速率
	tb, err := NewTokenBucket(100000, ctx)
	require.NoError(t, err)
	defer tb.Close()

	// 等待令牌积累
	time.Sleep(50 * time.Millisecond)

	// 减小速率到很低
	err = tb.SetRate(100)
	require.NoError(t, err)

	// 令牌数不应超过新的突发大小
	tokens := tb.GetTokens()
	burstSize := tb.GetBurstSize()
	assert.LessOrEqual(t, tokens, burstSize)
}

// TestTokenBucket_Close 测试关闭
func TestTokenBucket_Close(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tb, err := NewTokenBucket(1024, ctx)
	require.NoError(t, err)

	// Close 不应该 panic
	assert.NotPanics(t, func() {
		tb.Close()
	})

	// 重复关闭也不应该 panic
	assert.NotPanics(t, func() {
		tb.Close()
	})
}

// BenchmarkTokenBucket_WaitForTokens 基准测试等待令牌
func BenchmarkTokenBucket_WaitForTokens(b *testing.B) {
	ctx := context.Background()
	tb, _ := NewTokenBucket(100*1024*1024, ctx)
	defer tb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.WaitForTokens(1024)
	}
}

// BenchmarkTokenBucket_GetRate 基准测试获取速率
func BenchmarkTokenBucket_GetRate(b *testing.B) {
	ctx := context.Background()
	tb, _ := NewTokenBucket(1024, ctx)
	defer tb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.GetRate()
	}
}

// BenchmarkTokenBucket_SetRate 基准测试设置速率
func BenchmarkTokenBucket_SetRate(b *testing.B) {
	ctx := context.Background()
	tb, _ := NewTokenBucket(1024, ctx)
	defer tb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.SetRate(int64(1024 + i%100))
	}
}
