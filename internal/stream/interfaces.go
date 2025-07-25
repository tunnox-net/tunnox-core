package stream

import (
	"io"
	"tunnox-core/internal/packet"
)

// PackageStreamer 数据包流接口
type PackageStreamer interface {
	// ReadPacket 读取数据包
	ReadPacket() (*packet.TransferPacket, int, error)

	// WritePacket 写入数据包
	WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error)

	// ReadExact 读取指定长度的数据
	ReadExact(length int) ([]byte, error)

	// WriteExact 写入指定长度的数据
	WriteExact(data []byte) error

	GetReader() io.Reader

	GetWriter() io.Writer

	// Close 关闭流
	Close()
}

// StreamFactory 流工厂接口
type StreamFactory interface {
	// NewStreamProcessor 创建新的数据包流
	NewStreamProcessor(reader io.Reader, writer io.Writer) PackageStreamer

	// NewRateLimiterReader 创建限速读取器
	NewRateLimiterReader(reader io.Reader, bytesPerSecond int64) (*RateLimiterReader, error)

	// NewRateLimiterWriter 创建限速写入器
	NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64) (*RateLimiterWriter, error)

	// NewCompressionReader 创建压缩读取器
	NewCompressionReader(reader io.Reader) *GzipReader

	// NewCompressionWriter 创建压缩写入器
	NewCompressionWriter(writer io.Writer) *GzipWriter
}
