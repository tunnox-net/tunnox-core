package httppoll

import (
	"encoding/json"
	"fmt"
	"net/http"

	"tunnox-core/internal/packet"
)

// PacketConverter HTTP 包转换器
// 用于在 Tunnox 包（TransferPacket）和 HTTP Request/Response 之间转换
type PacketConverter struct {
	connectionID string
	clientID     int64
	mappingID    string
	tunnelType   string // "control" | "data"
}

// NewPacketConverter 创建包转换器
func NewPacketConverter() *PacketConverter {
	return &PacketConverter{}
}

// SetConnectionInfo 设置连接信息
func (c *PacketConverter) SetConnectionInfo(connID string, clientID int64, mappingID string, tunnelType string) {
	c.connectionID = connID
	c.clientID = clientID
	c.mappingID = mappingID
	c.tunnelType = tunnelType
}

// WritePacket 将 Tunnox 包转换为 HTTP Request
// 返回的 Request 包含所有必要的 Header 和 Body
// requestID 是可选的，如果提供则设置到 TunnelPackage 中
func (c *PacketConverter) WritePacket(pkt *packet.TransferPacket, requestID ...string) (*http.Request, error) {
	if pkt == nil {
		return nil, fmt.Errorf("packet is nil")
	}

	// 1. 提取包类型和数据
	packetType := pkt.PacketType
	var packetData interface{}

	switch {
	case packetType.IsJsonCommand() || packetType.IsCommandResp():
		// JsonCommand/CommandResp: 使用 CommandPacket
		if pkt.CommandPacket != nil {
			packetData = pkt.CommandPacket
		}
	case packetType.IsHandshake():
		// Handshake: 解析 Payload
		var handshakeReq packet.HandshakeRequest
		if err := json.Unmarshal(pkt.Payload, &handshakeReq); err == nil {
			packetData = &handshakeReq
		} else {
			packetData = pkt.Payload
		}
	case packetType == packet.HandshakeResp:
		// HandshakeResp: 解析 Payload
		var handshakeResp packet.HandshakeResponse
		if err := json.Unmarshal(pkt.Payload, &handshakeResp); err == nil {
			packetData = &handshakeResp
		} else {
			packetData = pkt.Payload
		}
	case packetType == packet.TunnelOpen:
		// TunnelOpen: 解析 Payload
		var tunnelOpenReq packet.TunnelOpenRequest
		if err := json.Unmarshal(pkt.Payload, &tunnelOpenReq); err == nil {
			packetData = &tunnelOpenReq
		} else {
			packetData = pkt.Payload
		}
	case packetType == packet.TunnelOpenAck:
		// TunnelOpenAck: 解析 Payload
		var tunnelOpenAck packet.TunnelOpenAckResponse
		if err := json.Unmarshal(pkt.Payload, &tunnelOpenAck); err == nil {
			packetData = &tunnelOpenAck
		} else {
			packetData = pkt.Payload
		}
	default:
		// 其他类型: 使用 Payload
		packetData = pkt.Payload
	}

	// 2. 构建 TunnelPackage
	tunnelPkg := &TunnelPackage{
		ConnectionID: c.connectionID,
		ClientID:     c.clientID,
		MappingID:    c.mappingID,
		TunnelType:   c.tunnelType,
		Type:         packetTypeToString(packetType),
		Data:         packetData,
	}

	// 如果提供了 requestID，设置到 TunnelPackage 中
	if len(requestID) > 0 && requestID[0] != "" {
		tunnelPkg.RequestID = requestID[0]
	}

	// 3. 编码 TunnelPackage
	encoded, err := EncodeTunnelPackage(tunnelPkg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode tunnel package: %w", err)
	}

	// 4. 构建 HTTP Request（控制包不需要 body，但需要设置 Content-Length: 0）
	req, err := http.NewRequest("POST", "/_tunnox/v1/push", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Tunnel-Package", encoded)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 0
	return req, nil
}

// ReadPacket 从 HTTP Response 读取 Tunnox 包
func (c *PacketConverter) ReadPacket(resp *http.Response) (*packet.TransferPacket, error) {
	// 1. 从 Header 读取 X-Tunnel-Package
	encoded := resp.Header.Get("X-Tunnel-Package")
	if encoded == "" {
		return nil, fmt.Errorf("missing X-Tunnel-Package header")
	}

	// 2. 解码 TunnelPackage
	tunnelPkg, err := DecodeTunnelPackage(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tunnel package: %w", err)
	}

	// 3. 更新连接状态
	if tunnelPkg.ConnectionID != "" {
		c.connectionID = tunnelPkg.ConnectionID
	}
	if tunnelPkg.ClientID > 0 {
		c.clientID = tunnelPkg.ClientID
	}
	if tunnelPkg.MappingID != "" {
		c.mappingID = tunnelPkg.MappingID
	}

	// 4. 转换为 TransferPacket
	return TunnelPackageToTransferPacket(tunnelPkg)
}

// WriteData 将字节流写入 HTTP Request Body（Base64 编码）
func (c *PacketConverter) WriteData(data []byte) ([]byte, error) {
	// Base64 编码在 HTTPStreamProcessor 中处理
	return data, nil
}

// ReadData 从 HTTP Response Body 读取字节流（Base64 解码）
func (c *PacketConverter) ReadData(resp *http.Response) ([]byte, error) {
	// Base64 解码在 HTTPStreamProcessor 中处理
	return nil, fmt.Errorf("not implemented")
}

// packetTypeToString 包类型字符串与字节的映射
func packetTypeToString(t packet.Type) string {
	baseType := t & 0x3F // 忽略压缩/加密标志
	switch baseType {
	case packet.Handshake:
		return "Handshake"
	case packet.HandshakeResp:
		return "HandshakeResponse"
	case packet.JsonCommand:
		return "JsonCommand"
	case packet.CommandResp:
		return "CommandResp"
	case packet.TunnelOpen:
		return "TunnelOpen"
	case packet.TunnelOpenAck:
		return "TunnelOpenAck"
	case packet.Heartbeat:
		return "Heartbeat"
	case packet.TunnelData:
		return "TunnelData"
	case packet.TunnelClose:
		return "TunnelClose"
	default:
		return fmt.Sprintf("Unknown_%d", baseType)
	}
}

// stringToPacketType 字符串转包类型
func stringToPacketType(s string) packet.Type {
	switch s {
	case "Handshake":
		return packet.Handshake
	case "HandshakeResponse":
		return packet.HandshakeResp
	case "JsonCommand":
		return packet.JsonCommand
	case "CommandResp", "CommandResponse":
		return packet.CommandResp
	case "TunnelOpen":
		return packet.TunnelOpen
	case "TunnelOpenAck":
		return packet.TunnelOpenAck
	case "Heartbeat":
		return packet.Heartbeat
	case "TunnelData":
		return packet.TunnelData
	case "TunnelClose":
		return packet.TunnelClose
	default:
		return 0
	}
}

// TunnelPackageToTransferPacket TunnelPackage -> TransferPacket
func TunnelPackageToTransferPacket(pkg *TunnelPackage) (*packet.TransferPacket, error) {
	packetType := stringToPacketType(pkg.Type)
	if packetType == 0 {
		return nil, fmt.Errorf("unknown packet type: %s", pkg.Type)
	}

	var pkt *packet.TransferPacket
	switch {
	case packetType.IsJsonCommand() || packetType.IsCommandResp():
		// 从 Data 中提取 CommandPacket
		dataBytes, err := json.Marshal(pkg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal command data: %w", err)
		}
		cmdPacket := &packet.CommandPacket{}
		if err := json.Unmarshal(dataBytes, cmdPacket); err != nil {
			return nil, fmt.Errorf("failed to unmarshal command packet: %w", err)
		}
		pkt = &packet.TransferPacket{
			PacketType:    packetType,
			CommandPacket: cmdPacket,
		}
	default:
		// 对于 Handshake, HandshakeResp, TunnelOpen 等，序列化为 Payload
		payload, err := json.Marshal(pkg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		pkt = &packet.TransferPacket{
			PacketType: packetType,
			Payload:    payload,
		}
	}

	return pkt, nil
}
