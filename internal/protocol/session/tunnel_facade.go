// Package session 提供会话管理功能
// 本文件为 tunnel 子包提供向后兼容的类型别名和函数包装
package session

import (
	"context"
	"net"
	"time"

	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/protocol/session/tunnel"
	"tunnox-core/internal/stream"
)

// ============================================================================
// tunnel 子包类型别名（向后兼容）
// ============================================================================

// TunnelBridge 隧道桥接器（类型别名）
type TunnelBridge = tunnel.Bridge

// TunnelBridgeConfig 隧道桥接器配置（类型别名）
type TunnelBridgeConfig = tunnel.BridgeConfig

// TunnelBridgeAccessor 隧道桥接访问器接口（类型别名）
type TunnelBridgeAccessor = tunnel.BridgeAccessor

// TunnelWaitingState 隧道等待状态（类型别名）
type TunnelWaitingState = tunnel.WaitingState

// TunnelRoutingTable 隧道路由表（类型别名）
type TunnelRoutingTable = tunnel.RoutingTable

// DataForwarder 数据转发器接口（类型别名）
type DataForwarder = tunnel.DataForwarder

// StreamDataForwarder 流数据转发器接口（类型别名）
type StreamDataForwarder = tunnel.StreamDataForwarder

// ============================================================================
// tunnel 错误重新导出
// ============================================================================

var (
	// ErrTunnelNotFound Tunnel未找到错误
	ErrTunnelNotFound = tunnel.ErrNotFound

	// ErrTunnelExpired Tunnel已过期错误
	ErrTunnelExpired = tunnel.ErrExpired
)

// ============================================================================
// tunnel 函数包装（向后兼容）
// ============================================================================

// NewTunnelBridge 创建隧道桥接器
func NewTunnelBridge(parentCtx context.Context, config *TunnelBridgeConfig) *TunnelBridge {
	return tunnel.NewBridge(parentCtx, config)
}

// NewTunnelRoutingTable 创建隧道路由表
func NewTunnelRoutingTable(s storage.Storage, ttl time.Duration) *TunnelRoutingTable {
	return tunnel.NewRoutingTable(s, ttl)
}

// createDataForwarder 创建数据转发器（内部使用）
func createDataForwarder(conn interface{}, s stream.PackageStreamer) DataForwarder {
	if netConn, ok := conn.(net.Conn); ok {
		return tunnel.CreateDataForwarder(netConn, s)
	}
	return tunnel.CreateDataForwarder(conn, s)
}

// ============================================================================
// 初始化 - 注册隧道连接工厂
// ============================================================================

func init() {
	// 注册隧道连接工厂到 tunnel 子包
	tunnel.SetTunnelConnectionFactory(func(
		connID string,
		conn net.Conn,
		s stream.PackageStreamer,
		clientID int64,
		mappingID string,
		tunnelID string,
	) tunnel.TunnelConnectionInterface {
		return CreateTunnelConnection(connID, conn, s, clientID, mappingID, tunnelID)
	})
}
