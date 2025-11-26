package stream

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/errors"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream/compression"
	"tunnox-core/internal/utils"
)

// StreamProcessor 流处理器
type StreamProcessor struct {
	*dispose.ManagerBase
	reader    io.Reader
	writer    io.Writer
	readLock  sync.Mutex // 独立的读锁
	writeLock sync.Mutex // 独立的写锁
	bufferMgr *utils.BufferManager
	// 注意：加密功能已移至 internal/stream/transform 模块
}

func (ps *StreamProcessor) GetReader() io.Reader {
	return ps.reader
}

func (ps *StreamProcessor) GetWriter() io.Writer {
	return ps.writer
}

func NewStreamProcessor(reader io.Reader, writer io.Writer, parentCtx context.Context) *StreamProcessor {
	sp := &StreamProcessor{
		ManagerBase: dispose.NewManager("StreamProcessor", parentCtx),
		reader:      reader,
		writer:      writer,
		bufferMgr:   utils.NewBufferManager(parentCtx),
		// 注意：加密功能已移至 internal/stream/transform 模块
	}
	return sp
}

// NewStreamProcessorWithEncryption 已废弃：加密功能已移至 internal/stream/transform 模块
// 使用 transform.NewTransformer() 配置加密
//
// Deprecated: 使用 transform 包代替
func NewStreamProcessorWithEncryption(reader io.Reader, writer io.Writer, key []byte, parentCtx context.Context) *StreamProcessor {
	// 直接创建基本流处理器，加密功能应通过 transform 模块配置
	return NewStreamProcessor(reader, writer, parentCtx)
}

func (ps *StreamProcessor) onClose() error {
	if ps.bufferMgr != nil {
		result := ps.bufferMgr.Close()
		if result.HasErrors() {
			return fmt.Errorf("buffer manager cleanup failed: %v", result.Error())
		}
	}
	return nil
}

// acquireReadLock 获取读取锁并检查状态
func (ps *StreamProcessor) acquireReadLock() error {
	if ps.ResourceBase.Dispose.IsClosed() {
		return io.EOF
	}
	ps.readLock.Lock()
	if ps.reader == nil {
		ps.readLock.Unlock()
		return errors.ErrReaderNil
	}
	return nil
}

// acquireWriteLock 获取写入锁并检查状态
func (ps *StreamProcessor) acquireWriteLock() error {
	if ps.ResourceBase.Dispose.IsClosed() {
		return errors.ErrStreamClosed
	}
	ps.writeLock.Lock()
	if ps.writer == nil {
		ps.writeLock.Unlock()
		return errors.ErrWriterNil
	}
	return nil
}

// ReadExact 读取指定长度的字节，使用内存池优化
// 如果读取的字节数不足指定长度，会继续读取直到达到指定长度或遇到错误
func (ps *StreamProcessor) ReadExact(length int) ([]byte, error) {
	if err := ps.acquireReadLock(); err != nil {
		return nil, err
	}
	defer ps.readLock.Unlock()

	// 从内存池获取缓冲区
	buffer := ps.bufferMgr.Allocate(length)
	defer ps.bufferMgr.Release(buffer)

	totalRead := 0

	// 循环读取，直到读取到指定长度的数据
	for totalRead < length {
		// 检查上下文是否已取消
		select {
		case <-ps.ResourceBase.Dispose.Ctx().Done():
			return nil, ps.ResourceBase.Dispose.Ctx().Err()
		default:
		}

		// 读取剩余的数据
		n, err := ps.reader.Read(buffer[totalRead:])
		totalRead += n

		// 如果遇到错误且不是EOF，或者已经读取完毕，则返回
		if err != nil {
			if err == io.EOF && totalRead == length {
				// 读取完毕且达到指定长度，返回成功
				// 创建新的切片，避免内存池中的数据被修改
				result := make([]byte, totalRead)
				copy(result, buffer[:totalRead])
				return result, nil
			}
			// 其他错误或EOF但未达到指定长度，返回错误
			result := make([]byte, totalRead)
			copy(result, buffer[:totalRead])
			return result, err
		}

		// 如果没有读取到任何数据，可能是阻塞，继续尝试
		if n == 0 {
			// 检查是否已经读取了部分数据
			if totalRead > 0 {
				// 已经读取了部分数据，但无法继续读取，返回部分数据
				result := make([]byte, totalRead)
				copy(result, buffer[:totalRead])
				return result, errors.ErrUnexpectedEOF
			}
			// 没有读取到任何数据，可能是阻塞，继续尝试
			continue
		}
	}

	// 创建新的切片，避免内存池中的数据被修改
	result := make([]byte, totalRead)
	copy(result, buffer[:totalRead])
	return result, nil
}

// ReadExactZeroCopy 零拷贝读取指定长度的字节
// 返回零拷贝缓冲区和清理函数，调用方负责调用清理函数
func (ps *StreamProcessor) ReadExactZeroCopy(length int) (*utils.ZeroCopyBuffer, error) {
	if err := ps.acquireReadLock(); err != nil {
		return nil, err
	}
	defer ps.readLock.Unlock()

	// 从内存池获取缓冲区
	buffer := ps.bufferMgr.Allocate(length)

	totalRead := 0

	// 循环读取，直到读取到指定长度的数据
	for totalRead < length {
		// 检查上下文是否已取消
		select {
		case <-ps.ResourceBase.Dispose.Ctx().Done():
			ps.bufferMgr.Release(buffer)
			return nil, ps.ResourceBase.Dispose.Ctx().Err()
		default:
		}

		// 读取剩余的数据
		n, err := ps.reader.Read(buffer[totalRead:])
		totalRead += n

		// 如果遇到错误且不是EOF，或者已经读取完毕，则返回
		if err != nil {
			if err == io.EOF && totalRead == length {
				// 读取完毕且达到指定长度，返回成功
				return utils.NewZeroCopyBuffer(buffer[:totalRead], ps.bufferMgr.GetPool()), nil
			}
			// 其他错误或EOF但未达到指定长度，返回错误
			ps.bufferMgr.Release(buffer)
			return nil, err
		}

		// 如果没有读取到任何数据，可能是阻塞，继续尝试
		if n == 0 {
			// 检查是否已经读取了部分数据
			if totalRead > 0 {
				// 已经读取了部分数据，但无法继续读取，返回部分数据
				ps.bufferMgr.Release(buffer)
				return nil, errors.ErrUnexpectedEOF
			}
			// 没有读取到任何数据，可能是阻塞，继续尝试
			continue
		}
	}

	return utils.NewZeroCopyBuffer(buffer[:totalRead], ps.bufferMgr.GetPool()), nil
}

// WriteExact 写入指定长度的字节，直到写完为止
// 如果写入的字节数不足指定长度，会继续写入直到达到指定长度或遇到错误
func (ps *StreamProcessor) WriteExact(data []byte) error {
	if err := ps.acquireWriteLock(); err != nil {
		return err
	}
	defer ps.writeLock.Unlock()

	totalWritten := 0
	dataLength := len(data)

	// 循环写入，直到写入指定长度的数据
	for totalWritten < dataLength {
		// 检查上下文是否已取消
		select {
		case <-ps.ResourceBase.Dispose.Ctx().Done():
			return ps.ResourceBase.Dispose.Ctx().Err()
		default:
		}

		// 写入剩余的数据
		n, err := ps.writer.Write(data[totalWritten:])
		totalWritten += n

		// 如果遇到错误，返回错误
		if err != nil {
			return err
		}

		// 如果没有写入任何数据，可能是阻塞，继续尝试
		if n == 0 {
			continue
		}
	}

	return nil
}

// readPacketType 读取包类型，使用内存池
func (ps *StreamProcessor) readPacketType() (packet.Type, error) {
	typeBuffer := ps.bufferMgr.Allocate(constants.PacketTypeSize)
	defer ps.bufferMgr.Release(typeBuffer)

	n, err := ps.reader.Read(typeBuffer)
	if err != nil {
		return 0, errors.NewStreamError("read_packet_type", "failed to read packet type", err)
	}
	if n != constants.PacketTypeSize {
		return 0, errors.ErrUnexpectedEOF
	}
	return packet.Type(typeBuffer[0]), nil
}

// readPacketBodySize 读取包体大小，使用内存池
func (ps *StreamProcessor) readPacketBodySize() (uint32, error) {
	sizeBuffer := ps.bufferMgr.Allocate(constants.PacketBodySizeBytes)
	defer ps.bufferMgr.Release(sizeBuffer)

	n, err := ps.reader.Read(sizeBuffer)
	if err != nil {
		return 0, errors.NewStreamError("read_packet_body_size", "failed to read packet body size", err)
	}
	if n != constants.PacketBodySizeBytes {
		return 0, errors.ErrUnexpectedEOF
	}
	return binary.BigEndian.Uint32(sizeBuffer), nil
}

// readPacketBody 读取包体数据，使用内存池
func (ps *StreamProcessor) readPacketBody(bodySize uint32) ([]byte, error) {
	bodyData := ps.bufferMgr.Allocate(int(bodySize))
	defer ps.bufferMgr.Release(bodyData)

	totalRead := 0
	for totalRead < int(bodySize) {
		n, err := ps.reader.Read(bodyData[totalRead:])
		if err != nil {
			return nil, errors.NewStreamError("read_packet_body", "failed to read packet body", err)
		}
		totalRead += n
	}

	// 创建新的切片，避免内存池中的数据被修改
	result := make([]byte, totalRead)
	copy(result, bodyData[:totalRead])
	return result, nil
}

// decompressData 解压数据
func (ps *StreamProcessor) decompressData(compressedData []byte) ([]byte, error) {
	gzipReader := compression.NewGzipReader(bytes.NewReader(compressedData), ps.ResourceBase.Dispose.Ctx())
	defer gzipReader.Close()

	var decompressedData bytes.Buffer
	_, err := io.Copy(&decompressedData, gzipReader)
	if err != nil {
		return nil, errors.NewCompressionError("decompress", "decompression failed", err)
	}

	return decompressedData.Bytes(), nil
}

// ReadPacket 读取整个数据包，返回读取的字节数
func (ps *StreamProcessor) ReadPacket() (*packet.TransferPacket, int, error) {
	if err := ps.acquireReadLock(); err != nil {
		return nil, 0, err
	}
	defer ps.readLock.Unlock()

	totalBytes := 0

	// 读取包类型字节
	packetType, err := ps.readPacketType()
	if err != nil {
		return nil, totalBytes, err
	}
	totalBytes += constants.PacketTypeSize

	// 如果是心跳包，直接返回
	if packetType.IsHeartbeat() {
		return &packet.TransferPacket{
			PacketType:    packetType,
			CommandPacket: nil,
		}, totalBytes, nil
	}

	// 读取数据体大小和数据体（JsonCommand 和其他有 Payload 的类型）
	bodySize, err := ps.readPacketBodySize()
	if err != nil {
		return nil, totalBytes, err
	}
	totalBytes += constants.PacketBodySizeBytes

	// 读取数据体
	bodyData, err := ps.readPacketBody(bodySize)
	if err != nil {
		return nil, totalBytes, err
	}
	totalBytes += len(bodyData)

	// 注意：加密功能已移至 internal/stream/transform 模块
	// 解密功能应通过 transform.StreamTransformer 处理
	if packetType.IsEncrypted() {
		// 加密功能已移至 transform 模块
		err = fmt.Errorf("encryption not supported in StreamProcessor, use transform package")
		if err != nil {
			return nil, totalBytes, err
		}
	}
	if packetType.IsCompressed() {
		bodyData, err = ps.decompressData(bodyData)
		if err != nil {
			return nil, totalBytes, err
		}
	}

	// 如果是JsonCommand包，解析为 CommandPacket
	if packetType.IsJsonCommand() {
		var commandPacket packet.CommandPacket
		err = json.Unmarshal(bodyData, &commandPacket)
		if err != nil {
			return nil, totalBytes, errors.NewPacketError("json_unmarshal", "failed to unmarshal command packet", err)
		}

		return &packet.TransferPacket{
			PacketType:    packetType,
			CommandPacket: &commandPacket,
		}, totalBytes, nil
	}

	// 其他类型的包（Handshake, HandshakeResp, TunnelOpen等），返回 Payload
	return &packet.TransferPacket{
		PacketType:    packetType,
		Payload:       bodyData,
		CommandPacket: nil,
	}, totalBytes, nil
}

// compressData 压缩数据
func (ps *StreamProcessor) compressData(data []byte) ([]byte, error) {
	var compressedData bytes.Buffer
	gzipWriter := compression.NewGzipWriter(&compressedData, ps.ResourceBase.Dispose.Ctx())

	_, err := gzipWriter.Write(data)
	if err != nil {
		gzipWriter.Close()
		return nil, errors.NewCompressionError("compress", "compression failed", err)
	}

	// 确保所有数据都写入并关闭
	gzipWriter.Close()

	return compressedData.Bytes(), nil
}

// writeRateLimitedData 限速写入数据
func (ps *StreamProcessor) writeRateLimitedData(data []byte, rateLimitBytesPerSecond int64) error {
	// 使用限速写入器
	rateLimitedWriter, err := NewRateLimiterWriter(ps.writer, rateLimitBytesPerSecond, ps.ResourceBase.Dispose.Ctx())
	if err != nil {
		return errors.NewStreamError("write_rate_limited", "failed to create rate limited writer", err)
	}
	defer rateLimitedWriter.Close()

	// 分块写入，确保所有数据都被写入
	totalWritten := 0
	for totalWritten < len(data) {
		n, err := rateLimitedWriter.Write(data[totalWritten:])
		if err != nil {
			return errors.NewStreamError("write_rate_limited", "rate limited write failed", err)
		}
		totalWritten += n
	}
	return nil
}

// WritePacket 写入整个数据包，返回写入的字节数
func (ps *StreamProcessor) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	if err := ps.acquireWriteLock(); err != nil {
		return 0, err
	}
	defer ps.writeLock.Unlock()

	// 检查数据包是否为nil
	if pkt == nil {
		return 0, errors.NewPacketError("write_packet", "packet is nil", nil)
	}

	totalBytes := 0

	// 写入包类型字节
	packetType := pkt.PacketType
	if useCompression {
		packetType |= packet.Compressed
	}
	// 注意：加密功能已移至 internal/stream/transform 模块
	// 加密标志位的设置应通过 transform 模块处理
	// 此处保留代码兼容性，但不再使用
	if false { // encMgr 已移除
		packetType |= packet.Encrypted
	}

	typeByte := []byte{byte(packetType)}
	_, err := ps.writer.Write(typeByte)
	if err != nil {
		return totalBytes, errors.NewStreamError("write_packet_type", "failed to write packet type", err)
	}
	totalBytes += constants.PacketTypeSize

	// 如果是心跳包，写入完成
	if packetType.IsHeartbeat() {
		return totalBytes, nil
	}

	// 准备要写入的数据体
	var bodyData []byte

	// 如果是JsonCommand包，序列化 CommandPacket
	if packetType.IsJsonCommand() && pkt.CommandPacket != nil {
		// 将 CommandPacket 序列化为 JSON 字节数据
		var err error
		bodyData, err = json.Marshal(pkt.CommandPacket)
		if err != nil {
			return totalBytes, errors.NewPacketError("json_marshal", "failed to marshal command packet", err)
		}
	} else if len(pkt.Payload) > 0 {
		// 对于其他类型（Handshake, HandshakeResp, TunnelOpen等），使用 Payload
		bodyData = pkt.Payload
	}

	// 如果没有数据体，直接返回
	if len(bodyData) == 0 {
		return totalBytes, nil
	}

	// 先压缩，再加密
	if useCompression {
		var err error
		bodyData, err = ps.compressData(bodyData)
		if err != nil {
			return totalBytes, err
		}
	}

	// 写入数据体大小
	sizeBuffer := make([]byte, constants.PacketBodySizeBytes)
	binary.BigEndian.PutUint32(sizeBuffer, uint32(len(bodyData)))
	_, err = ps.writer.Write(sizeBuffer)
	if err != nil {
		return totalBytes, errors.NewStreamError("write_packet_body_size", "failed to write packet body size", err)
	}
	totalBytes += constants.PacketBodySizeBytes

	// 写入数据体，根据限速参数决定是否使用限速
	if rateLimitBytesPerSecond > 0 {
		err = ps.writeRateLimitedData(bodyData, rateLimitBytesPerSecond)
		if err != nil {
			return totalBytes, err
		}
	} else {
		// 直接写入，不限速
		_, err = ps.writer.Write(bodyData)
		if err != nil {
			return totalBytes, errors.NewStreamError("write_packet_body", "failed to write packet body", err)
		}
	}
	totalBytes += len(bodyData)

	// ✅ Flush writer if it implements Flusher interface (for buffered writers like GzipWriter)
	if flusher, ok := ps.writer.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return totalBytes, errors.NewStreamError("flush_writer", "failed to flush writer", err)
		}
	}

	return totalBytes, nil
}

// Close 关闭流处理器（兼容接口）
func (ps *StreamProcessor) Close() {
	ps.ResourceBase.Dispose.Close()
}

// CloseWithResult 关闭并返回结果（新方法）
func (ps *StreamProcessor) CloseWithResult() *dispose.DisposeResult {
	return ps.ResourceBase.Dispose.Close()
}

// EnableEncryption 已废弃：加密功能已移至 internal/stream/transform 模块
//
// Deprecated: 使用 transform 包代替
func (ps *StreamProcessor) EnableEncryption(key []byte) {
	// 加密功能已移至 transform 模块，此方法为兼容性保留
}

// DisableEncryption 禁用加密
// DisableEncryption 已废弃：加密功能已移至 internal/stream/transform 模块
//
// Deprecated: 使用 transform 包代替
func (ps *StreamProcessor) DisableEncryption() {
	// 加密功能已移至 transform 模块，此方法为兼容性保留
}

// IsEncryptionEnabled 检查是否启用了加密
// IsEncryptionEnabled 已废弃：加密功能已移至 internal/stream/transform 模块
//
// Deprecated: 使用 transform 包代替
func (ps *StreamProcessor) IsEncryptionEnabled() bool {
	// 加密功能已移至 transform 模块，此方法为兼容性保留
	return false
}

// GetEncryptionKey 已废弃：加密功能已移至 internal/stream/transform 模块
//
// Deprecated: 使用 transform 包代替
func (ps *StreamProcessor) GetEncryptionKey() []byte {
	// 加密功能已移至 transform 模块，此方法为兼容性保留
	return nil
}
