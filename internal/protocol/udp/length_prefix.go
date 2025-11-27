package udp

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// MaxUDPPacketSize UDP数据包的最大大小（64KB - 1）
	MaxUDPPacketSize = 65535
	// LengthPrefixSize 长度前缀的字节数
	LengthPrefixSize = 4
)

// ReadLengthPrefixedPacket 从reader读取长度前缀的数据包
// 数据格式：[4字节长度][数据内容]
// 返回读取的数据，如果读取失败则返回错误
func ReadLengthPrefixedPacket(reader io.Reader) ([]byte, error) {
	// 读取4字节长度前缀
	lenBuf := make([]byte, LengthPrefixSize)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return nil, err
	}

	// 解析数据长度
	dataLen := binary.BigEndian.Uint32(lenBuf)
	
	// 验证数据长度有效性
	if dataLen == 0 {
		return nil, fmt.Errorf("invalid data length: 0")
	}
	if dataLen > MaxUDPPacketSize {
		return nil, fmt.Errorf("data length %d exceeds maximum %d", dataLen, MaxUDPPacketSize)
	}

	// 读取实际数据
	data := make([]byte, dataLen)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	return data, nil
}

// WriteLengthPrefixedPacket 向writer写入长度前缀的数据包
// 数据格式：[4字节长度][数据内容]
// 返回写入的总字节数和错误（如果有）
func WriteLengthPrefixedPacket(writer io.Writer, data []byte) error {
	dataLen := len(data)
	
	// 验证数据长度有效性
	if dataLen == 0 {
		return fmt.Errorf("invalid data length: 0")
	}
	if dataLen > MaxUDPPacketSize {
		return fmt.Errorf("data length %d exceeds maximum %d", dataLen, MaxUDPPacketSize)
	}

	// 写入长度前缀
	lenBuf := make([]byte, LengthPrefixSize)
	binary.BigEndian.PutUint32(lenBuf, uint32(dataLen))
	
	if _, err := writer.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write length prefix: %w", err)
	}

	// 写入实际数据
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// ReadLengthPrefixedPacketWithMaxSize 读取长度前缀的数据包，并指定最大允许长度
// 用于防止恶意客户端发送过大的数据包
func ReadLengthPrefixedPacketWithMaxSize(reader io.Reader, maxSize uint32) ([]byte, error) {
	// 读取4字节长度前缀
	lenBuf := make([]byte, LengthPrefixSize)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return nil, err
	}

	// 解析数据长度
	dataLen := binary.BigEndian.Uint32(lenBuf)
	
	// 验证数据长度有效性
	if dataLen == 0 {
		return nil, fmt.Errorf("invalid data length: 0")
	}
	if dataLen > maxSize {
		return nil, fmt.Errorf("data length %d exceeds maximum allowed %d", dataLen, maxSize)
	}

	// 读取实际数据
	data := make([]byte, dataLen)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	return data, nil
}

