// Package transport 传输层协议注册表
// 支持通过 build tags 选择性编译协议支持
package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"sync"

	coreerrors "tunnox-core/internal/core/errors"
)

// tlsConfigKey 用于在 context 中传递 TLS 配置
type tlsConfigKey struct{}

// TLSOptions TLS 配置选项
type TLSOptions struct {
	InsecureSkipVerify bool   // 是否跳过证书验证
	CACertFile         string // CA 证书文件路径
	ServerName         string // 服务器名称（用于证书验证）
}

// WithTLSOptions 将 TLS 配置添加到 context
func WithTLSOptions(ctx context.Context, opts *TLSOptions) context.Context {
	return context.WithValue(ctx, tlsConfigKey{}, opts)
}

// GetTLSOptions 从 context 获取 TLS 配置
func GetTLSOptions(ctx context.Context) *TLSOptions {
	if v := ctx.Value(tlsConfigKey{}); v != nil {
		return v.(*TLSOptions)
	}
	return nil
}

// BuildTLSConfig 根据 TLSOptions 构建 tls.Config
func BuildTLSConfig(opts *TLSOptions, nextProtos []string) (*tls.Config, error) {
	tlsConf := &tls.Config{
		NextProtos: nextProtos,
	}

	if opts == nil {
		// 默认行为：跳过证书验证（向后兼容）
		tlsConf.InsecureSkipVerify = true
		return tlsConf, nil
	}

	tlsConf.InsecureSkipVerify = opts.InsecureSkipVerify
	tlsConf.ServerName = opts.ServerName

	// 如果提供了 CA 证书文件，加载它
	if opts.CACertFile != "" {
		caCert, err := os.ReadFile(opts.CACertFile)
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeConfigError, "failed to read CA cert file")
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, coreerrors.New(coreerrors.CodeConfigError, "failed to parse CA cert")
		}
		tlsConf.RootCAs = caCertPool
	}

	return tlsConf, nil
}

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
