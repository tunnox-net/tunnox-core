package stream

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream/compression"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 流处理器写入相关方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

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

	// 如果是JsonCommand包或CommandResp包，序列化 CommandPacket
	if (packetType.IsJsonCommand() || packetType.IsCommandResp()) && pkt.CommandPacket != nil {
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

	// Flush writer if it implements Flusher interface (for buffered writers like GzipWriter)
	if flusher, ok := ps.writer.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return totalBytes, errors.NewStreamError("flush_writer", "failed to flush writer", err)
		}
	}

	return totalBytes, nil
}
