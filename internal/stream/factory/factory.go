package factory

import (
	"context"
	"io"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/stream/compression"
	"tunnox-core/internal/stream/encryption"
	"tunnox-core/internal/stream/processor"
	"tunnox-core/internal/stream/rate_limiting"
)

// StreamFactoryConfig 流工厂配置
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
		EnableCompression: true,
		EnableEncryption:  false,
		EnableRateLimit:   false,
		CompressionLevel:  6,
		EncryptionKey:     nil,
		RateLimitBytes:    1024 * 1024, // 1MB/s
		BufferSize:        4096,
	}
}

// 使用stream包中的StreamFactory接口
type StreamFactory = stream.StreamFactory

// DefaultStreamFactory 默认流工厂实现
type DefaultStreamFactory struct {
	config             *StreamFactoryConfig
	compressionFactory compression.CompressionFactory
	encryption         encryption.Encryption
	rateLimiter        rate_limiting.RateLimiter
}

// NewDefaultStreamFactory 创建新的默认流工厂
func NewDefaultStreamFactory(ctx context.Context) *DefaultStreamFactory {
	config := DefaultStreamFactoryConfig()
	return &DefaultStreamFactory{
		config:             config,
		compressionFactory: compression.NewDefaultCompressionFactory(),
		encryption:         encryption.NewNoEncryption(),
		rateLimiter:        rate_limiting.NewNoRateLimiter(),
	}
}

// NewConfigurableStreamFactory 创建可配置的流工厂
func NewConfigurableStreamFactory(ctx context.Context, config *StreamFactoryConfig) *DefaultStreamFactory {
	if config == nil {
		config = DefaultStreamFactoryConfig()
	}

	factory := &DefaultStreamFactory{
		config:             config,
		compressionFactory: compression.NewDefaultCompressionFactory(),
	}

	// 根据配置设置加密
	if config.EnableEncryption && config.EncryptionKey != nil {
		if enc, err := encryption.NewAESEncryption(config.EncryptionKey); err == nil {
			factory.encryption = enc
		}
	}

	// 根据配置设置限流
	if config.EnableRateLimit {
		factory.rateLimiter = rate_limiting.NewTokenBucketRateLimiter(config.RateLimitBytes, config.RateLimitBytes)
	}

	return factory
}

// CreateStreamProcessor 创建流处理器
func (sf *DefaultStreamFactory) CreateStreamProcessor(reader io.Reader, writer io.Writer) processor.StreamProcessor {
	return sf.CreateStreamProcessorWithConfig(reader, writer, sf.config)
}

// CreateStreamProcessorWithConfig 使用配置创建流处理器
func (sf *DefaultStreamFactory) CreateStreamProcessorWithConfig(reader io.Reader, writer io.Writer, config *StreamFactoryConfig) processor.StreamProcessor {
	var compressionReader compression.CompressionReader
	var compressionWriter compression.CompressionWriter
	var rateLimiter rate_limiting.RateLimiter

	// 创建压缩组件
	if config.EnableCompression {
		compressionReader = sf.compressionFactory.NewCompressionReader(reader)
		compressionWriter = sf.compressionFactory.NewCompressionWriter(writer)
	} else {
		compressionReader = &compression.NoCompressionReader{Reader: reader}
		compressionWriter = &compression.NoCompressionWriter{Writer: writer}
	}

	// 创建限流组件
	if config.EnableRateLimit {
		rateLimiter = rate_limiting.NewTokenBucketRateLimiter(config.RateLimitBytes, config.RateLimitBytes)
	} else {
		rateLimiter = rate_limiting.NewNoRateLimiter()
	}

	return processor.NewDefaultStreamProcessor(
		reader,
		writer,
		compressionReader,
		compressionWriter,
		rateLimiter,
		sf.encryption,
	)
}

// GetConfig 获取配置
func (sf *DefaultStreamFactory) GetConfig() *StreamFactoryConfig {
	return sf.config
}

// SetConfig 设置配置
func (sf *DefaultStreamFactory) SetConfig(config *StreamFactoryConfig) {
	sf.config = config
}
