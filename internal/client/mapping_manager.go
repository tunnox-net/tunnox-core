package client

import (
	"encoding/json"

	"tunnox-core/internal/client/mapping"
	clientconfig "tunnox-core/internal/config"
	"tunnox-core/internal/utils"
)

// handleConfigUpdate 处理配置更新
func (c *TunnoxClient) handleConfigUpdate(configBody string) {
	utils.Infof("Client: ✅ received ConfigSet from server, body length=%d", len(configBody))

	var configUpdate struct {
		Mappings []clientconfig.MappingConfig `json:"mappings"`
	}

	if err := json.Unmarshal([]byte(configBody), &configUpdate); err != nil {
		utils.Errorf("Client: failed to parse config update: %v", err)
		return
	}

	utils.Infof("Client: parsed %d mappings from ConfigSet", len(configUpdate.Mappings))

	// 构建新配置的映射ID集合
	newMappingIDs := make(map[string]bool)
	for i, mappingConfig := range configUpdate.Mappings {
		utils.Infof("Client: processing mapping[%d]: ID=%s, Protocol=%s, LocalPort=%d",
			i, mappingConfig.MappingID, mappingConfig.Protocol, mappingConfig.LocalPort)
		newMappingIDs[mappingConfig.MappingID] = true
		c.addOrUpdateMapping(mappingConfig)
	}

	// 删除不再存在的映射
	c.mu.Lock()
	for mappingID, handler := range c.mappingHandlers {
		if !newMappingIDs[mappingID] {
			utils.Infof("Client: removing mapping %s (no longer in config)", mappingID)
			handler.Stop()
			delete(c.mappingHandlers, mappingID)
		}
	}
	c.mu.Unlock()

	utils.Infof("Client: ✅ config updated successfully, total active mappings=%d", len(newMappingIDs))
}

// addOrUpdateMapping 添加或更新映射
func (c *TunnoxClient) addOrUpdateMapping(mappingCfg clientconfig.MappingConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在，存在则先停止
	if handler, exists := c.mappingHandlers[mappingCfg.MappingID]; exists {
		utils.Infof("Client: updating mapping %s", mappingCfg.MappingID)
		handler.Stop()
		delete(c.mappingHandlers, mappingCfg.MappingID)
	}

	// ✅ 目标端配置（LocalPort==0）不需要启动监听
	if mappingCfg.LocalPort == 0 {
		utils.Debugf("Client: skipping mapping %s (target-side, no local listener needed)", mappingCfg.MappingID)
		return
	}

	// 根据协议类型创建适配器和处理器
	protocol := mappingCfg.Protocol
	if protocol == "" {
		protocol = "tcp" // 默认 TCP
	}

	// 创建协议适配器
	adapter, err := mapping.CreateAdapter(protocol, mappingCfg)
	if err != nil {
		utils.Errorf("Client: failed to create adapter: %v", err)
		return
	}

	// 创建映射处理器（使用BaseMappingHandler）
	handler := mapping.NewBaseMappingHandler(c, mappingCfg, adapter)

	if err := handler.Start(); err != nil {
		utils.Errorf("Client: ❌ failed to start %s mapping %s: %v", protocol, mappingCfg.MappingID, err)
		return
	}

	c.mappingHandlers[mappingCfg.MappingID] = handler
	utils.Infof("Client: ✅ %s mapping %s started successfully on port %d", protocol, mappingCfg.MappingID, mappingCfg.LocalPort)
}

// RemoveMapping 移除映射
func (c *TunnoxClient) RemoveMapping(mappingID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if handler, exists := c.mappingHandlers[mappingID]; exists {
		handler.Stop()
		delete(c.mappingHandlers, mappingID)
		utils.Infof("Client: mapping %s stopped", mappingID)
	}
}

