package processor

import (
	"io"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/stream/compression"
)

// StreamProcessor 流处理器接口
type StreamProcessor interface {
	// ReadPacket 读取数据包
	ReadPacket() (*packet.TransferPacket, int, error)

	// WritePacket 写入数据包
	WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error)

	// ReadExact 读取指定长度的数据
	ReadExact(length int) ([]byte, error)

	// WriteExact 写入指定长度的数据
	WriteExact(data []byte) error

	// GetReader 获取读取器
	GetReader() io.Reader

	// GetWriter 获取写入器
	GetWriter() io.Writer

	// Close 关闭流
	Close()
}

// DefaultStreamProcessor 默认流处理器实现
type DefaultStreamProcessor struct {
	reader            io.Reader
	writer            io.Writer
	compressionReader *compression.GzipReader
	compressionWriter *compression.GzipWriter
	rateLimiter       *stream.RateLimiter
	// 注意：加密功能已移至 internal/stream/transform 模块
}

// NewDefaultStreamProcessor 创建新的默认流处理器
func NewDefaultStreamProcessor(
	reader io.Reader,
	writer io.Writer,
	compressionReader *compression.GzipReader,
	compressionWriter *compression.GzipWriter,
	rateLimiter interface{}, // 改为interface{}以兼容旧代码
) *DefaultStreamProcessor {
	var streamRateLimiter *stream.RateLimiter
	if rateLimiter != nil {
		// 如果需要限流，可以在这里创建stream.RateLimiter
		// 暂时设为nil，由外部处理
	}

	return &DefaultStreamProcessor{
		reader:            reader,
		writer:            writer,
		compressionReader: compressionReader,
		compressionWriter: compressionWriter,
		rateLimiter:       streamRateLimiter,
		// 注意：加密功能已移至 internal/stream/transform 模块
	}
}

// ReadPacket 读取数据包
func (sp *DefaultStreamProcessor) ReadPacket() (*packet.TransferPacket, int, error) {
	// 实现数据包读取逻辑
	// 这里应该包含解压缩、解密等处理
	return nil, 0, nil
}

// WritePacket 写入数据包
func (sp *DefaultStreamProcessor) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	// 实现数据包写入逻辑
	// 这里应该包含压缩、加密、限流等处理
	return 0, nil
}

// ReadExact 读取指定长度的数据
func (sp *DefaultStreamProcessor) ReadExact(length int) ([]byte, error) {
	data := make([]byte, length)
	_, err := io.ReadFull(sp.reader, data)
	return data, err
}

// WriteExact 写入指定长度的数据
func (sp *DefaultStreamProcessor) WriteExact(data []byte) error {
	_, err := sp.writer.Write(data)
	return err
}

// GetReader 获取读取器
func (sp *DefaultStreamProcessor) GetReader() io.Reader {
	return sp.reader
}

// GetWriter 获取写入器
func (sp *DefaultStreamProcessor) GetWriter() io.Writer {
	return sp.writer
}

// Close 关闭流
func (sp *DefaultStreamProcessor) Close() {
	if sp.compressionReader != nil {
		sp.compressionReader.Close()
	}
	if sp.compressionWriter != nil {
		sp.compressionWriter.Close()
	}
	if sp.rateLimiter != nil {
		sp.rateLimiter.Close()
	}
}
