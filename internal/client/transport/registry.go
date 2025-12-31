// Package transport 传输层协议注册表
// 支持通过 build tags 选择性编译协议支持
package transport

import (
	"context"
	"net"
	"sync"

	coreerrors "tunnox-core/internal/core/errors"
)

// Dialer 是协议拨号器的接口
type Dialer func(ctx context.Context, address string) (net.Conn, error)

// ProtocolInfo 协议信息
type ProtocolInfo struct {
	Name     string // 协议名称: tcp, websocket, quic, kcp
	Priority int    // 优先级（数字越小优先级越高）
	Dialer   Dialer // 拨号函数
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]*ProtocolInfo)
)

// RegisterProtocol 注册协议
func RegisterProtocol(name string, priority int, dialer Dialer) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = &ProtocolInfo{
		Name:     name,
		Priority: priority,
		Dialer:   dialer,
	}
}

// GetProtocol 获取协议信息
func GetProtocol(name string) (*ProtocolInfo, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	info, ok := registry[name]
	return info, ok
}

// GetRegisteredProtocols 获取所有已注册的协议（按优先级排序）
func GetRegisteredProtocols() []*ProtocolInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()

	// 收集所有协议
	protocols := make([]*ProtocolInfo, 0, len(registry))
	for _, info := range registry {
		protocols = append(protocols, info)
	}

	// 按优先级排序
	for i := 0; i < len(protocols); i++ {
		for j := i + 1; j < len(protocols); j++ {
			if protocols[j].Priority < protocols[i].Priority {
				protocols[i], protocols[j] = protocols[j], protocols[i]
			}
		}
	}

	return protocols
}

// IsProtocolAvailable 检查协议是否可用
func IsProtocolAvailable(name string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[name]
	return ok
}

// Dial 使用指定协议建立连接
func Dial(ctx context.Context, protocol, address string) (net.Conn, error) {
	info, ok := GetProtocol(protocol)
	if !ok {
		return nil, coreerrors.Newf(coreerrors.CodeProtocolError, "protocol %q is not available (not compiled in)", protocol)
	}
	return info.Dialer(ctx, address)
}

// GetAvailableProtocolNames 获取所有可用协议名称
func GetAvailableProtocolNames() []string {
	protocols := GetRegisteredProtocols()
	names := make([]string, len(protocols))
	for i, p := range protocols {
		names[i] = p.Name
	}
	return names
}
