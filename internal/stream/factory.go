package stream

import (
	"context"
	"io"
	"tunnox-core/internal/stream/compression"
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
	config     *StreamFactoryConfig
	// 注意：加密功能已移至 internal/stream/transform 模块
	ctx        context.Context
}

// NewDefaultStreamFactory 创建新的默认流工厂
func NewDefaultStreamFactory(ctx context.Context) *DefaultStreamFactory {
	config := DefaultStreamFactoryConfig()
	return &DefaultStreamFactory{
		config:     config,
		// 注意：加密功能已移至 internal/stream/transform 模块
		ctx:        ctx,
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
func (sf *DefaultStreamFactory) CreateStreamProcessorWithConfig(reader io.Reader, writer io.Writer, config *StreamFactoryConfig) PackageStreamer {
	// 限流
	if config.EnableRateLimit {
		if rateLimiterReader, err := NewRateLimiterReader(reader, config.RateLimitBytes, sf.ctx); err == nil {
			reader = rateLimiterReader
		}
		if rateLimiterWriter, err := NewRateLimiterWriter(writer, config.RateLimitBytes, sf.ctx); err == nil {
			writer = rateLimiterWriter
		}
	}

	// 压缩
	if config.EnableCompression {
		reader = compression.NewGzipReader(reader, sf.ctx)
		writer = compression.NewGzipWriter(writer, sf.ctx)
	}

	// 注意：加密功能已移至 internal/stream/transform 模块
	// 加密应通过 transform.NewTransformer() 配置
	return NewStreamProcessor(reader, writer, sf.ctx)
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
