package validator

import (
	"errors"
	"tunnox-core/internal/packet"
)

// PacketValidator 数据包验证器接口
type PacketValidator interface {
	// ValidateTransferPacket 验证传输数据包
	ValidateTransferPacket(transferPacket *packet.TransferPacket) error

	// ValidateCommandPacket 验证命令数据包
	ValidateCommandPacket(commandPacket *packet.CommandPacket) error

	// ValidatePacketType 验证数据包类型
	ValidatePacketType(packetType packet.Type) error

	// ValidateCommandType 验证命令类型
	ValidateCommandType(commandType packet.CommandType) error
}

// DefaultPacketValidator 默认数据包验证器
type DefaultPacketValidator struct{}

// NewDefaultPacketValidator 创建新的默认数据包验证器
func NewDefaultPacketValidator() *DefaultPacketValidator {
	return &DefaultPacketValidator{}
}

// ValidateTransferPacket 验证传输数据包
func (v *DefaultPacketValidator) ValidateTransferPacket(transferPacket *packet.TransferPacket) error {
	if transferPacket == nil {
		return errors.New("transfer packet is nil")
	}

	// 验证数据包类型
	if err := v.ValidatePacketType(transferPacket.PacketType); err != nil {
		return err
	}

	// 验证命令数据包
	if transferPacket.CommandPacket != nil {
		if err := v.ValidateCommandPacket(transferPacket.CommandPacket); err != nil {
			return err
		}
	}

	return nil
}

// ValidateCommandPacket 验证命令数据包
func (v *DefaultPacketValidator) ValidateCommandPacket(commandPacket *packet.CommandPacket) error {
	if commandPacket == nil {
		return errors.New("command packet is nil")
	}

	// 验证命令类型
	if err := v.ValidateCommandType(commandPacket.CommandType); err != nil {
		return err
	}

	// 验证命令ID
	if commandPacket.CommandId == "" {
		return errors.New("command ID is empty")
	}

	// 验证发送者ID
	if commandPacket.SenderId == "" {
		return errors.New("sender ID is empty")
	}

	// 验证接收者ID
	if commandPacket.ReceiverId == "" {
		return errors.New("receiver ID is empty")
	}

	return nil
}

// ValidatePacketType 验证数据包类型
func (v *DefaultPacketValidator) ValidatePacketType(packetType packet.Type) error {
	switch packetType {
	case packet.JsonCommand, packet.Compressed, packet.Encrypted, packet.Heartbeat:
		return nil
	default:
		return errors.New("invalid packet type")
	}
}

// ValidateCommandType 验证命令类型
func (v *DefaultPacketValidator) ValidateCommandType(commandType packet.CommandType) error {
	// 定义有效的命令类型
	validCommands := map[packet.CommandType]bool{
		packet.Connect: true, packet.Disconnect: true, packet.Reconnect: true, packet.HeartbeatCmd: true,
		packet.TcpMapCreate: true, packet.TcpMapDelete: true, packet.TcpMapUpdate: true, packet.TcpMapList: true, packet.TcpMapStatus: true,
		packet.HttpMapCreate: true, packet.HttpMapDelete: true, packet.HttpMapUpdate: true, packet.HttpMapList: true, packet.HttpMapStatus: true,
		packet.SocksMapCreate: true, packet.SocksMapDelete: true, packet.SocksMapUpdate: true, packet.SocksMapList: true, packet.SocksMapStatus: true,
		packet.DataTransferStart: true, packet.DataTransferStop: true, packet.DataTransferStatus: true, packet.ProxyForward: true, packet.DataTransferOut: true,
		packet.ConfigGet: true, packet.ConfigSet: true, packet.StatsGet: true, packet.LogGet: true, packet.HealthCheck: true,
		packet.RpcInvoke: true, packet.RpcRegister: true, packet.RpcUnregister: true, packet.RpcList: true,
	}

	if validCommands[commandType] {
		return nil
	}
	return errors.New("invalid command type")
}
