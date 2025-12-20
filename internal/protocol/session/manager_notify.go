package session

import (
corelog "tunnox-core/internal/core/log"
	"encoding/json"
	"tunnox-core/internal/config"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// NotifyClientUpdate 通知客户端更新配置
// 实现 managers.ClientNotifier 接口
func (s *SessionManager) NotifyClientUpdate(clientID int64) {
	corelog.Infof("SessionManager: Notifying client %d of update...", clientID)

	s.controlConnLock.RLock()
	conn, ok := s.clientIDIndexMap[clientID]
	s.controlConnLock.RUnlock()

	if !ok || conn == nil {
		corelog.Warnf("SessionManager: Client %d not found or not connected, cannot notify update", clientID)
		return
	}

	// 1. 获取客户端的所有映射
	mappings, err := s.cloudControl.GetClientPortMappings(clientID)
	if err != nil {
		corelog.Errorf("SessionManager: Failed to get mappings for client %d: %v", clientID, err)
		return
	}

	// 2. 转换为配置格式
	var mappingConfigs []config.MappingConfig
	for _, m := range mappings {
		// 只发送当前客户端作为 ListenClient 的映射（需要启动监听的）
		// 或者 SourceClientID (兼容旧版)
		if m.ListenClientID == clientID || (m.ListenClientID == 0 && m.SourceClientID == clientID) {
			cfg := config.MappingConfig{
				MappingID:  m.ID,
				SecretKey:  m.SecretKey,
				Protocol:   string(m.Protocol), // models.Protocol is string alias? Assuming so
				LocalPort:  m.SourcePort,
				TargetHost: m.TargetHost, // 可能为空，对于P2P或转发可能是 server
				TargetPort: m.TargetPort,

				// 商业化配额
				BandwidthLimit:    int64(m.Config.BandwidthLimit),
				MaxConnections:    m.Config.MaxConnections,
				EnableCompression: m.Config.EnableCompression,
			}
			mappingConfigs = append(mappingConfigs, cfg)
		}
	}

	// 3. 构造 payload
	payloadObj := struct {
		Mappings []config.MappingConfig `json:"mappings"`
	}{
		Mappings: mappingConfigs,
	}

	payloadBytes, err := json.Marshal(payloadObj)
	if err != nil {
		corelog.Errorf("SessionManager: Failed to marshal config update payload: %v", err)
		return
	}

	cmdID, _ := utils.GenerateUUID()
	cmdPacket := &packet.CommandPacket{
		CommandType: packet.ConfigSet, // 51
		CommandId:   cmdID,
		CommandBody: string(payloadBytes),
	}

	transferPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPacket,
	}

	// 5. 写入流
	streamer := conn.GetStream()
	if streamer != nil {
		if _, err := streamer.WritePacket(transferPacket, false, 0); err != nil {
			corelog.Errorf("SessionManager: Failed to send config update to client %d: %v", clientID, err)
		} else {
			corelog.Infof("SessionManager: Sent ConfigSet to client %d with %d mappings", clientID, len(mappingConfigs))
		}
	} else {
		corelog.Warnf("SessionManager: Client %d stream is nil", clientID)
	}
}
