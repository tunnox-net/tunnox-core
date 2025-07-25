package parser

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"tunnox-core/internal/packet"
)

// PacketParser 数据包解析器接口
type PacketParser interface {
	// ParsePacket 解析数据包
	ParsePacket(reader io.Reader) (*packet.TransferPacket, error)

	// ParseCommandPacket 解析命令数据包
	ParseCommandPacket(data []byte) (*packet.CommandPacket, error)
}

// DefaultPacketParser 默认数据包解析器
type DefaultPacketParser struct{}

// NewDefaultPacketParser 创建新的默认数据包解析器
func NewDefaultPacketParser() *DefaultPacketParser {
	return &DefaultPacketParser{}
}

// ParsePacket 解析数据包
func (p *DefaultPacketParser) ParsePacket(reader io.Reader) (*packet.TransferPacket, error) {
	// 读取数据包类型
	typeBytes := make([]byte, 1)
	if _, err := io.ReadFull(reader, typeBytes); err != nil {
		return nil, err
	}

	packetType := packet.Type(typeBytes[0])

	// 读取数据包长度
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, lengthBytes); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lengthBytes)

	// 读取数据包内容
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	// 根据类型解析具体内容
	switch packetType {
	case packet.JsonCommand:
		commandPacket, err := p.ParseCommandPacket(data)
		if err != nil {
			return nil, err
		}
		return &packet.TransferPacket{
			PacketType:    packetType,
			CommandPacket: commandPacket,
		}, nil

	default:
		return nil, errors.New("unknown packet type")
	}
}

// ParseCommandPacket 解析命令数据包
func (p *DefaultPacketParser) ParseCommandPacket(data []byte) (*packet.CommandPacket, error) {
	var commandPacket packet.CommandPacket
	if err := json.Unmarshal(data, &commandPacket); err != nil {
		return nil, err
	}
	return &commandPacket, nil
}
