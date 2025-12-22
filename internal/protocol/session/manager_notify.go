package session

import (
	"encoding/json"
	"tunnox-core/internal/config"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// NotifyClientUpdate 通知客户端更新配置
// 实现 managers.ClientNotifier 接口
// 在集群模式下，如果客户端不在本节点，会通过 Redis 广播通知其他节点
func (s *SessionManager) NotifyClientUpdate(clientID int64) {
	corelog.Infof("SessionManager: ⚡ NotifyClientUpdate called for client %d", clientID)

	s.controlConnLock.RLock()
	conn, ok := s.clientIDIndexMap[clientID]
	s.controlConnLock.RUnlock()

	// 1. 获取客户端的所有映射（无论客户端在哪个节点，都需要获取映射）
	mappings, err := s.cloudControl.GetClientPortMappings(clientID)
	if err != nil {
		corelog.Errorf("SessionManager: Failed to get mappings for client %d: %v", clientID, err)
		return
	}

	// 2. 转换为配置格式
	var mappingConfigs []config.MappingConfig
	for _, m := range mappings {
		// 只发送当前客户端作为 ListenClient 的映射（需要启动监听的）
		if m.ListenClientID == clientID {
			cfg := config.MappingConfig{
				MappingID:      m.ID,
				SecretKey:      m.SecretKey,
				Protocol:       string(m.Protocol),
				LocalPort:      m.SourcePort,
				TargetHost:     m.TargetHost,
				TargetPort:     m.TargetPort,
				TargetClientID: m.TargetClientID,

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

	// 4. 如果客户端在本节点，直接发送
	if ok && conn != nil {
		s.sendConfigToLocalClient(clientID, conn, payloadBytes)
		return
	}

	// 5. 客户端不在本节点，尝试通过广播通知其他节点
	corelog.Infof("SessionManager: Client %d not on this node, broadcasting config update to cluster", clientID)
	if err := s.BroadcastConfigPush(clientID, string(payloadBytes)); err != nil {
		corelog.Errorf("SessionManager: Failed to broadcast config update for client %d: %v", clientID, err)
	}
}

// sendConfigToLocalClient 向本地客户端发送配置更新
func (s *SessionManager) sendConfigToLocalClient(clientID int64, conn *ControlConnection, payloadBytes []byte) {
	cmdID, _ := utils.GenerateUUID()
	cmdPacket := &packet.CommandPacket{
		CommandType: packet.ConfigSet,
		CommandId:   cmdID,
		CommandBody: string(payloadBytes),
	}

	transferPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPacket,
	}

	streamer := conn.GetStream()
	if streamer != nil {
		if _, err := streamer.WritePacket(transferPacket, false, 0); err != nil {
			corelog.Errorf("SessionManager: Failed to send config update to client %d: %v", clientID, err)
		} else {
			corelog.Infof("SessionManager: Sent ConfigSet to client %d locally", clientID)
		}
	} else {
		corelog.Warnf("SessionManager: Client %d stream is nil", clientID)
	}
}
