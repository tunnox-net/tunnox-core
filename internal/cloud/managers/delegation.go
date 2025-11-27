package managers

import (
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

// 此文件包含所有委托给子Manager的方法

// ========== 匿名用户管理（委托给 AnonymousManager） ==========

// GenerateAnonymousCredentials 生成匿名凭据
func (c *CloudControl) GenerateAnonymousCredentials() (*models.Client, error) {
	return c.anonymousManager.GenerateAnonymousCredentials()
}

// GetAnonymousClient 获取匿名客户端
func (c *CloudControl) GetAnonymousClient(clientID int64) (*models.Client, error) {
	return c.anonymousManager.GetAnonymousClient(clientID)
}

// ListAnonymousClients 列出匿名客户端
func (c *CloudControl) ListAnonymousClients() ([]*models.Client, error) {
	return c.anonymousManager.ListAnonymousClients()
}

// DeleteAnonymousClient 删除匿名客户端
func (c *CloudControl) DeleteAnonymousClient(clientID int64) error {
	return c.anonymousManager.DeleteAnonymousClient(clientID)
}

// CreateAnonymousMapping 创建匿名映射
func (c *CloudControl) CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) {
	return c.anonymousManager.CreateAnonymousMapping(sourceClientID, targetClientID, protocol, sourcePort, targetPort)
}

// GetAnonymousMappings 获取匿名映射
func (c *CloudControl) GetAnonymousMappings() ([]*models.PortMapping, error) {
	return c.anonymousManager.GetAnonymousMappings()
}

// CleanupExpiredAnonymous 清理过期匿名资源
func (c *CloudControl) CleanupExpiredAnonymous() error {
	return c.anonymousManager.CleanupExpiredAnonymous()
}

// ========== 统计管理（委托给 StatsManager） ==========

// GetUserStats 获取用户统计
func (c *CloudControl) GetUserStats(userID string) (*stats.UserStats, error) {
	return c.statsManager.GetUserStats(userID)
}

// GetClientStats 获取客户端统计
func (c *CloudControl) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	return c.statsManager.GetClientStats(clientID)
}

// GetSystemStats 获取系统统计
func (c *CloudControl) GetSystemStats() (*stats.SystemStats, error) {
	return c.statsManager.GetSystemStats()
}

// GetTrafficStats 获取流量统计
func (c *CloudControl) GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error) {
	return c.statsManager.GetTrafficStats(timeRange)
}

// GetConnectionStats 获取连接统计
func (c *CloudControl) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) {
	return c.statsManager.GetConnectionStats(timeRange)
}

// ========== 搜索管理（委托给 SearchManager） ==========

// SearchUsers 搜索用户
func (c *CloudControl) SearchUsers(keyword string) ([]*models.User, error) {
	return c.searchManager.SearchUsers(keyword)
}

// SearchClients 搜索客户端
func (c *CloudControl) SearchClients(keyword string) ([]*models.Client, error) {
	return c.searchManager.SearchClients(keyword)
}

// SearchPortMappings 搜索端口映射
func (c *CloudControl) SearchPortMappings(keyword string) ([]*models.PortMapping, error) {
	return c.searchManager.SearchPortMappings(keyword)
}

// ========== 连接管理（委托给 ConnectionManager） ==========

// RegisterConnection 注册连接
func (c *CloudControl) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error {
	return c.connectionManager.RegisterConnection(mappingID, connInfo)
}

// UnregisterConnection 注销连接
func (c *CloudControl) UnregisterConnection(connID string) error {
	return c.connectionManager.UnregisterConnection(connID)
}

// GetConnections 获取映射的连接
func (c *CloudControl) GetConnections(mappingID string) ([]*models.ConnectionInfo, error) {
	return c.connectionManager.GetConnections(mappingID)
}

// GetClientConnections 获取客户端的连接
func (c *CloudControl) GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error) {
	return c.connectionManager.GetClientConnections(clientID)
}

// UpdateConnectionStats 更新连接统计
func (c *CloudControl) UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error {
	return c.connectionManager.UpdateConnectionStats(connID, bytesSent, bytesReceived)
}

