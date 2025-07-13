package stream

import (
	"context"
	"io"
)

// DefaultStreamFactory 默认流工厂实现
type DefaultStreamFactory struct {
	ctx context.Context
}

// NewDefaultStreamFactory 创建默认流工厂
func NewDefaultStreamFactory(ctx context.Context) *DefaultStreamFactory {
	return &DefaultStreamFactory{
		ctx: ctx,
	}
}

// NewStreamProcessor 创建新的数据包流处理器
func (f *DefaultStreamFactory) NewStreamProcessor(reader io.Reader, writer io.Writer) PackageStreamer {
	return NewStreamProcessor(reader, writer, f.ctx)
}

// NewRateLimiterReader 创建限速读取器
func (f *DefaultStreamFactory) NewRateLimiterReader(reader io.Reader, bytesPerSecond int64) (RateLimiterReaderInterface, error) {
	return NewRateLimiterReader(reader, bytesPerSecond, f.ctx)
}

// NewRateLimiterWriter 创建限速写入器
func (f *DefaultStreamFactory) NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64) (RateLimiterWriterInterface, error) {
	return NewRateLimiterWriter(writer, bytesPerSecond, f.ctx)
}

// NewCompressionReader 创建压缩读取器
func (f *DefaultStreamFactory) NewCompressionReader(reader io.Reader) CompressionReader {
	return NewGzipReader(reader, f.ctx)
}

// NewCompressionWriter 创建压缩写入器
func (f *DefaultStreamFactory) NewCompressionWriter(writer io.Writer) CompressionWriter {
	return NewGzipWriter(writer, f.ctx)
}

// StreamFactoryConfig 流工厂配置
type StreamFactoryConfig struct {
	// 默认压缩设置
	DefaultCompression bool
	// 默认限速设置（字节/秒）
	DefaultRateLimit int64
	// 缓冲区大小配置
	BufferSize int
	// 是否启用内存池
	EnableMemoryPool bool
	// 加密配置
	EncryptionKey EncryptionKey
}

// ConfigurableStreamFactory 可配置的流工厂
type ConfigurableStreamFactory struct {
	config StreamFactoryConfig
	ctx    context.Context
}

// NewConfigurableStreamFactory 创建可配置的流工厂
func NewConfigurableStreamFactory(ctx context.Context, config StreamFactoryConfig) *ConfigurableStreamFactory {
	return &ConfigurableStreamFactory{
		config: config,
		ctx:    ctx,
	}
}

// NewStreamProcessor 创建新的数据包流处理器（带配置）
func (f *ConfigurableStreamFactory) NewStreamProcessor(reader io.Reader, writer io.Writer) PackageStreamer {
	processor := NewStreamProcessor(reader, writer, f.ctx)

	// 如果配置了加密密钥，启用加密
	if f.config.EncryptionKey != nil {
		processor.EnableEncryption(f.config.EncryptionKey)
	}

	return processor
}

// NewRateLimiterReader 创建限速读取器（使用默认配置）
func (f *ConfigurableStreamFactory) NewRateLimiterReader(reader io.Reader, bytesPerSecond int64) (RateLimiterReaderInterface, error) {
	if bytesPerSecond <= 0 {
		bytesPerSecond = f.config.DefaultRateLimit
	}
	return NewRateLimiterReader(reader, bytesPerSecond, f.ctx)
}

// NewRateLimiterWriter 创建限速写入器（使用默认配置）
func (f *ConfigurableStreamFactory) NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64) (RateLimiterWriterInterface, error) {
	if bytesPerSecond <= 0 {
		bytesPerSecond = f.config.DefaultRateLimit
	}
	return NewRateLimiterWriter(writer, bytesPerSecond, f.ctx)
}

// NewCompressionReader 创建压缩读取器
func (f *ConfigurableStreamFactory) NewCompressionReader(reader io.Reader) CompressionReader {
	return NewGzipReader(reader, f.ctx)
}

// NewCompressionWriter 创建压缩写入器
func (f *ConfigurableStreamFactory) NewCompressionWriter(writer io.Writer) CompressionWriter {
	return NewGzipWriter(writer, f.ctx)
}

// NewEncryptionReader 创建加密读取器
func (f *ConfigurableStreamFactory) NewEncryptionReader(reader io.Reader) *EncryptionReader {
	if f.config.EncryptionKey != nil {
		return NewEncryptionReader(reader, f.config.EncryptionKey, f.ctx)
	}
	return nil
}

// NewEncryptionWriter 创建加密写入器
func (f *ConfigurableStreamFactory) NewEncryptionWriter(writer io.Writer) *EncryptionWriter {
	if f.config.EncryptionKey != nil {
		return NewEncryptionWriter(writer, f.config.EncryptionKey, f.ctx)
	}
	return nil
}

// NewEncryptionManager 创建加密管理器
func (f *ConfigurableStreamFactory) NewEncryptionManager() *EncryptionManager {
	if f.config.EncryptionKey != nil {
		return NewEncryptionManager(f.config.EncryptionKey, f.ctx)
	}
	return nil
}

// GetConfig 获取工厂配置
func (f *ConfigurableStreamFactory) GetConfig() StreamFactoryConfig {
	return f.config
}
