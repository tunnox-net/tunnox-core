package builder

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"tunnox-core/internal/packet"
)

// PacketBuilder 数据包构建器接口
type PacketBuilder interface {
	// BuildPacket 构建数据包
	BuildPacket(writer io.Writer, transferPacket *packet.TransferPacket) error

	// BuildCommandPacket 构建命令数据包
	BuildCommandPacket(commandType packet.CommandType, commandID, token, senderID, receiverID, commandBody string) (*packet.CommandPacket, error)

	// BuildTransferPacket 构建传输数据包
	BuildTransferPacket(packetType packet.Type, commandPacket *packet.CommandPacket) *packet.TransferPacket
}

// DefaultPacketBuilder 默认数据包构建器
type DefaultPacketBuilder struct{}

// NewDefaultPacketBuilder 创建新的默认数据包构建器
func NewDefaultPacketBuilder() *DefaultPacketBuilder {
	return &DefaultPacketBuilder{}
}

// BuildPacket 构建数据包
func (b *DefaultPacketBuilder) BuildPacket(writer io.Writer, transferPacket *packet.TransferPacket) error {
	// 写入数据包类型
	typeBytes := []byte{byte(transferPacket.PacketType)}
	if _, err := writer.Write(typeBytes); err != nil {
		return err
	}

	// 序列化命令数据包
	var data []byte
	var err error

	if transferPacket.CommandPacket != nil {
		data, err = json.Marshal(transferPacket.CommandPacket)
		if err != nil {
			return err
		}
	}

	// 写入数据包长度
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
	if _, err := writer.Write(lengthBytes); err != nil {
		return err
	}

	// 写入数据包内容
	if len(data) > 0 {
		if _, err := writer.Write(data); err != nil {
			return err
		}
	}

	return nil
}

// BuildCommandPacket 构建命令数据包
func (b *DefaultPacketBuilder) BuildCommandPacket(commandType packet.CommandType, commandID, token, senderID, receiverID, commandBody string) (*packet.CommandPacket, error) {
	return &packet.CommandPacket{
		CommandType: commandType,
		CommandId:   commandID,
		Token:       token,
		SenderId:    senderID,
		ReceiverId:  receiverID,
		CommandBody: commandBody,
	}, nil
}

// BuildTransferPacket 构建传输数据包
func (b *DefaultPacketBuilder) BuildTransferPacket(packetType packet.Type, commandPacket *packet.CommandPacket) *packet.TransferPacket {
	return &packet.TransferPacket{
		PacketType:    packetType,
		CommandPacket: commandPacket,
	}
}
