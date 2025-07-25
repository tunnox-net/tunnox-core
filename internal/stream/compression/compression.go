package compression

import (
	"io"
)

// CompressionReader 压缩读取器接口
type CompressionReader interface {
	// Read 实现io.Reader接口
	Read(p []byte) (n int, err error)

	// Close 关闭读取器
	Close()
}

// CompressionWriter 压缩写入器接口
type CompressionWriter interface {
	// Write 实现io.Writer接口
	Write(p []byte) (n int, err error)

	// Close 关闭写入器
	Close()
}

// CompressionFactory 压缩工厂接口
type CompressionFactory interface {
	// NewCompressionReader 创建压缩读取器
	NewCompressionReader(reader io.Reader) CompressionReader

	// NewCompressionWriter 创建压缩写入器
	NewCompressionWriter(writer io.Writer) CompressionWriter
}

// DefaultCompressionFactory 默认压缩工厂实现
type DefaultCompressionFactory struct{}

// NewDefaultCompressionFactory 创建新的默认压缩工厂
func NewDefaultCompressionFactory() *DefaultCompressionFactory {
	return &DefaultCompressionFactory{}
}

// NewCompressionReader 创建压缩读取器
func (f *DefaultCompressionFactory) NewCompressionReader(reader io.Reader) CompressionReader {
	// 这里应该实现具体的压缩读取器
	return &NoCompressionReader{Reader: reader}
}

// NewCompressionWriter 创建压缩写入器
func (f *DefaultCompressionFactory) NewCompressionWriter(writer io.Writer) CompressionWriter {
	// 这里应该实现具体的压缩写入器
	return &NoCompressionWriter{Writer: writer}
}

// NoCompressionReader 无压缩读取器
type NoCompressionReader struct {
	Reader io.Reader
}

func (r *NoCompressionReader) Read(p []byte) (n int, err error) {
	return r.Reader.Read(p)
}

func (r *NoCompressionReader) Close() {
	// 无压缩读取器不需要特殊关闭逻辑
}

// NoCompressionWriter 无压缩写入器
type NoCompressionWriter struct {
	Writer io.Writer
}

func (w *NoCompressionWriter) Write(p []byte) (n int, err error) {
	return w.Writer.Write(p)
}

func (w *NoCompressionWriter) Close() {
	// 无压缩写入器不需要特殊关闭逻辑
}
