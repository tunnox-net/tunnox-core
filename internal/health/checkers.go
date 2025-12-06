package health

import (
	"context"
	"time"
)

// StorageHealthChecker 存储健康检查器
type StorageHealthChecker struct {
	storage StorageChecker
}

// StorageChecker 存储检查接口
type StorageChecker interface {
	// Ping 检查存储连接是否正常
	// 如果存储实现了 Ping 方法，直接调用
	// 否则使用 Exists 方法作为健康检查
	Ping(ctx context.Context) error
}

// NewStorageHealthChecker 创建存储健康检查器
func NewStorageHealthChecker(storage StorageChecker) *StorageHealthChecker {
	return &StorageHealthChecker{storage: storage}
}

// Check 检查存储健康状态
func (c *StorageHealthChecker) Check(ctx context.Context) (*ComponentHealth, error) {
	if c.storage == nil {
		return &ComponentHealth{
			Name:      "storage",
			Status:    ComponentStatusUnhealthy,
			Message:   "storage not configured",
			LastCheck: time.Now(),
		}, nil
	}

	err := c.storage.Ping(ctx)
	if err != nil {
		return &ComponentHealth{
			Name:      "storage",
			Status:    ComponentStatusUnhealthy,
			Message:   err.Error(),
			LastCheck: time.Now(),
		}, nil
	}

	return &ComponentHealth{
		Name:      "storage",
		Status:    ComponentStatusHealthy,
		LastCheck: time.Now(),
	}, nil
}

// BrokerHealthChecker 消息代理健康检查器
type BrokerHealthChecker struct {
	broker BrokerChecker
}

// BrokerChecker 消息代理检查接口
type BrokerChecker interface {
	Ping(ctx context.Context) error
}

// NewBrokerHealthChecker 创建消息代理健康检查器
func NewBrokerHealthChecker(broker BrokerChecker) *BrokerHealthChecker {
	return &BrokerHealthChecker{broker: broker}
}

// Check 检查消息代理健康状态
func (c *BrokerHealthChecker) Check(ctx context.Context) (*ComponentHealth, error) {
	if c.broker == nil {
		return &ComponentHealth{
			Name:      "broker",
			Status:    ComponentStatusUnhealthy,
			Message:   "broker not configured",
			LastCheck: time.Now(),
		}, nil
	}

	err := c.broker.Ping(ctx)
	if err != nil {
		return &ComponentHealth{
			Name:      "broker",
			Status:    ComponentStatusUnhealthy,
			Message:   err.Error(),
			LastCheck: time.Now(),
		}, nil
	}

	return &ComponentHealth{
		Name:      "broker",
		Status:    ComponentStatusHealthy,
		LastCheck: time.Now(),
	}, nil
}

// ProtocolHealthChecker 协议子系统健康检查器
type ProtocolHealthChecker struct {
	sessionManager SessionManagerChecker
}

// SessionManagerChecker 会话管理器检查接口
type SessionManagerChecker interface {
	GetActiveConnections() int
	GetActiveTunnels() int
}

// NewProtocolHealthChecker 创建协议子系统健康检查器
func NewProtocolHealthChecker(sessionManager SessionManagerChecker) *ProtocolHealthChecker {
	return &ProtocolHealthChecker{sessionManager: sessionManager}
}

// Check 检查协议子系统健康状态
func (c *ProtocolHealthChecker) Check(ctx context.Context) (*ComponentHealth, error) {
	if c.sessionManager == nil {
		return &ComponentHealth{
			Name:      "protocol",
			Status:    ComponentStatusDegraded,
			Message:   "session manager not configured",
			LastCheck: time.Now(),
		}, nil
	}

	// 检查是否有活跃连接和隧道
	activeConns := c.sessionManager.GetActiveConnections()
	activeTunnels := c.sessionManager.GetActiveTunnels()

	// 协议子系统通常总是健康的（只要 session manager 存在）
	// 但可以通过连接数判断是否降级
	status := ComponentStatusHealthy
	message := ""
	if activeConns == 0 && activeTunnels == 0 {
		// 没有活跃连接，可能是刚启动或所有连接已断开
		// 这不算不健康，只是没有活动
		message = "no active connections"
	}

	return &ComponentHealth{
		Name:      "protocol",
		Status:    status,
		Message:   message,
		LastCheck: time.Now(),
	}, nil
}

