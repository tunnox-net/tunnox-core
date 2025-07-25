package factory

import (
	"context"
	"io"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/stream/encryption"
	"tunnox-core/internal/stream/processor"
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
	config     *StreamFactoryConfig
	encryption encryption.Encryption
}

// NewDefaultStreamFactory 创建新的默认流工厂
func NewDefaultStreamFactory(ctx context.Context) *DefaultStreamFactory {
	config := DefaultStreamFactoryConfig()
	return &DefaultStreamFactory{
		config:     config,
		encryption: encryption.NewNoEncryption(),
	}
}

// NewConfigurableStreamFactory 创建可配置的流工厂
func NewConfigurableStreamFactory(ctx context.Context, config *StreamFactoryConfig) *DefaultStreamFactory {
	if config == nil {
		config = DefaultStreamFactoryConfig()
	}

	factory := &DefaultStreamFactory{
		config: config,
	}

	// 根据配置设置加密
	if config.EnableEncryption && config.EncryptionKey != nil {
		if enc, err := encryption.NewAESEncryption(config.EncryptionKey); err == nil {
			factory.encryption = enc
		}
	}

	return factory
}

// CreateStreamProcessor 创建流处理器
func (sf *DefaultStreamFactory) CreateStreamProcessor(reader io.Reader, writer io.Writer) processor.StreamProcessor {
	return sf.CreateStreamProcessorWithConfig(reader, writer, sf.config)
}

// CreateStreamProcessorWithConfig 使用配置创建流处理器
func (sf *DefaultStreamFactory) CreateStreamProcessorWithConfig(reader io.Reader, writer io.Writer, config *StreamFactoryConfig) processor.StreamProcessor {
	var compressionReader *stream.GzipReader
	var compressionWriter *stream.GzipWriter

	// 创建压缩组件
	if config.EnableCompression {
		compressionReader = stream.NewGzipReader(reader, context.Background())
		compressionWriter = stream.NewGzipWriter(writer, context.Background())
	} else {
		// 对于无压缩，直接使用原始reader和writer
		// 这里需要创建一个无压缩的包装器或者修改processor接口
		compressionReader = nil
		compressionWriter = nil
	}

	// 创建流处理器
	processor := processor.NewDefaultStreamProcessor(
		reader,
		writer,
		compressionReader,
		compressionWriter,
		nil, // 不再使用rate_limiting包
		sf.encryption,
	)

	// 如果启用限流，包装限流器
	if config.EnableRateLimit {
		// 使用stream包中的限流器
		if rateLimiterReader, err := stream.NewRateLimiterReader(reader, config.RateLimitBytes, context.Background()); err == nil {
			reader = rateLimiterReader
		}
		if rateLimiterWriter, err := stream.NewRateLimiterWriter(writer, config.RateLimitBytes, context.Background()); err == nil {
			writer = rateLimiterWriter
		}
	}

	return processor
}

// NewRateLimiterReader 创建限速读取器
func (sf *DefaultStreamFactory) NewRateLimiterReader(reader io.Reader, bytesPerSecond int64) (*stream.RateLimiterReader, error) {
	return stream.NewRateLimiterReader(reader, bytesPerSecond, context.Background())
}

// NewRateLimiterWriter 创建限速写入器
func (sf *DefaultStreamFactory) NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64) (*stream.RateLimiterWriter, error) {
	return stream.NewRateLimiterWriter(writer, bytesPerSecond, context.Background())
}

// NewCompressionReader 创建压缩读取器
func (sf *DefaultStreamFactory) NewCompressionReader(reader io.Reader) *stream.GzipReader {
	return stream.NewGzipReader(reader, context.Background())
}

// NewCompressionWriter 创建压缩写入器
func (sf *DefaultStreamFactory) NewCompressionWriter(writer io.Writer) *stream.GzipWriter {
	return stream.NewGzipWriter(writer, context.Background())
}

// GetConfig 获取配置
func (sf *DefaultStreamFactory) GetConfig() *StreamFactoryConfig {
	return sf.config
}

// SetConfig 设置配置
func (sf *DefaultStreamFactory) SetConfig(config *StreamFactoryConfig) {
	sf.config = config
}
