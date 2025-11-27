package client

import (
	"encoding/json"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// handleCommand 处理命令
func (c *TunnoxClient) handleCommand(pkt *packet.TransferPacket) {
	if pkt.CommandPacket == nil {
		utils.Warnf("Client: received command packet with nil CommandPacket")
		return
	}

	cmdType := pkt.CommandPacket.CommandType
	utils.Infof("Client: received command, type=%v", cmdType)

	switch cmdType {
	case packet.ConfigSet:
		// 接收服务器推送的配置
		c.handleConfigUpdate(pkt.CommandPacket.CommandBody)

	case packet.TunnelOpenRequestCmd:
		// 隧道打开请求（作为目标客户端）
		c.handleTunnelOpenRequest(pkt.CommandPacket.CommandBody)

	case packet.KickClient:
		// 踢下线命令
		c.handleKickCommand(pkt.CommandPacket.CommandBody)
	}
}

// handleKickCommand 处理踢下线命令
func (c *TunnoxClient) handleKickCommand(cmdBody string) {
	var kickInfo struct {
		Reason string `json:"reason"`
		Code   string `json:"code"`
	}

	if err := json.Unmarshal([]byte(cmdBody), &kickInfo); err != nil {
		utils.Errorf("Client: failed to parse kick command: %v", err)
		kickInfo.Reason = "Unknown reason"
		kickInfo.Code = "UNKNOWN"
	}

	utils.Errorf("Client: KICKED BY SERVER - Reason: %s, Code: %s", kickInfo.Reason, kickInfo.Code)

	// 标记为被踢下线，禁止重连
	c.kicked = true

	// 停止客户端
	c.Stop()
}

