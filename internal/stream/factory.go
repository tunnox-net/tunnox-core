package stream

import (
	"context"
	"io"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream/compression"
	"tunnox-core/internal/stream/encryption"
)

// StreamFactoryConfig 流工厂配置
// 合并自 factory/factory.go
type StreamFactoryConfig struct {
	EnableCompression bool
	EnableEncryption  bool
	EnableRateLimit   bool
	CompressionLevel  int
	EncryptionKey     []byte
	RateLimitBytes    int64
	BufferSize        int
}

// DefaultStreamFactoryConfig 默认流工厂配置
func DefaultStreamFactoryConfig() *StreamFactoryConfig {
	return &StreamFactoryConfig{
		EnableCompression: false,
		EnableEncryption:  false,
		EnableRateLimit:   false,
		CompressionLevel:  6,
		EncryptionKey:     nil,
		RateLimitBytes:    1024 * 1024, // 1MB/s
		BufferSize:        4096,
	}
}

// DefaultStreamFactory 默认流工厂实现
// 合并自 factory/factory.go
type DefaultStreamFactory struct {
	config *StreamFactoryConfig
	// 注意：加密功能已移至 internal/stream/transform 模块
	ctx context.Context
}

// NewDefaultStreamFactory 创建新的默认流工厂
func NewDefaultStreamFactory(ctx context.Context) *DefaultStreamFactory {
	config := DefaultStreamFactoryConfig()
	return &DefaultStreamFactory{
		config: config,
		// 注意：加密功能已移至 internal/stream/transform 模块
		ctx: ctx,
	}
}

// NewConfigurableStreamFactory 创建可配置的流工厂
func NewConfigurableStreamFactory(ctx context.Context, config *StreamFactoryConfig) *DefaultStreamFactory {
	if config == nil {
		config = DefaultStreamFactoryConfig()
	}
	factory := &DefaultStreamFactory{
		config: config,
		ctx:    ctx,
	}
	// 注意：加密功能已移至 internal/stream/transform 模块
	// 加密配置应通过 transform.TransformConfig 设置
	return factory
}

// CreateStreamProcessor 创建流处理器
func (sf *DefaultStreamFactory) CreateStreamProcessor(reader io.Reader, writer io.Writer) PackageStreamer {
	return sf.CreateStreamProcessorWithConfig(reader, writer, sf.config)
}

// CreateStreamProcessorWithConfig 使用配置创建流处理器
// StreamProcessor统一处理：压缩 + 加密 + 限流
// Transform只处理：流量统计等商业特性
func (sf *DefaultStreamFactory) CreateStreamProcessorWithConfig(reader io.Reader, writer io.Writer, config *StreamFactoryConfig) PackageStreamer {
	// 为每个连接创建独立的 context，确保连接关闭时子组件的 dispose goroutine 能正确退出
	connCtx, connCancel := context.WithCancel(sf.ctx)

	// 1. 加密（最内层，直接包装原始连接）
	if config.EnableEncryption && len(config.EncryptionKey) > 0 {
		encryptConfig := &encryption.EncryptConfig{
			Method: encryption.MethodAESGCM,
			Key:    config.EncryptionKey,
		}

		encryptor, err := encryption.NewEncryptor(encryptConfig)
		if err != nil {
			corelog.Default().Warnf("Failed to create encryptor: %v", err)
		} else {
			if decryptReader, err := encryptor.NewDecryptReader(reader); err == nil {
				reader = decryptReader
			}
			if encryptWriter, err := encryptor.NewEncryptWriter(writer); err == nil {
				writer = encryptWriter
			}
		}
	}

	// 2. 压缩（使用连接级别的 context）
	if config.EnableCompression {
		reader = compression.NewGzipReader(reader, connCtx)
		writer = compression.NewGzipWriter(writer, connCtx)
	}

	// 3. 限流（使用连接级别的 context）
	if config.EnableRateLimit {
		if rateLimiterReader, err := NewRateLimiterReader(reader, config.RateLimitBytes, connCtx); err == nil {
			reader = rateLimiterReader
		}
		if rateLimiterWriter, err := NewRateLimiterWriter(writer, config.RateLimitBytes, connCtx); err == nil {
			writer = rateLimiterWriter
		}
	}

	sp := NewStreamProcessor(reader, writer, connCtx)
	// StreamProcessor 关闭时取消连接级别的 context，让子组件的 dispose goroutine 退出
	sp.AddCleanHandler(func() error {
		connCancel()
		return nil
	})
	return sp
}

// NewRateLimiterReader 创建限速读取器
func (sf *DefaultStreamFactory) NewRateLimiterReader(reader io.Reader, bytesPerSecond int64) (*RateLimiterReader, error) {
	return NewRateLimiterReader(reader, bytesPerSecond, sf.ctx)
}

// NewRateLimiterWriter 创建限速写入器
func (sf *DefaultStreamFactory) NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64) (*RateLimiterWriter, error) {
	return NewRateLimiterWriter(writer, bytesPerSecond, sf.ctx)
}

// NewCompressionReader 创建压缩读取器
func (sf *DefaultStreamFactory) NewCompressionReader(reader io.Reader) *compression.GzipReader {
	return compression.NewGzipReader(reader, sf.ctx)
}

// NewCompressionWriter 创建压缩写入器
func (sf *DefaultStreamFactory) NewCompressionWriter(writer io.Writer) *compression.GzipWriter {
	return compression.NewGzipWriter(writer, sf.ctx)
}

// GetConfig 获取配置
func (sf *DefaultStreamFactory) GetConfig() *StreamFactoryConfig {
	return sf.config
}

// SetConfig 设置配置
func (sf *DefaultStreamFactory) SetConfig(config *StreamFactoryConfig) {
	sf.config = config
}

// NewStreamProcessor 创建新的数据包流（接口实现）
func (sf *DefaultStreamFactory) NewStreamProcessor(reader io.Reader, writer io.Writer) PackageStreamer {
	return sf.CreateStreamProcessor(reader, writer)
}
