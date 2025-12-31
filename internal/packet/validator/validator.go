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
	// 提取基础类型（去除压缩/加密标志 0x40 和 0x80）
	baseType := packetType & 0x3F

	// 验证基础类型
	switch baseType {
	case packet.Handshake, packet.HandshakeResp, packet.Heartbeat,
		packet.JsonCommand, packet.CommandResp,
		packet.TunnelOpen, packet.TunnelOpenAck, packet.TunnelData, packet.TunnelClose, packet.DataStreamEOF:
		return nil
	default:
		// 也检查是否是纯标志位（如单独的 Compressed 或 Encrypted）
		// 这种情况通常不会单独出现，但为了向后兼容保留
		if packetType == packet.Compressed || packetType == packet.Encrypted {
			return nil
		}
		return errors.New("invalid packet type")
	}
}

// ValidateCommandType 验证命令类型
func (v *DefaultPacketValidator) ValidateCommandType(commandType packet.CommandType) error {
	// 定义有效的命令类型
	validCommands := map[packet.CommandType]bool{
		// 连接管理类命令 (10-19)
		packet.Connect: true, packet.Disconnect: true, packet.Reconnect: true, packet.HeartbeatCmd: true,
		packet.KickClient: true, packet.ServerShutdown: true,

		// 端口映射类命令 (20-39)
		packet.TcpMapCreate: true, packet.TcpMapDelete: true, packet.TcpMapUpdate: true, packet.TcpMapList: true, packet.TcpMapStatus: true,
		packet.HttpMapCreate: true, packet.HttpMapDelete: true, packet.HttpMapUpdate: true, packet.HttpMapList: true, packet.HttpMapStatus: true,
		packet.SocksMapCreate: true, packet.SocksMapDelete: true, packet.SocksMapUpdate: true, packet.SocksMapList: true, packet.SocksMapStatus: true,

		// 隧道管理类命令 (35-39)
		packet.TunnelOpenRequestCmd: true, packet.TunnelMigrate: true, packet.TunnelMigrateAck: true, packet.TunnelStateSync: true,

		// 数据传输类命令 (40-49)
		packet.DataTransferStart: true, packet.DataTransferStop: true, packet.DataTransferStatus: true, packet.ProxyForward: true, packet.DataTransferOut: true,

		// 系统管理类命令 (50-59)
		packet.ConfigGet: true, packet.ConfigSet: true, packet.StatsGet: true, packet.LogGet: true, packet.HealthCheck: true,

		// RPC类命令 (60-69)
		packet.RpcInvoke: true, packet.RpcRegister: true, packet.RpcUnregister: true, packet.RpcList: true,

		// 连接码管理类命令 (70-79)
		packet.ConnectionCodeGenerate: true, packet.ConnectionCodeList: true, packet.ConnectionCodeActivate: true, packet.ConnectionCodeRevoke: true,
		packet.MappingList: true, packet.MappingGet: true, packet.MappingDelete: true,

		// HTTP 代理类命令 (80-89)
		packet.HTTPProxyRequest: true, packet.HTTPProxyResponse: true,
		packet.HTTPDomainGetBaseDomains: true, packet.HTTPDomainCheckSubdomain: true, packet.HTTPDomainGenSubdomain: true,
		packet.HTTPDomainCreate: true, packet.HTTPDomainDelete: true, packet.HTTPDomainList: true,

		// SOCKS5 代理类命令 (90-99)
		packet.SOCKS5TunnelRequestCmd: true,

		// 通知类命令 (100-109)
		packet.NotifyClient: true, packet.NotifyClientAck: true, packet.SendNotifyToClient: true,
	}

	if validCommands[commandType] {
		return nil
	}
	return errors.New("invalid command type")
}
