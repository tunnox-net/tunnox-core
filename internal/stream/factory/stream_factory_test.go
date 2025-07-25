package factory

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamFactory_BasicCreation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 创建默认流工厂
	factory := stream.NewDefaultStreamFactory(ctx)

	// 测试创建流处理器
	var buf bytes.Buffer
	processor := factory.NewStreamProcessor(&buf, &buf)
	require.NotNil(t, processor)

	// 测试创建压缩读写器
	compressionReader := factory.NewCompressionReader(&buf)
	require.NotNil(t, compressionReader)

	compressionWriter := factory.NewCompressionWriter(&buf)
	require.NotNil(t, compressionWriter)

	// 测试创建限速读写器
	rateLimiterReader, err := factory.NewRateLimiterReader(&buf, 1024)
	require.NoError(t, err)
	require.NotNil(t, rateLimiterReader)

	rateLimiterWriter, err := factory.NewRateLimiterWriter(&buf, 1024)
	require.NoError(t, err)
	require.NotNil(t, rateLimiterWriter)

	// 清理资源
	processor.Close()
	compressionReader.Close()
	compressionWriter.Close()
	rateLimiterReader.Close()
	rateLimiterWriter.Close()
}

func TestConfigurableStreamFactory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 创建配置
	config := stream.StreamFactoryConfig{
		DefaultCompression: true,
		DefaultRateLimit:   1024,
		BufferSize:         4096,
		EnableMemoryPool:   true,
	}

	// 创建可配置流工厂
	factory := stream.NewConfigurableStreamFactory(ctx, config)
	require.NotNil(t, factory)

	// 验证配置
	retrievedConfig := factory.GetConfig()
	assert.Equal(t, config.DefaultCompression, retrievedConfig.DefaultCompression)
	assert.Equal(t, config.DefaultRateLimit, retrievedConfig.DefaultRateLimit)
	assert.Equal(t, config.BufferSize, retrievedConfig.BufferSize)
	assert.Equal(t, config.EnableMemoryPool, retrievedConfig.EnableMemoryPool)
}

func TestStreamManager_BasicOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 创建流工厂和管理器
	factory := stream.NewDefaultStreamFactory(ctx)
	manager := stream.NewStreamManager(factory, ctx)
	require.NotNil(t, manager)

	// 测试创建流
	var buf bytes.Buffer
	stream1, err := manager.CreateStream("test1", &buf, &buf)
	require.NoError(t, err)
	require.NotNil(t, stream1)

	// 测试重复创建（应该失败）
	_, err = manager.CreateStream("test1", &buf, &buf)
	require.Error(t, err)

	// 测试获取流
	retrievedStream, exists := manager.GetStream("test1")
	require.True(t, exists)
	require.Equal(t, stream1, retrievedStream)

	// 测试获取不存在的流
	_, exists = manager.GetStream("nonexistent")
	require.False(t, exists)

	// 测试列出流
	streams := manager.ListStreams()
	require.Len(t, streams, 1)
	require.Contains(t, streams, "test1")

	// 测试流数量
	count := manager.GetStreamCount()
	require.Equal(t, 1, count)

	// 测试移除流
	err = manager.RemoveStream("test1")
	require.NoError(t, err)

	// 验证流已被移除
	_, exists = manager.GetStream("test1")
	require.False(t, exists)

	count = manager.GetStreamCount()
	require.Equal(t, 0, count)
}

func TestStreamManager_WithPacketOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建流工厂和管理器
	factory := stream.NewDefaultStreamFactory(ctx)
	manager := stream.NewStreamManager(factory, ctx)

	// 创建测试数据包
	commandPacket := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		Token:       "test-token",
		SenderId:    "sender-001",
		ReceiverId:  "receiver-001",
		CommandBody: "Test command body",
	}

	testPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: commandPacket,
	}

	// 创建流
	var buf bytes.Buffer
	stream1, err := manager.CreateStream("test1", &buf, &buf)
	require.NoError(t, err)

	// 测试数据包写入
	writtenBytes, err := stream1.WritePacket(testPacket, false, 0)
	require.NoError(t, err)
	require.Greater(t, writtenBytes, 0)

	// 测试数据包读取
	readPacket, readBytes, err := stream1.ReadPacket()
	require.NoError(t, err)
	require.Greater(t, readBytes, 0)
	require.NotNil(t, readPacket)
	require.NotNil(t, readPacket.CommandPacket)
	require.Equal(t, testPacket.CommandPacket.CommandType, readPacket.CommandPacket.CommandType)
	require.Equal(t, testPacket.CommandPacket.Token, readPacket.CommandPacket.Token)

	// 清理资源
	manager.CloseAllStreams()
}

func TestStreamProfiles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 测试获取预定义配置
	profile, err := stream.GetProfile("default")
	require.NoError(t, err)
	require.Equal(t, "default", profile.Name)
	require.Equal(t, stream.StreamTypeBasic, profile.Type)

	// 测试获取不存在的配置
	_, err = stream.GetProfile("nonexistent")
	require.Error(t, err)

	// 测试从配置创建工厂
	factory, err := stream.CreateFactoryFromProfile(ctx, "high_performance")
	require.NoError(t, err)
	require.NotNil(t, factory)

	// 测试从配置创建管理器
	manager, err := stream.CreateManagerFromProfile(ctx, "bandwidth_saving")
	require.NoError(t, err)
	require.NotNil(t, manager)

	// 测试指标
	metrics := manager.GetMetrics()
	require.Equal(t, 0, metrics.TotalStreams)
	require.Equal(t, 0, metrics.ActiveStreams)
}

func TestStreamManager_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建流管理器
	factory := stream.NewDefaultStreamFactory(ctx)
	manager := stream.NewStreamManager(factory, ctx)

	// 并发创建多个流
	const numStreams = 10
	var buf bytes.Buffer

	// 使用goroutine并发创建流
	done := make(chan struct{})
	go func() {
		for i := 0; i < numStreams; i++ {
			streamID := fmt.Sprintf("concurrent_%d", i)
			_, err := manager.CreateStream(streamID, &buf, &buf)
			require.NoError(t, err)
		}
		done <- struct{}{}
	}()

	// 等待完成
	<-done

	// 验证所有流都已创建
	count := manager.GetStreamCount()
	require.Equal(t, numStreams, count)

	// 验证所有流都在列表中
	streams := manager.ListStreams()
	require.Len(t, streams, numStreams)

	// 清理资源
	manager.CloseAllStreams()
}
