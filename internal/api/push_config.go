package api

import (
	"encoding/json"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/config"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// pushMappingToClients 推送映射配置给相关客户端
func (s *ManagementAPIServer) pushMappingToClients(mapping *models.PortMapping) {
	if s.sessionMgr == nil {
		utils.Warnf("API: SessionManager not configured, cannot push config")
		return
	}

	utils.Infof("API: pushing mapping %s to clients (source=%d, target=%d)",
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
	configData := map[string]interface{}{
		"mappings": mappingConfigs,
	}

	configJSON, err := json.Marshal(configData)
	if err != nil {
		utils.Errorf("API: failed to marshal mapping config: %v", err)
		return
	}

	// 推送给源客户端
	s.pushConfigToClient(mapping.SourceClientID, string(configJSON))

	// 推送给目标客户端（如果不是同一个客户端）
	if mapping.TargetClientID != mapping.SourceClientID {
		// 目标端配置：LocalPort设为0
		targetConfig := mappingConfigs[0]
		targetConfig.LocalPort = 0 // 目标端不需要监听

		targetData := map[string]interface{}{
			"mappings": []config.MappingConfig{targetConfig},
		}

		targetJSON, err := json.Marshal(targetData)
		if err != nil {
			utils.Errorf("API: failed to marshal target config: %v", err)
			return
		}

		s.pushConfigToClient(mapping.TargetClientID, string(targetJSON))
	}
}

// pushConfigToClient 推送配置给指定客户端
func (s *ManagementAPIServer) pushConfigToClient(clientID int64, configBody string) {
	utils.Infof("API: pushing config to client %d", clientID)

	// 获取客户端的控制连接
	connInterface := s.sessionMgr.GetControlConnectionInterface(clientID)
	if connInterface == nil {
		utils.Warnf("API: client %d not connected, config will be sent when client connects", clientID)
		return
	}
	utils.Infof("API: ✅ found control connection for client %d, connInterface=%p", clientID, connInterface)

	// 获取ConnID和RemoteAddr用于追踪
	var connID string
	var remoteAddr string

	// 尝试获取ConnID
	type hasConnID interface {
		GetConnID() string
	}
	if v, ok := connInterface.(hasConnID); ok {
		connID = v.GetConnID()
		utils.Infof("API: control connection ConnID=%s", connID)
	}

	// 尝试获取RemoteAddr
	type hasRemoteAddr interface {
		GetRemoteAddr() string
	}
	if v, ok := connInterface.(hasRemoteAddr); ok {
		remoteAddr = v.GetRemoteAddr()
		utils.Infof("API: control connection RemoteAddr=%s", remoteAddr)
	}

	// 定义接口来访问GetStream方法
	type hasGetStream interface {
		GetStream() interface{}
	}

	// 通过GetStream()方法获取Stream
	var streamProcessor *stream.StreamProcessor
	if hs, ok := connInterface.(hasGetStream); ok {
		streamInterface := hs.GetStream()
		if streamInterface == nil {
			utils.Warnf("API: client %d stream is nil, client may not be fully connected", clientID)
			return
		}
		utils.Infof("API: got stream interface, stream=%p, type=%T", streamInterface, streamInterface)

		// 类型断言为 *stream.StreamProcessor
		var ok bool
		streamProcessor, ok = streamInterface.(*stream.StreamProcessor)
		if !ok {
			utils.Warnf("API: client %d stream type assertion failed, stream type=%T", clientID, streamInterface)
			return
		}
		utils.Infof("API: ✅ stream type assertion success, streamProcessor=%p", streamProcessor)
	}

	if streamProcessor == nil {
		utils.Warnf("API: cannot access stream for client %d, will retry when client reconnects", clientID)
		return
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

	// 发送配置（异步推送，避免阻塞API请求）
	go func() {
		utils.Infof("API: starting async config push to client %d (ConnID=%s, RemoteAddr=%s)",
			clientID, connID, remoteAddr)
		utils.Infof("API: streamProcessor=%p, about to call WritePacket", streamProcessor)

		// 使用channel+超时来避免永久阻塞
		done := make(chan error, 1)
		go func() {
			utils.Infof("API: calling WritePacket on streamProcessor=%p", streamProcessor)
			n, err := streamProcessor.WritePacket(pkt, false, 0)
			utils.Infof("API: WritePacket returned: bytes=%d, err=%v", n, err)
			done <- err
		}()

		select {
		case err := <-done:
			if err != nil {
				utils.Errorf("API: failed to push config to client %d: %v", clientID, err)
			} else {
				utils.Infof("API: ✅ config pushed successfully to client %d", clientID)
			}
		case <-time.After(5 * time.Second):
			utils.Errorf("API: push config to client %d timed out after 5s", clientID)
		}
	}()

	utils.Infof("API: config push initiated for client %d (async)", clientID)
}

// SetSessionManager 设置SessionManager（由Server启动时调用）
func (s *ManagementAPIServer) SetSessionManager(sessionMgr SessionManager) {
	s.sessionMgr = sessionMgr
	utils.Infof("API: SessionManager configured")
}
