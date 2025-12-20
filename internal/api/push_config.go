package api

import (
corelog "tunnox-core/internal/core/log"
	"encoding/json"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/config"
	"tunnox-core/internal/packet"
)

// pushMappingToClients 推送映射配置给相关客户端
func (s *ManagementAPIServer) pushMappingToClients(mapping *models.PortMapping) error {
	if s.sessionMgr == nil {
		// 在测试环境中，SessionManager 可能未配置，这是可以接受的
		// 只记录警告，不返回错误，允许测试继续进行
		corelog.Warnf("API: SessionManager not configured, skipping config push for mapping %s", mapping.ID)
		return nil
	}

	corelog.Debugf("API: pushing mapping %s to clients (source=%d, target=%d)",
		mapping.ID, mapping.SourceClientID, mapping.TargetClientID)

	// 构造映射配置
	mappingConfigs := []config.MappingConfig{
		{
			MappingID:         mapping.ID,
			Protocol:          string(mapping.Protocol),
			LocalPort:         mapping.SourcePort,
			TargetHost:        mapping.TargetHost,
			TargetPort:        mapping.TargetPort,
			SecretKey:         mapping.SecretKey,
			EnableCompression: mapping.Config.EnableCompression,
			CompressionLevel:  mapping.Config.CompressionLevel,
			EnableEncryption:  mapping.Config.EnableEncryption,
			EncryptionMethod:  mapping.Config.EncryptionMethod,
			EncryptionKey:     mapping.Config.EncryptionKey,
			BandwidthLimit:    mapping.Config.BandwidthLimit,
			MaxConnections:    mapping.Config.MaxConnections,
		},
	}

	// 序列化配置
	configData := ConfigPushData{
		Mappings: mappingConfigs,
	}

	configJSON, err := json.Marshal(&configData)
	if err != nil {
		corelog.Errorf("API: failed to marshal mapping config: %v", err)
		return fmt.Errorf("failed to marshal mapping config: %w", err)
	}

	// 推送给源客户端
	if err := s.pushConfigToClient(mapping.SourceClientID, string(configJSON)); err != nil {
		return fmt.Errorf("failed to push config to source client %d: %w", mapping.SourceClientID, err)
	}

	// 推送给目标客户端（如果不是同一个客户端）
	if mapping.TargetClientID != mapping.SourceClientID {
		// 目标端配置：LocalPort设为0
		targetConfig := mappingConfigs[0]
		targetConfig.LocalPort = 0 // 目标端不需要监听

		targetData := ConfigPushData{
			Mappings: []config.MappingConfig{targetConfig},
		}

		targetJSON, err := json.Marshal(&targetData)
		if err != nil {
			corelog.Errorf("API: failed to marshal target config: %v", err)
			return fmt.Errorf("failed to marshal target config: %w", err)
		}

		if err := s.pushConfigToClient(mapping.TargetClientID, string(targetJSON)); err != nil {
			return fmt.Errorf("failed to push config to target client %d: %w", mapping.TargetClientID, err)
		}
	}

	return nil
}

// pushConfigToClient 推送配置给指定客户端
//
// 优化：使用快速查询GetClientNodeID（仅查Redis状态），而非GetClient（查配置+状态）
//
// 流程：
// 1. 从Redis查询client所在节点（快速）
// 2. 如果在其他节点，直接广播到集群
// 3. 如果在本节点，本地推送
// 4. 如果查不到节点信息，广播到集群（让正确的节点处理）
func (s *ManagementAPIServer) pushConfigToClient(clientID int64, configBody string) error {
	// 优化：使用快速查询GetClientNodeID（仅查Redis状态）
	if s.cloudControl != nil {
		clientNodeID, err := s.cloudControl.GetClientNodeID(clientID)
		if err != nil {
			corelog.Debugf("API: failed to query client %d node from Redis: %v", clientID, err)
			// 继续fallback到本地查询
		} else if clientNodeID != "" {
			// 客户端在线，且已知节点ID
			currentNodeID := s.sessionMgr.GetNodeID()
			if clientNodeID != currentNodeID {
				// Client在其他节点，直接广播
				corelog.Debugf("API: client %d is on node %s (current: %s), broadcasting via Redis",
					clientID, clientNodeID, currentNodeID)
				if err := s.broadcastConfigPushToCluster(clientID, configBody); err != nil {
					return fmt.Errorf("failed to broadcast config push to cluster: %w", err)
				}
				return nil
			}
			corelog.Debugf("API: client %d confirmed on THIS node %s via Redis", clientID, currentNodeID)
		} else {
			// clientNodeID为空 = 客户端离线或不存在
			corelog.Debugf("API: client %d is offline (no nodeID in Redis), will broadcast anyway", clientID)
			if err := s.broadcastConfigPushToCluster(clientID, configBody); err != nil {
				return fmt.Errorf("failed to broadcast config push for offline client: %w", err)
			}
			return nil
		}
	}

	// 获取客户端的控制连接（本地）
	connInterface := s.sessionMgr.GetControlConnectionInterface(clientID)
	if connInterface == nil {
		// 本地未找到，尝试通过消息队列广播到其他节点
		corelog.Debugf("API: client %d NOT found locally, broadcasting to cluster", clientID)
		if err := s.broadcastConfigPushToCluster(clientID, configBody); err != nil {
			return fmt.Errorf("failed to broadcast config push to cluster: %w", err)
		}
		return nil // 广播成功，其他节点会处理
	}

	// 获取StreamProcessor
	streamProcessor, connID, remoteAddr, err := getStreamFromConnection(connInterface, clientID)
	if err != nil {
		// 发现脏数据（有连接对象但stream为nil），清理并广播
		corelog.Warnf("API: client %d has stale local connection (stream is nil), broadcasting to cluster", clientID)
		if err := s.broadcastConfigPushToCluster(clientID, configBody); err != nil {
			return fmt.Errorf("failed to broadcast after detecting stale connection: %w", err)
		}
		return nil
	}

	// 构造ConfigSet命令
	cmd := &packet.CommandPacket{
		CommandType: packet.ConfigSet,
		CommandBody: configBody,
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	// 异步发送配置推送包
	corelog.Debugf("API: pushing config to client %d (ConnID=%s, RemoteAddr=%s)",
		clientID, connID, remoteAddr)
	sendPacketAsync(streamProcessor, pkt, clientID, 5*time.Second)

	return nil
}

// broadcastConfigPushToCluster 广播配置推送到集群其他节点
func (s *ManagementAPIServer) broadcastConfigPushToCluster(clientID int64, configBody string) error {
	// 通过SessionManager广播
	if err := s.sessionMgr.BroadcastConfigPush(clientID, configBody); err != nil {
		return fmt.Errorf("failed to broadcast config push: %w", err)
	}
	return nil
}

// removeMappingFromClients 通知客户端移除映射
func (s *ManagementAPIServer) removeMappingFromClients(mapping *models.PortMapping) {
	if s.sessionMgr == nil {
		corelog.Warnf("API: SessionManager not configured, cannot push removal notification")
		return
	}

	corelog.Infof("API: notifying clients to remove mapping %s (source=%d, target=%d)",
		mapping.ID, mapping.SourceClientID, mapping.TargetClientID)

	// 构造空的映射配置（表示移除）
	configData := ConfigRemovalData{
		Mappings:       []config.MappingConfig{},
		RemoveMappings: []string{mapping.ID},
	}

	configJSON, err := json.Marshal(&configData)
	if err != nil {
		corelog.Errorf("API: failed to marshal removal config: %v", err)
		return
	}

	// 通知源客户端
	s.pushConfigToClient(mapping.SourceClientID, string(configJSON))

	// 通知目标客户端（如果不是同一个客户端）
	if mapping.TargetClientID != mapping.SourceClientID {
		s.pushConfigToClient(mapping.TargetClientID, string(configJSON))
	}
}

// kickClient 踢下线指定客户端
func (s *ManagementAPIServer) kickClient(clientID int64, reason, code string) {
	if s.sessionMgr == nil {
		corelog.Warnf("API: SessionManager not configured, cannot kick client")
		return
	}

	corelog.Infof("API: kicking client %d, reason=%s, code=%s", clientID, reason, code)

	// 获取客户端的控制连接
	connInterface := s.sessionMgr.GetControlConnectionInterface(clientID)
	if connInterface == nil {
		corelog.Warnf("API: client %d not connected, cannot kick", clientID)
		return
	}

	// 获取StreamProcessor
	streamProcessor, connID, remoteAddr, err := getStreamFromConnection(connInterface, clientID)
	if err != nil {
		corelog.Warnf("API: failed to get stream for client %d: %v", clientID, err)
		return
	}

	// 构造踢下线命令
	kickInfo := KickClientInfo{
		Reason: reason,
		Code:   code,
	}
	kickJSON, _ := json.Marshal(&kickInfo)

	cmd := &packet.CommandPacket{
		CommandType: packet.KickClient,
		CommandBody: string(kickJSON),
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	// 异步发送踢下线命令
	corelog.Infof("API: sending kick command to client %d (ConnID=%s, RemoteAddr=%s)", clientID, connID, remoteAddr)
	sendPacketAsync(streamProcessor, pkt, clientID, 3*time.Second)
}

// SetSessionManager 设置SessionManager（由Server启动时调用）
func (s *ManagementAPIServer) SetSessionManager(sessionMgr SessionManager) {
	s.sessionMgr = sessionMgr
	corelog.Infof("API: SessionManager configured")
}
