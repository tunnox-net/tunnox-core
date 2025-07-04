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
		return io.ErrClosedPipe
	}
	return nil
}

// writeLock 获取写入锁并检查状态
func (ps *PackageStream) writeLock() error {
	if ps.IsClosed() {
		return io.ErrClosedPipe
	}
	ps.transLock.Lock()
	if ps.writer == nil {
		ps.transLock.Unlock()
		return io.ErrClosedPipe
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
				return buffer[:totalRead], io.ErrUnexpectedEOF
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

// ReadPacket 读取整个数据包，返回读取的字节数
func (ps *PackageStream) ReadPacket() (*packet.TransferPacket, int, error) {
	if err := ps.readLock(); err != nil {
		return nil, 0, err
	}
	defer ps.transLock.Unlock()

	totalBytes := 0

	// 读取包类型字节 - 直接读取，不调用 ReadExact 避免死锁
	typeBuffer := make([]byte, 1)
	n, err := ps.reader.Read(typeBuffer)
	if err != nil {
		return nil, totalBytes, err
	}
	if n != 1 {
		return nil, totalBytes, io.ErrUnexpectedEOF
	}
	totalBytes += 1

	packetType := packet.Type(typeBuffer[0])

	// 如果是心跳包，直接返回
	if packetType.IsHeartbeat() {
		return &packet.TransferPacket{
			PacketType:    packetType,
			CommandPacket: nil,
		}, totalBytes, nil
	}

	// 如果是JsonCommand包，读取数据体大小和数据体
	if packetType.IsJsonCommand() {
		// 读取4字节的数据体大小 - 直接读取
		sizeBuffer := make([]byte, 4)
		n, err := ps.reader.Read(sizeBuffer)
		if err != nil {
			return nil, totalBytes, err
		}
		if n != 4 {
			return nil, totalBytes, io.ErrUnexpectedEOF
		}
		totalBytes += 4

		bodySize := binary.BigEndian.Uint32(sizeBuffer)

		// 读取数据体 - 直接读取
		bodyData := make([]byte, int(bodySize))
		totalRead := 0
		for totalRead < int(bodySize) {
			n, err := ps.reader.Read(bodyData[totalRead:])
			if err != nil {
				return nil, totalBytes, err
			}
			totalRead += n
		}
		totalBytes += len(bodyData)

		// 如果数据被压缩，需要解压
		if packetType.IsCompressed() {
			// 使用gzip_rewriter解压
			gzipReader := NewGzipReader(bytes.NewReader(bodyData), ps.Ctx())
			defer gzipReader.Close()

			var decompressedData bytes.Buffer
			_, err = io.Copy(&decompressedData, gzipReader)
			if err != nil {
				return nil, totalBytes, err
			}

			bodyData = decompressedData.Bytes()
		}

		// 解析 CommandPacket
		var commandPacket packet.CommandPacket
		err = json.Unmarshal(bodyData, &commandPacket)
		if err != nil {
			return nil, totalBytes, err
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
		return totalBytes, err
	}
	totalBytes += 1

	// 如果是心跳包，写入完成
	if packetType.IsHeartbeat() {
		return totalBytes, nil
	}

	// 如果是JsonCommand包，写入数据体
	if packetType.IsJsonCommand() && pkt.CommandPacket != nil {
		// 将 CommandPacket 序列化为 JSON 字节数据
		bodyData, err := json.Marshal(pkt.CommandPacket)
		if err != nil {
			return totalBytes, err
		}

		// 如果需要压缩，先压缩数据
		if useCompression {
			var compressedData bytes.Buffer
			gzipWriter := NewGzipWriter(&compressedData, ps.Ctx())

			_, err = gzipWriter.Write(bodyData)
			if err != nil {
				gzipWriter.Close()
				return totalBytes, err
			}

			// 确保所有数据都写入并关闭
			gzipWriter.Close()

			bodyData = compressedData.Bytes()
		}

		// 写入数据体大小（4字节）
		sizeBuffer := make([]byte, 4)
		binary.BigEndian.PutUint32(sizeBuffer, uint32(len(bodyData)))
		_, err = ps.writer.Write(sizeBuffer)
		if err != nil {
			return totalBytes, err
		}
		totalBytes += 4

		// 写入数据体，根据限速参数决定是否使用限速
		if rateLimitBytesPerSecond > 0 {
			// 使用限速写入器
			rateLimitedWriter := NewRateLimiterWriter(ps.writer, rateLimitBytesPerSecond, ps.Ctx())
			defer rateLimitedWriter.Close()

			// 分块写入，确保所有数据都被写入
			totalWritten := 0
			for totalWritten < len(bodyData) {
				n, err := rateLimitedWriter.Write(bodyData[totalWritten:])
				if err != nil {
					return totalBytes, err
				}
				totalWritten += n
			}
		} else {
			// 直接写入，不限速
			_, err = ps.writer.Write(bodyData)
			if err != nil {
				return totalBytes, err
			}
		}
		totalBytes += len(bodyData)
	}

	return totalBytes, nil
}
