package mobile

import (
	"encoding/json"
	"fmt"

	"tunnox-core/internal/cloud/models"
)

// AddSocks5Mapping 添加 SOCKS5 映射
// mappingID: 映射 ID（由服务器分配）
// listenPort: 本地监听端口
// targetClientID: 目标客户端 ID（流量要转发到哪个客户端）
// secretKey: 映射密钥（用于隧道认证）
// 返回错误信息，成功返回空字符串
func (c *TunnoxMobileClient) AddSocks5Mapping(mappingID string, listenPort int64, targetClientID int64, secretKey string) string {
	// 构造 PortMapping 对象
	portMapping := &models.PortMapping{
		ID:             mappingID,
		Protocol:       "socks",
		SourcePort:     int(listenPort),
		TargetClientID: targetClientID,
		ListenClientID: c.GetClientID(),
		SecretKey:      secretKey,
		Status:         "active",
	}

	// 获取 SOCKS5 管理器
	socks5Mgr := c.client.GetSocks5Manager()
	if socks5Mgr == nil {
		return "SOCKS5 manager not initialized"
	}

	// 添加映射
	err := socks5Mgr.AddMapping(portMapping)
	if err != nil {
		errMsg := fmt.Sprintf("failed to add SOCKS5 mapping: %v", err)
		c.notifyError(errMsg)
		return errMsg
	}

	// 通知成功
	c.notifySocks5Started(mappingID, listenPort)
	return ""
}

// RemoveSocks5Mapping 移除 SOCKS5 映射
// mappingID: 映射 ID
func (c *TunnoxMobileClient) RemoveSocks5Mapping(mappingID string) {
	socks5Mgr := c.client.GetSocks5Manager()
	if socks5Mgr == nil {
		return
	}

	socks5Mgr.RemoveMapping(mappingID)
	c.notifySocks5Stopped(mappingID)
}

// ListSocks5Mappings 列出所有 SOCKS5 映射
// 返回映射列表的 JSON 字符串
func (c *TunnoxMobileClient) ListSocks5Mappings() string {
	socks5Mgr := c.client.GetSocks5Manager()
	if socks5Mgr == nil {
		return "[]"
	}

	mappingIDs := socks5Mgr.ListMappings()
	if len(mappingIDs) == 0 {
		return "[]"
	}

	// 构造映射信息列表
	mappings := make([]map[string]interface{}, 0, len(mappingIDs))
	for _, mappingID := range mappingIDs {
		listener, exists := socks5Mgr.GetMapping(mappingID)
		if !exists {
			continue
		}

		listenAddr := listener.GetListenAddr()
		mappings = append(mappings, map[string]interface{}{
			"id":          mappingID,
			"listen_addr": listenAddr,
			"status":      "active",
		})
	}

	// 转为 JSON
	jsonBytes, err := json.Marshal(mappings)
	if err != nil {
		return "[]"
	}

	return string(jsonBytes)
}

// GetSocks5MappingCount 获取 SOCKS5 映射数量
func (c *TunnoxMobileClient) GetSocks5MappingCount() int64 {
	socks5Mgr := c.client.GetSocks5Manager()
	if socks5Mgr == nil {
		return 0
	}

	return int64(len(socks5Mgr.ListMappings()))
}

// IsSocks5MappingActive 检查指定映射是否激活
// mappingID: 映射 ID
func (c *TunnoxMobileClient) IsSocks5MappingActive(mappingID string) bool {
	socks5Mgr := c.client.GetSocks5Manager()
	if socks5Mgr == nil {
		return false
	}

	_, exists := socks5Mgr.GetMapping(mappingID)
	return exists
}

// GetSocks5ListenAddr 获取指定映射的监听地址
// mappingID: 映射 ID
// 返回监听地址，如 ":1080"，如果映射不存在返回空字符串
func (c *TunnoxMobileClient) GetSocks5ListenAddr(mappingID string) string {
	socks5Mgr := c.client.GetSocks5Manager()
	if socks5Mgr == nil {
		return ""
	}

	listener, exists := socks5Mgr.GetMapping(mappingID)
	if !exists {
		return ""
	}

	return listener.GetListenAddr()
}

// StopAllSocks5Mappings 停止所有 SOCKS5 映射
func (c *TunnoxMobileClient) StopAllSocks5Mappings() {
	socks5Mgr := c.client.GetSocks5Manager()
	if socks5Mgr == nil {
		return
	}

	mappingIDs := socks5Mgr.ListMappings()
	for _, mappingID := range mappingIDs {
		c.RemoveSocks5Mapping(mappingID)
	}
}

// CreateSocks5MappingFromJSON 从 JSON 创建 SOCKS5 映射
// mappingJSON: 映射配置的 JSON 字符串，格式：
// {
//   "id": "mapping-id",
//   "listen_port": 1080,
//   "target_client_id": 12345,
//   "secret_key": "secret"
// }
// 返回错误信息，成功返回空字符串
func (c *TunnoxMobileClient) CreateSocks5MappingFromJSON(mappingJSON string) string {
	var mapping struct {
		ID             string `json:"id"`
		ListenPort     int    `json:"listen_port"`
		TargetClientID int64  `json:"target_client_id"`
		SecretKey      string `json:"secret_key"`
	}

	err := json.Unmarshal([]byte(mappingJSON), &mapping)
	if err != nil {
		return fmt.Sprintf("failed to parse JSON: %v", err)
	}

	return c.AddSocks5Mapping(
		mapping.ID,
		int64(mapping.ListenPort),
		mapping.TargetClientID,
		mapping.SecretKey,
	)
}

// GetSocks5MappingJSON 获取指定映射的 JSON 表示
// mappingID: 映射 ID
// 返回 JSON 字符串，如果映射不存在返回空对象 "{}"
func (c *TunnoxMobileClient) GetSocks5MappingJSON(mappingID string) string {
	socks5Mgr := c.client.GetSocks5Manager()
	if socks5Mgr == nil {
		return "{}"
	}

	listener, exists := socks5Mgr.GetMapping(mappingID)
	if !exists {
		return "{}"
	}

	result := map[string]interface{}{
		"id":          mappingID,
		"listen_addr": listener.GetListenAddr(),
		"status":      "active",
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}

	return string(jsonBytes)
}
