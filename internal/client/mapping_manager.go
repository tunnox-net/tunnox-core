package client

import (
	"encoding/json"

	"tunnox-core/internal/client/mapping"
	"tunnox-core/internal/cloud/models"
	clientconfig "tunnox-core/internal/config"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// handleConfigUpdate 处理配置更新
func (c *TunnoxClient) handleConfigUpdate(configBody string) {
	corelog.Infof("Client: received ConfigSet from server, body length=%d", len(configBody))

	var configUpdate struct {
		Mappings []clientconfig.MappingConfig `json:"mappings"`
	}

	if err := json.Unmarshal([]byte(configBody), &configUpdate); err != nil {
		corelog.Errorf("Client: failed to parse config update: %v", err)
		return
	}

	corelog.Infof("Client: parsed %d mappings from ConfigSet", len(configUpdate.Mappings))

	// 构建新配置的映射ID集合
	newMappingIDs := make(map[string]bool)
	newSOCKS5MappingIDs := make(map[string]bool)

	for i, mappingConfig := range configUpdate.Mappings {
		corelog.Infof("Client: processing mapping[%d]: ID=%s, Protocol=%s, LocalPort=%d",
			i, mappingConfig.MappingID, mappingConfig.Protocol, mappingConfig.LocalPort)
		newMappingIDs[mappingConfig.MappingID] = true

		// SOCKS5 映射由 SOCKS5Manager 处理（兼容服务端发送的 "socks" 和 "socks5"）
		if isSOCKS5Protocol(mappingConfig.Protocol) && mappingConfig.LocalPort > 0 {
			newSOCKS5MappingIDs[mappingConfig.MappingID] = true
			if err := c.addOrUpdateSOCKS5Mapping(mappingConfig); err != nil {
				corelog.Warnf("Client: failed to add SOCKS5 mapping %s: %v", mappingConfig.MappingID, err)
			}
		} else {
			if err := c.addOrUpdateMapping(mappingConfig); err != nil {
				corelog.Warnf("Client: failed to add mapping %s: %v", mappingConfig.MappingID, err)
			}
		}
	}

	// 删除不再存在的普通映射
	c.mu.Lock()
	for mappingID, handler := range c.mappingHandlers {
		if !newMappingIDs[mappingID] {
			corelog.Infof("Client: removing mapping %s (no longer in config)", mappingID)
			handler.Stop()
			delete(c.mappingHandlers, mappingID)
		}
	}
	c.mu.Unlock()

	// 删除不再存在的 SOCKS5 映射
	if c.socks5Manager != nil {
		for _, mappingID := range c.socks5Manager.ListMappings() {
			if !newSOCKS5MappingIDs[mappingID] {
				corelog.Infof("Client: removing SOCKS5 mapping %s (no longer in config)", mappingID)
				c.socks5Manager.RemoveMapping(mappingID)
			}
		}
	}

	corelog.Infof("Client: config updated successfully, total active mappings=%d", len(newMappingIDs))
}

// addOrUpdateSOCKS5Mapping 添加或更新 SOCKS5 映射
// 返回 error 以便调用方可以处理端口绑定失败等情况
func (c *TunnoxClient) addOrUpdateSOCKS5Mapping(mappingCfg clientconfig.MappingConfig) error {
	if c.socks5Manager == nil {
		corelog.Errorf("Client: SOCKS5Manager not initialized")
		return coreerrors.New(coreerrors.CodeInvalidState, "SOCKS5Manager not initialized")
	}

	// 转换为 models.PortMapping 格式
	portMapping := &models.PortMapping{
		ID:             mappingCfg.MappingID,
		Protocol:       models.ProtocolSOCKS,
		SourcePort:     mappingCfg.LocalPort,
		TargetClientID: mappingCfg.TargetClientID,
		ListenClientID: c.GetClientID(),
		SecretKey:      mappingCfg.SecretKey,
	}

	if err := c.socks5Manager.AddMapping(portMapping); err != nil {
		corelog.Errorf("Client: failed to add SOCKS5 mapping %s: %v", mappingCfg.MappingID, err)
		return err
	}

	return nil
}

// addOrUpdateMapping 添加或更新映射
// 返回 error 以便调用方可以处理端口绑定失败等情况
func (c *TunnoxClient) addOrUpdateMapping(mappingCfg clientconfig.MappingConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在
	if handler, exists := c.mappingHandlers[mappingCfg.MappingID]; exists {
		// 比较配置是否相同，如果相同则跳过（避免重复的 ConfigSet 导致 listener 重启）
		existingConfig := handler.GetConfig()
		if isMappingConfigEqual(existingConfig, mappingCfg) {
			corelog.Debugf("Client: mapping %s config unchanged, skipping update", mappingCfg.MappingID)
			return nil
		}
		corelog.Infof("Client: updating mapping %s (config changed)", mappingCfg.MappingID)
		handler.Stop()
		delete(c.mappingHandlers, mappingCfg.MappingID)
	}

	// ✅ 目标端配置（LocalPort==0）不需要启动监听
	if mappingCfg.LocalPort == 0 {
		corelog.Debugf("Client: skipping mapping %s (target-side, no local listener needed)", mappingCfg.MappingID)
		return nil
	}

	// 根据协议类型创建适配器和处理器
	protocol := mappingCfg.Protocol
	if protocol == "" {
		protocol = "tcp" // 默认 TCP
	}

	// SOCKS5 映射由 SOCKS5Manager 处理，不在这里创建
	if isSOCKS5Protocol(protocol) {
		return nil
	}

	// 创建协议适配器
	// ✅ 传入 client context，确保适配器能正确响应 client 的生命周期
	adapter, err := mapping.CreateAdapter(protocol, mappingCfg, c.GetContext())
	if err != nil {
		corelog.Errorf("Client: failed to create adapter: %v", err)
		return err
	}

	// 创建映射处理器（使用BaseMappingHandler）
	handler := mapping.NewBaseMappingHandler(c, mappingCfg, adapter)

	// 注册 TunnelManager 到 NotificationDispatcher（用于接收关闭通知）
	if tunnelManager := handler.GetTunnelManager(); tunnelManager != nil {
		c.notificationDispatcher.AddHandler(tunnelManager)
		corelog.Infof("Client: registered TunnelManager for mapping %s to NotificationDispatcher", mappingCfg.MappingID)
	}

	if err := handler.Start(); err != nil {
		corelog.Errorf("Client: ❌ failed to start %s mapping %s: %v", protocol, mappingCfg.MappingID, err)
		return err
	}

	c.mappingHandlers[mappingCfg.MappingID] = handler
	corelog.Infof("Client: %s mapping %s started successfully on port %d", protocol, mappingCfg.MappingID, mappingCfg.LocalPort)
	return nil
}

// isMappingConfigEqual 比较两个映射配置是否相同
// 只比较影响运行时行为的关键字段
func isMappingConfigEqual(a, b clientconfig.MappingConfig) bool {
	return a.MappingID == b.MappingID &&
		a.Protocol == b.Protocol &&
		a.LocalPort == b.LocalPort &&
		a.TargetHost == b.TargetHost &&
		a.TargetPort == b.TargetPort &&
		a.TargetClientID == b.TargetClientID &&
		a.SecretKey == b.SecretKey &&
		a.BandwidthLimit == b.BandwidthLimit &&
		a.MaxConnections == b.MaxConnections &&
		a.EnableCompression == b.EnableCompression &&
		a.CompressionLevel == b.CompressionLevel &&
		a.EnableEncryption == b.EnableEncryption &&
		a.EncryptionMethod == b.EncryptionMethod &&
		a.EncryptionKey == b.EncryptionKey
}

// isSOCKS5Protocol 判断是否为 SOCKS5 协议
// 兼容服务端使用的 "socks" 和客户端使用的 "socks5"
func isSOCKS5Protocol(protocol string) bool {
	return protocol == "socks5" || protocol == "socks"
}

// RemoveMapping 移除映射
func (c *TunnoxClient) RemoveMapping(mappingID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if handler, exists := c.mappingHandlers[mappingID]; exists {
		// 从 NotificationDispatcher 移除 TunnelManager
		if tunnelManager := handler.GetTunnelManager(); tunnelManager != nil {
			c.notificationDispatcher.RemoveHandler(tunnelManager)
			corelog.Infof("Client: removed TunnelManager for mapping %s from NotificationDispatcher", mappingID)
		}

		handler.Stop()
		delete(c.mappingHandlers, mappingID)
		corelog.Infof("Client: mapping %s stopped", mappingID)
	}
}
