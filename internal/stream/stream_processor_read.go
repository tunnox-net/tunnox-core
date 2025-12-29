package stream

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream/compression"
	"tunnox-core/internal/utils"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 流处理器读取相关方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

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

// ReadAvailable 读取当前可用的数据（不等待完整长度）
// 用于流模式下的数据转发，避免阻塞等待固定长度的数据
func (ps *StreamProcessor) ReadAvailable(maxLength int) ([]byte, error) {
	if err := ps.acquireReadLock(); err != nil {
		return nil, err
	}
	defer ps.readLock.Unlock()

	if maxLength <= 0 {
		maxLength = 32 * 1024
	}

	// 使用内存池分配缓冲区
	buffer := ps.bufferMgr.Allocate(maxLength)

	// 直接读取可用数据，不循环等待
	n, err := ps.reader.Read(buffer)
	if n > 0 {
		// 复制数据后释放内存池 buffer
		result := make([]byte, n)
		copy(result, buffer[:n])
		ps.bufferMgr.Release(buffer)
		return result, err
	}
	ps.bufferMgr.Release(buffer)
	return nil, err
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

// readPacketBody 读取包体数据，使用内存池优化
// 通过 bufferMgr 复用缓冲区，减少 GC 压力
func (ps *StreamProcessor) readPacketBody(bodySize uint32) ([]byte, error) {
	// 安全检查：防止恶意大包导致 OOM
	if bodySize > constants.MaxPacketBodySize {
		return nil, errors.NewPacketError("read_packet_body",
			fmt.Sprintf("packet body size %d exceeds maximum allowed %d", bodySize, constants.MaxPacketBodySize), nil)
	}

	// 使用内存池获取缓冲区，减少内存分配
	buffer := ps.bufferMgr.Allocate(int(bodySize))

	totalRead := 0
	for totalRead < int(bodySize) {
		n, err := ps.reader.Read(buffer[totalRead:])
		if err != nil {
			ps.bufferMgr.Release(buffer)
			return nil, errors.NewStreamError("read_packet_body", "failed to read packet body", err)
		}
		totalRead += n
	}

	// 复制数据到新 slice，然后释放内存池 buffer
	// 这样调用方可以安全持有返回的数据，同时内存池 buffer 可以被复用
	result := make([]byte, totalRead)
	copy(result, buffer[:totalRead])
	ps.bufferMgr.Release(buffer)

	return result, nil
}

// decompressData 解压数据
// 预分配缓冲区以减少扩容次数，假设压缩比约为 3:1
func (ps *StreamProcessor) decompressData(compressedData []byte) ([]byte, error) {
	gzipReader := compression.NewGzipReader(bytes.NewReader(compressedData), ps.ResourceBase.Dispose.Ctx())
	defer gzipReader.Close()

	// 预分配缓冲区，假设压缩比约为 3:1，减少扩容次数
	estimatedSize := len(compressedData) * 3
	if estimatedSize > constants.MaxPacketBodySize {
		estimatedSize = constants.MaxPacketBodySize
	}
	decompressedData := bytes.NewBuffer(make([]byte, 0, estimatedSize))

	_, err := io.Copy(decompressedData, gzipReader)
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

	// 如果是JsonCommand包或CommandResp包，解析为 CommandPacket
	if packetType.IsJsonCommand() || packetType.IsCommandResp() {
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
