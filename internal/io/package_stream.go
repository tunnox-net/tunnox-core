package io

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"sync"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

type PackageStream struct {
	reader    io.Reader
	writer    io.Writer
	transLock sync.Mutex
	utils.Dispose
}

func NewPackageStream(reader io.Reader, writer io.Writer, parentCtx context.Context) *PackageStream {
	stream := &PackageStream{reader: reader, writer: writer}
	stream.SetCtx(parentCtx, nil)
	return stream
}

// readLock 获取读取锁并检查状态
func (ps *PackageStream) readLock() error {
	if ps.IsClosed() {
		return io.EOF
	}
	ps.transLock.Lock()
	if ps.reader == nil {
		ps.transLock.Unlock()
		return utils.ErrReaderNil
	}
	return nil
}

// writeLock 获取写入锁并检查状态
func (ps *PackageStream) writeLock() error {
	if ps.IsClosed() {
		return utils.ErrStreamClosed
	}
	ps.transLock.Lock()
	if ps.writer == nil {
		ps.transLock.Unlock()
		return utils.ErrWriterNil
	}
	return nil
}

// ReadExact 读取指定长度的字节，直到读完为止
// 如果读取的字节数不足指定长度，会继续读取直到达到指定长度或遇到错误
func (ps *PackageStream) ReadExact(length int) ([]byte, error) {
	if err := ps.readLock(); err != nil {
		return nil, err
	}
	defer ps.transLock.Unlock()

	// 创建指定长度的缓冲区
	buffer := make([]byte, length)
	totalRead := 0

	// 循环读取，直到读取到指定长度的数据
	for totalRead < length {
		// 检查上下文是否已取消
		select {
		case <-ps.Ctx().Done():
			return nil, ps.Ctx().Err()
		default:
		}

		// 读取剩余的数据
		n, err := ps.reader.Read(buffer[totalRead:])
		totalRead += n

		// 如果遇到错误且不是EOF，或者已经读取完毕，则返回
		if err != nil {
			if err == io.EOF && totalRead == length {
				// 读取完毕且达到指定长度，返回成功
				return buffer, nil
			}
			// 其他错误或EOF但未达到指定长度，返回错误
			return buffer[:totalRead], err
		}

		// 如果没有读取到任何数据，可能是阻塞，继续尝试
		// 但是对于 bytes.Buffer，如果 n == 0 且没有错误，说明没有更多数据
		if n == 0 {
			// 检查是否已经读取了部分数据
			if totalRead > 0 {
				// 已经读取了部分数据，但无法继续读取，返回部分数据
				return buffer[:totalRead], utils.ErrUnexpectedEOF
			}
			// 没有读取到任何数据，可能是阻塞，继续尝试
			continue
		}
	}

	return buffer, nil
}

// WriteExact 写入指定长度的字节，直到写完为止
// 如果写入的字节数不足指定长度，会继续写入直到达到指定长度或遇到错误
func (ps *PackageStream) WriteExact(data []byte) error {
	if err := ps.writeLock(); err != nil {
		return err
	}
	defer ps.transLock.Unlock()

	totalWritten := 0
	dataLength := len(data)

	// 循环写入，直到写入指定长度的数据
	for totalWritten < dataLength {
		// 检查上下文是否已取消
		select {
		case <-ps.Ctx().Done():
			return ps.Ctx().Err()
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

// readPacketType 读取包类型
func (ps *PackageStream) readPacketType() (packet.Type, error) {
	typeBuffer := make([]byte, utils.PacketTypeSize)
	n, err := ps.reader.Read(typeBuffer)
	if err != nil {
		return 0, utils.NewStreamError("read_packet_type", "failed to read packet type", err)
	}
	if n != utils.PacketTypeSize {
		return 0, utils.ErrUnexpectedEOF
	}
	return packet.Type(typeBuffer[0]), nil
}

// readPacketBodySize 读取包体大小
func (ps *PackageStream) readPacketBodySize() (uint32, error) {
	sizeBuffer := make([]byte, utils.PacketBodySizeBytes)
	n, err := ps.reader.Read(sizeBuffer)
	if err != nil {
		return 0, utils.NewStreamError("read_packet_body_size", "failed to read packet body size", err)
	}
	if n != utils.PacketBodySizeBytes {
		return 0, utils.ErrUnexpectedEOF
	}
	return binary.BigEndian.Uint32(sizeBuffer), nil
}

// readPacketBody 读取包体数据
func (ps *PackageStream) readPacketBody(bodySize uint32) ([]byte, error) {
	bodyData := make([]byte, int(bodySize))
	totalRead := 0
	for totalRead < int(bodySize) {
		n, err := ps.reader.Read(bodyData[totalRead:])
		if err != nil {
			return nil, utils.NewStreamError("read_packet_body", "failed to read packet body", err)
		}
		totalRead += n
	}
	return bodyData, nil
}

// decompressData 解压数据
func (ps *PackageStream) decompressData(compressedData []byte) ([]byte, error) {
	gzipReader := NewGzipReader(bytes.NewReader(compressedData), ps.Ctx())
	defer gzipReader.Close()

	var decompressedData bytes.Buffer
	_, err := io.Copy(&decompressedData, gzipReader)
	if err != nil {
		return nil, utils.NewCompressionError("decompress", "decompression failed", err)
	}

	return decompressedData.Bytes(), nil
}

// ReadPacket 读取整个数据包，返回读取的字节数
func (ps *PackageStream) ReadPacket() (*packet.TransferPacket, int, error) {
	if err := ps.readLock(); err != nil {
		return nil, 0, err
	}
	defer ps.transLock.Unlock()

	totalBytes := 0

	// 读取包类型字节
	packetType, err := ps.readPacketType()
	if err != nil {
		return nil, totalBytes, err
	}
	totalBytes += utils.PacketTypeSize

	// 如果是心跳包，直接返回
	if packetType.IsHeartbeat() {
		return &packet.TransferPacket{
			PacketType:    packetType,
			CommandPacket: nil,
		}, totalBytes, nil
	}

	// 如果是JsonCommand包，读取数据体大小和数据体
	if packetType.IsJsonCommand() {
		// 读取数据体大小
		bodySize, err := ps.readPacketBodySize()
		if err != nil {
			return nil, totalBytes, err
		}
		totalBytes += utils.PacketBodySizeBytes

		// 读取数据体
		bodyData, err := ps.readPacketBody(bodySize)
		if err != nil {
			return nil, totalBytes, err
		}
		totalBytes += len(bodyData)

		// 如果数据被压缩，需要解压
		if packetType.IsCompressed() {
			bodyData, err = ps.decompressData(bodyData)
			if err != nil {
				return nil, totalBytes, err
			}
		}

		// 解析 CommandPacket
		var commandPacket packet.CommandPacket
		err = json.Unmarshal(bodyData, &commandPacket)
		if err != nil {
			return nil, totalBytes, utils.NewPacketError("json_unmarshal", "failed to unmarshal command packet", err)
		}

		return &packet.TransferPacket{
			PacketType:    packetType,
			CommandPacket: &commandPacket,
		}, totalBytes, nil
	}

	// 其他类型的包，暂时不支持
	return &packet.TransferPacket{
		PacketType:    packetType,
		CommandPacket: nil,
	}, totalBytes, nil
}

// compressData 压缩数据
func (ps *PackageStream) compressData(data []byte) ([]byte, error) {
	var compressedData bytes.Buffer
	gzipWriter := NewGzipWriter(&compressedData, ps.Ctx())

	_, err := gzipWriter.Write(data)
	if err != nil {
		gzipWriter.Close()
		return nil, utils.NewCompressionError("compress", "compression failed", err)
	}

	// 确保所有数据都写入并关闭
	gzipWriter.Close()

	return compressedData.Bytes(), nil
}

// writeRateLimitedData 限速写入数据
func (ps *PackageStream) writeRateLimitedData(data []byte, rateLimitBytesPerSecond int64) error {
	// 使用限速写入器
	rateLimitedWriter, err := NewRateLimiterWriter(ps.writer, rateLimitBytesPerSecond, ps.Ctx())
	if err != nil {
		return utils.NewStreamError("write_rate_limited", "failed to create rate limited writer", err)
	}
	defer rateLimitedWriter.Close()

	// 分块写入，确保所有数据都被写入
	totalWritten := 0
	for totalWritten < len(data) {
		n, err := rateLimitedWriter.Write(data[totalWritten:])
		if err != nil {
			return utils.NewStreamError("write_rate_limited", "rate limited write failed", err)
		}
		totalWritten += n
	}
	return nil
}

// WritePacket 写入整个数据包，返回写入的字节数
func (ps *PackageStream) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	if err := ps.writeLock(); err != nil {
		return 0, err
	}
	defer ps.transLock.Unlock()

	totalBytes := 0

	// 写入包类型字节
	packetType := pkt.PacketType
	if useCompression {
		packetType |= packet.Compressed
	}

	typeByte := []byte{byte(packetType)}
	_, err := ps.writer.Write(typeByte)
	if err != nil {
		return totalBytes, utils.NewStreamError("write_packet_type", "failed to write packet type", err)
	}
	totalBytes += utils.PacketTypeSize

	// 如果是心跳包，写入完成
	if packetType.IsHeartbeat() {
		return totalBytes, nil
	}

	// 如果是JsonCommand包，写入数据体
	if packetType.IsJsonCommand() && pkt.CommandPacket != nil {
		// 将 CommandPacket 序列化为 JSON 字节数据
		bodyData, err := json.Marshal(pkt.CommandPacket)
		if err != nil {
			return totalBytes, utils.NewPacketError("json_marshal", "failed to marshal command packet", err)
		}

		// 如果需要压缩，先压缩数据
		if useCompression {
			bodyData, err = ps.compressData(bodyData)
			if err != nil {
				return totalBytes, err
			}
		}

		// 写入数据体大小
		sizeBuffer := make([]byte, utils.PacketBodySizeBytes)
		binary.BigEndian.PutUint32(sizeBuffer, uint32(len(bodyData)))
		_, err = ps.writer.Write(sizeBuffer)
		if err != nil {
			return totalBytes, utils.NewStreamError("write_packet_body_size", "failed to write packet body size", err)
		}
		totalBytes += utils.PacketBodySizeBytes

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
				return totalBytes, utils.NewStreamError("write_packet_body", "failed to write packet body", err)
			}
		}
		totalBytes += len(bodyData)
	}

	return totalBytes, nil
}
