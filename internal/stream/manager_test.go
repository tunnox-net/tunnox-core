package stream

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewStreamManager 测试创建流管理器
func TestNewStreamManager(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)

	require.NotNil(t, manager)
	assert.Equal(t, 0, manager.GetStreamCount())
}

// TestStreamManager_CreateStream 测试创建流
func TestStreamManager_CreateStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		streamID   string
		wantErr    bool
	}{
		{
			name:     "valid stream ID",
			streamID: "test-stream-1",
			wantErr:  false,
		},
		{
			name:     "empty stream ID",
			streamID: "",
			wantErr:  false,
		},
		{
			name:     "unicode stream ID",
			streamID: "stream-中文-日本語",
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			factory := NewDefaultStreamFactory(ctx)
			manager := NewStreamManager(factory, ctx)

			var buf bytes.Buffer
			stream, err := manager.CreateStream(tc.streamID, &buf, &buf)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, stream)
			} else {
				require.NoError(t, err)
				require.NotNil(t, stream)
				assert.Equal(t, 1, manager.GetStreamCount())
			}

			manager.CloseAllStreams()
		})
	}
}

// TestStreamManager_CreateStream_Duplicate 测试创建重复流
func TestStreamManager_CreateStream_Duplicate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	var buf bytes.Buffer
	streamID := "duplicate-test"

	// 第一次创建应该成功
	stream1, err := manager.CreateStream(streamID, &buf, &buf)
	require.NoError(t, err)
	require.NotNil(t, stream1)

	// 第二次创建应该失败
	stream2, err := manager.CreateStream(streamID, &buf, &buf)
	assert.Error(t, err)
	assert.Nil(t, stream2)

	// 流数量仍然是 1
	assert.Equal(t, 1, manager.GetStreamCount())
}

// TestStreamManager_GetStream 测试获取流
func TestStreamManager_GetStream(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	var buf bytes.Buffer
	streamID := "get-test"

	// 创建流
	createdStream, err := manager.CreateStream(streamID, &buf, &buf)
	require.NoError(t, err)

	// 获取流
	gotStream, exists := manager.GetStream(streamID)
	assert.True(t, exists)
	assert.Equal(t, createdStream, gotStream)

	// 获取不存在的流
	_, exists = manager.GetStream("nonexistent")
	assert.False(t, exists)
}

// TestStreamManager_RemoveStream 测试移除流
func TestStreamManager_RemoveStream(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	var buf bytes.Buffer
	streamID := "remove-test"

	// 创建流
	_, err := manager.CreateStream(streamID, &buf, &buf)
	require.NoError(t, err)
	assert.Equal(t, 1, manager.GetStreamCount())

	// 移除流
	err = manager.RemoveStream(streamID)
	require.NoError(t, err)
	assert.Equal(t, 0, manager.GetStreamCount())

	// 再次移除应该失败
	err = manager.RemoveStream(streamID)
	assert.Error(t, err)
}

// TestStreamManager_ListStreams 测试列出流
func TestStreamManager_ListStreams(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	var buf bytes.Buffer

	// 初始为空
	streams := manager.ListStreams()
	assert.Len(t, streams, 0)

	// 创建多个流
	streamIDs := []string{"stream-1", "stream-2", "stream-3"}
	for _, id := range streamIDs {
		_, err := manager.CreateStream(id, &buf, &buf)
		require.NoError(t, err)
	}

	// 验证列表
	streams = manager.ListStreams()
	assert.Len(t, streams, 3)
	for _, id := range streamIDs {
		assert.Contains(t, streams, id)
	}
}

// TestStreamManager_GetStreamCount 测试获取流数量
func TestStreamManager_GetStreamCount(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	var buf bytes.Buffer

	// 初始数量为 0
	assert.Equal(t, 0, manager.GetStreamCount())

	// 创建流
	for i := 0; i < 5; i++ {
		_, err := manager.CreateStream(string(rune('a'+i)), &buf, &buf)
		require.NoError(t, err)
	}
	assert.Equal(t, 5, manager.GetStreamCount())

	// 移除一个流
	err := manager.RemoveStream("a")
	require.NoError(t, err)
	assert.Equal(t, 4, manager.GetStreamCount())
}

// TestStreamManager_CloseAllStreams 测试关闭所有流
func TestStreamManager_CloseAllStreams(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)

	var buf bytes.Buffer

	// 创建多个流
	for i := 0; i < 5; i++ {
		_, err := manager.CreateStream(string(rune('a'+i)), &buf, &buf)
		require.NoError(t, err)
	}
	assert.Equal(t, 5, manager.GetStreamCount())

	// 关闭所有流
	err := manager.CloseAllStreams()
	require.NoError(t, err)
	assert.Equal(t, 0, manager.GetStreamCount())
}

// TestStreamManager_Dispose 测试 Dispose
func TestStreamManager_Dispose(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)

	var buf bytes.Buffer

	// 创建流
	_, err := manager.CreateStream("test", &buf, &buf)
	require.NoError(t, err)

	// Dispose 应该关闭所有流
	err = manager.Dispose()
	require.NoError(t, err)
	assert.Equal(t, 0, manager.GetStreamCount())
}

// TestStreamManager_CreateStreamWithConfig 测试使用配置创建流
func TestStreamManager_CreateStreamWithConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config StreamConfig
	}{
		{
			name: "basic config",
			config: StreamConfig{
				ID:                "config-test-1",
				EnableCompression: false,
				RateLimit:         0,
				BufferSize:        4096,
			},
		},
		{
			name: "with compression",
			config: StreamConfig{
				ID:                "config-test-2",
				EnableCompression: true,
				RateLimit:         0,
				BufferSize:        8192,
			},
		},
		{
			name: "with rate limit",
			config: StreamConfig{
				ID:                "config-test-3",
				EnableCompression: false,
				RateLimit:         1024,
				BufferSize:        4096,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			factory := NewDefaultStreamFactory(ctx)
			manager := NewStreamManager(factory, ctx)
			defer manager.CloseAllStreams()

			var buf bytes.Buffer
			stream, err := manager.CreateStreamWithConfig(tc.config, &buf, &buf)

			require.NoError(t, err)
			require.NotNil(t, stream)
		})
	}
}

// TestStreamManager_ConcurrentAccess 测试并发访问
func TestStreamManager_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	const numGoroutines = 20
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*3)

	// 并发创建流
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var buf bytes.Buffer
			streamID := string(rune('a' + idx))
			_, err := manager.CreateStream(streamID, &buf, &buf)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()

	// 并发读取
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			streamID := string(rune('a' + idx))
			_, _ = manager.GetStream(streamID)
		}(i)
	}

	wg.Wait()

	// 并发列出
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.ListStreams()
			_ = manager.GetStreamCount()
		}()
	}

	wg.Wait()

	close(errors)

	// 检查错误
	for err := range errors {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestStreamConfig 测试 StreamConfig 结构
func TestStreamConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config StreamConfig
	}{
		{
			name: "empty config",
			config: StreamConfig{},
		},
		{
			name: "full config",
			config: StreamConfig{
				ID:                "full-config",
				EnableCompression: true,
				RateLimit:         1024 * 1024,
				BufferSize:        32 * 1024,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 验证配置字段可以正常读取
			_ = tc.config.ID
			_ = tc.config.EnableCompression
			_ = tc.config.RateLimit
			_ = tc.config.BufferSize
		})
	}
}

// BenchmarkStreamManager_CreateStream 基准测试创建流
func BenchmarkStreamManager_CreateStream(b *testing.B) {
	ctx := context.Background()
	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		streamID := string(rune(i % 1000))
		manager.CreateStream(streamID+string(rune(i/1000)), &buf, &buf)
	}
}

// BenchmarkStreamManager_GetStream 基准测试获取流
func BenchmarkStreamManager_GetStream(b *testing.B) {
	ctx := context.Background()
	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	var buf bytes.Buffer
	_, _ = manager.CreateStream("test", &buf, &buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetStream("test")
	}
}

// BenchmarkStreamManager_ListStreams 基准测试列出流
func BenchmarkStreamManager_ListStreams(b *testing.B) {
	ctx := context.Background()
	factory := NewDefaultStreamFactory(ctx)
	manager := NewStreamManager(factory, ctx)
	defer manager.CloseAllStreams()

	var buf bytes.Buffer
	for i := 0; i < 100; i++ {
		manager.CreateStream(string(rune(i)), &buf, &buf)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.ListStreams()
	}
}
