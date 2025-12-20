package client

import (
	"encoding/json"

	"tunnox-core/internal/client/mapping"
	"tunnox-core/internal/cloud/models"
	clientconfig "tunnox-core/internal/config"
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

		// SOCKS5 映射由 SOCKS5Manager 处理
		if mappingConfig.Protocol == "socks5" && mappingConfig.LocalPort > 0 {
			newSOCKS5MappingIDs[mappingConfig.MappingID] = true
			c.addOrUpdateSOCKS5Mapping(mappingConfig)
		} else {
			c.addOrUpdateMapping(mappingConfig)
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
func (c *TunnoxClient) addOrUpdateSOCKS5Mapping(mappingCfg clientconfig.MappingConfig) {
	if c.socks5Manager == nil {
		corelog.Errorf("Client: SOCKS5Manager not initialized")
		return
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
	}
}

// addOrUpdateMapping 添加或更新映射
func (c *TunnoxClient) addOrUpdateMapping(mappingCfg clientconfig.MappingConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在，存在则先停止
	if handler, exists := c.mappingHandlers[mappingCfg.MappingID]; exists {
		corelog.Infof("Client: updating mapping %s", mappingCfg.MappingID)
		handler.Stop()
		delete(c.mappingHandlers, mappingCfg.MappingID)
	}

	// ✅ 目标端配置（LocalPort==0）不需要启动监听
	if mappingCfg.LocalPort == 0 {
		corelog.Debugf("Client: skipping mapping %s (target-side, no local listener needed)", mappingCfg.MappingID)
		return
	}

	// 根据协议类型创建适配器和处理器
	protocol := mappingCfg.Protocol
	if protocol == "" {
		protocol = "tcp" // 默认 TCP
	}

	// SOCKS5 映射由 SOCKS5Manager 处理，不在这里创建
	if protocol == "socks5" {
		return
	}

	// 创建协议适配器
	// ✅ 传入 client context，确保适配器能正确响应 client 的生命周期
	adapter, err := mapping.CreateAdapter(protocol, mappingCfg, c.GetContext())
	if err != nil {
		corelog.Errorf("Client: failed to create adapter: %v", err)
		return
	}

	// 创建映射处理器（使用BaseMappingHandler）
	handler := mapping.NewBaseMappingHandler(c, mappingCfg, adapter)

	if err := handler.Start(); err != nil {
		corelog.Errorf("Client: ❌ failed to start %s mapping %s: %v", protocol, mappingCfg.MappingID, err)
		return
	}

	c.mappingHandlers[mappingCfg.MappingID] = handler
	corelog.Infof("Client: %s mapping %s started successfully on port %d", protocol, mappingCfg.MappingID, mappingCfg.LocalPort)
}

// RemoveMapping 移除映射
func (c *TunnoxClient) RemoveMapping(mappingID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if handler, exists := c.mappingHandlers[mappingID]; exists {
		handler.Stop()
		delete(c.mappingHandlers, mappingID)
		corelog.Infof("Client: mapping %s stopped", mappingID)
	}
}
