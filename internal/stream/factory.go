package io

import (
	"context"
	"io"
)

// DefaultStreamFactory 默认流工厂实现
type DefaultStreamFactory struct{}

// NewDefaultStreamFactory 创建默认流工厂
func NewDefaultStreamFactory() *DefaultStreamFactory {
	return &DefaultStreamFactory{}
}

// NewPackageStream 创建新的数据包流
func (f *DefaultStreamFactory) NewPackageStream(reader io.Reader, writer io.Writer, ctx context.Context) PackageStreamer {
	return NewPackageStream(reader, writer, ctx)
}

// NewRateLimiterReader 创建限速读取器
func (f *DefaultStreamFactory) NewRateLimiterReader(reader io.Reader, bytesPerSecond int64, ctx context.Context) (RateLimiterReaderInterface, error) {
	return NewRateLimiterReader(reader, bytesPerSecond, ctx)
}

// NewRateLimiterWriter 创建限速写入器
func (f *DefaultStreamFactory) NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64, ctx context.Context) (RateLimiterWriterInterface, error) {
	return NewRateLimiterWriter(writer, bytesPerSecond, ctx)
}

// NewCompressionReader 创建压缩读取器
func (f *DefaultStreamFactory) NewCompressionReader(reader io.Reader, ctx context.Context) CompressionReader {
	return NewGzipReader(reader, ctx)
}

// NewCompressionWriter 创建压缩写入器
func (f *DefaultStreamFactory) NewCompressionWriter(writer io.Writer, ctx context.Context) CompressionWriter {
	return NewGzipWriter(writer, ctx)
}
