package tests

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDisposeIntegration 测试所有组件的Dispose集成
func TestDisposeIntegration(t *testing.T) {
	ctx := context.Background()

	// 测试云控制组件
	t.Run("CloudControl_Dispose", func(t *testing.T) {
		cloudControl := cloud.NewBuiltInCloudControl(nil)
		require.NotNil(t, cloudControl)

		// 启动云控制
		cloudControl.Start()

		// 验证未关闭
		assert.False(t, cloudControl.IsClosed())

		// 关闭云控制
		err := cloudControl.Close()
		assert.NoError(t, err)

		// 验证已关闭
		assert.True(t, cloudControl.IsClosed())
	})

	// 测试存储组件
	t.Run("Storage_Dispose", func(t *testing.T) {
		storage := cloud.NewMemoryStorage(ctx)
		require.NotNil(t, storage)

		// 验证未关闭
		assert.False(t, storage.IsClosed())

		// 关闭存储
		err := storage.Close()
		assert.NoError(t, err)

		// 验证已关闭
		assert.True(t, storage.IsClosed())
	})

	// 测试配置管理器
	t.Run("ConfigManager_Dispose", func(t *testing.T) {
		storage := cloud.NewMemoryStorage(ctx)
		config := &cloud.CloudControlConfig{}
		configManager := cloud.NewConfigManager(storage, config, ctx)
		require.NotNil(t, configManager)

		// 验证未关闭
		assert.False(t, configManager.IsClosed())

		// 关闭配置管理器
		configManager.Close()

		// 验证已关闭
		assert.True(t, configManager.IsClosed())
	})

	// 测试清理管理器
	t.Run("CleanupManager_Dispose", func(t *testing.T) {
		storage := cloud.NewMemoryStorage(ctx)
		lock := cloud.NewMemoryLock()
		cleanupManager := cloud.NewCleanupManager(storage, lock, ctx)
		require.NotNil(t, cleanupManager)

		// 验证未关闭
		assert.False(t, cleanupManager.IsClosed())

		// 关闭清理管理器
		cleanupManager.Close()

		// 验证已关闭
		assert.True(t, cleanupManager.IsClosed())
	})

	// 测试缓冲区管理器
	t.Run("BufferManager_Dispose", func(t *testing.T) {
		bufferManager := utils.NewBufferManager(ctx)
		require.NotNil(t, bufferManager)

		// 验证未关闭
		assert.False(t, bufferManager.IsClosed())

		// 关闭缓冲区管理器
		bufferManager.Close()

		// 验证已关闭
		assert.True(t, bufferManager.IsClosed())
	})

	// 测试限速器
	t.Run("RateLimiter_Dispose", func(t *testing.T) {
		rateLimiter := utils.NewRateLimiter(100, time.Second, ctx)
		require.NotNil(t, rateLimiter)

		// 验证未关闭
		assert.False(t, rateLimiter.IsClosed())

		// 关闭限速器
		rateLimiter.Close()

		// 验证已关闭
		assert.True(t, rateLimiter.IsClosed())
	})

	// 测试流组件
	t.Run("Stream_Dispose", func(t *testing.T) {
		// 测试PackageStream
		reader := &mockReader{}
		writer := &mockWriter{}
		packageStream := stream.NewPackageStream(reader, writer, ctx)
		require.NotNil(t, packageStream)

		// 验证未关闭
		assert.False(t, packageStream.IsClosed())

		// 关闭PackageStream
		packageStream.Close()

		// 验证已关闭
		assert.True(t, packageStream.IsClosed())

		// 测试GzipReader
		gzipReader := stream.NewGzipReader(reader, ctx)
		require.NotNil(t, gzipReader)

		// 验证未关闭
		assert.False(t, gzipReader.IsClosed())

		// 关闭GzipReader
		gzipReader.Close()

		// 验证已关闭
		assert.True(t, gzipReader.IsClosed())

		// 测试GzipWriter
		gzipWriter := stream.NewGzipWriter(writer, ctx)
		require.NotNil(t, gzipWriter)

		// 验证未关闭
		assert.False(t, gzipWriter.IsClosed())

		// 关闭GzipWriter
		gzipWriter.Close()

		// 验证已关闭
		assert.True(t, gzipWriter.IsClosed())
	})

	// 测试令牌桶
	t.Run("TokenBucket_Dispose", func(t *testing.T) {
		tokenBucket, err := stream.NewTokenBucket(1000, ctx)
		require.NoError(t, err)
		require.NotNil(t, tokenBucket)

		// 关闭令牌桶
		tokenBucket.Close()
	})

	// 测试限速读写器
	t.Run("RateLimiterIO_Dispose", func(t *testing.T) {
		reader := &mockReader{}
		writer := &mockWriter{}

		rateLimiterReader, err := stream.NewRateLimiterReader(reader, 1000, ctx)
		require.NoError(t, err)
		require.NotNil(t, rateLimiterReader)

		rateLimiterWriter, err := stream.NewRateLimiterWriter(writer, 1000, ctx)
		require.NoError(t, err)
		require.NotNil(t, rateLimiterWriter)

		// 验证未关闭
		assert.False(t, rateLimiterReader.IsClosed())
		assert.False(t, rateLimiterWriter.IsClosed())

		// 关闭限速读写器
		rateLimiterReader.Close()
		rateLimiterWriter.Close()

		// 验证已关闭
		assert.True(t, rateLimiterReader.IsClosed())
		assert.True(t, rateLimiterWriter.IsClosed())
	})
}

// TestDisposeCascade 测试Dispose的级联关闭
func TestDisposeCascade(t *testing.T) {
	// 创建云控制（包含多个子组件）
	cloudControl := cloud.NewBuiltInCloudControl(nil)
	require.NotNil(t, cloudControl)

	// 启动云控制
	cloudControl.Start()

	// 验证所有子组件都未关闭
	assert.False(t, cloudControl.IsClosed())

	// 关闭云控制（应该级联关闭所有子组件）
	err := cloudControl.Close()
	assert.NoError(t, err)

	// 验证云控制已关闭
	assert.True(t, cloudControl.IsClosed())
}

// TestDisposeConcurrency 测试Dispose的并发安全性
func TestDisposeConcurrency(t *testing.T) {
	ctx := context.Background()

	t.Run("Concurrent_Close", func(t *testing.T) {
		storage := cloud.NewMemoryStorage(ctx)
		require.NotNil(t, storage)

		// 并发关闭
		done := make(chan struct{})
		go func() {
			for i := 0; i < 10; i++ {
				storage.Close()
			}
			done <- struct{}{}
		}()
		go func() {
			for i := 0; i < 10; i++ {
				storage.Close()
			}
			done <- struct{}{}
		}()

		<-done
		<-done

		// 验证最终状态
		assert.True(t, storage.IsClosed())
	})

	t.Run("Concurrent_Operations", func(t *testing.T) {
		storage := cloud.NewMemoryStorage(ctx)
		require.NotNil(t, storage)

		// 并发操作和关闭
		done := make(chan struct{})
		go func() {
			for i := 0; i < 100; i++ {
				storage.Set(ctx, "key", "value", time.Minute)
			}
			done <- struct{}{}
		}()
		go func() {
			time.Sleep(10 * time.Millisecond)
			storage.Close()
			done <- struct{}{}
		}()

		<-done
		<-done

		// 验证最终状态
		assert.True(t, storage.IsClosed())
	})
}

// 模拟读写器
type mockReader struct{}

func (r *mockReader) Read(p []byte) (n int, err error) {
	return 0, nil
}

type mockWriter struct{}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
