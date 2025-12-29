// Package client socks5_manager.go
// SOCKS5 管理器 facade - 向后兼容层
// 实际实现已移至 internal/client/socks5 子包
package client

import (
	"context"

	"tunnox-core/internal/client/socks5"
)

// SOCKS5Manager SOCKS5 映射管理器
// Deprecated: 请使用 socks5.Manager
type SOCKS5Manager = socks5.Manager

// NewSOCKS5Manager 创建 SOCKS5 管理器
// Deprecated: 请使用 socks5.NewManager
func NewSOCKS5Manager(ctx context.Context, clientID int64, tunnelCreator SOCKS5TunnelCreator) *SOCKS5Manager {
	return socks5.NewManager(ctx, clientID, tunnelCreator)
}
