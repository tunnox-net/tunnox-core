package mobile

import (
	"context"
	"fmt"
	"sync"

	"github.com/xjasonlyu/tun2socks/v2/engine"
)

// Tun2SocksEngine tun2socks 引擎封装
type Tun2SocksEngine struct {
	ctx    context.Context
	cancel context.CancelFunc
	key    *engine.Key
	mu     sync.RWMutex
}

// NewTun2SocksEngine 创建 tun2socks 引擎
func NewTun2SocksEngine() *Tun2SocksEngine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Tun2SocksEngine{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动 tun2socks
// tunFd: VpnService.Builder.establish() 返回的文件描述符
// socks5Addr: 本地 SOCKS5 地址，如 "127.0.0.1:1080"
// mtu: MTU 值，建议 1500
// 返回错误信息，成功返回空字符串
func (e *Tun2SocksEngine) Start(tunFd int64, socks5Addr string, mtu int64) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.key != nil {
		return "engine already running"
	}

	key := &engine.Key{
		Device:   fmt.Sprintf("fd://%d", tunFd),
		Proxy:    fmt.Sprintf("socks5://%s", socks5Addr),
		MTU:      int(mtu),
		LogLevel: "info",
	}

	// Insert configuration and start engine
	engine.Insert(key)
	engine.Start()

	e.key = key
	return ""
}

// Stop 停止 tun2socks
func (e *Tun2SocksEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.key != nil {
		engine.Stop()
		e.key = nil
	}
	e.cancel()
}

// IsRunning 检查是否正在运行
func (e *Tun2SocksEngine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.key != nil
}

// GetProxy 获取当前代理地址
func (e *Tun2SocksEngine) GetProxy() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.key == nil {
		return ""
	}
	return e.key.Proxy
}

// GetMTU 获取当前 MTU
func (e *Tun2SocksEngine) GetMTU() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.key == nil {
		return 0
	}
	return int64(e.key.MTU)
}

// SetLogLevel 设置日志级别
// level: "debug", "info", "warn", "error"
func (e *Tun2SocksEngine) SetLogLevel(level string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.key != nil {
		e.key.LogLevel = level
	}
}

// Restart 重启引擎
// tunFd: 新的 TUN 文件描述符
// socks5Addr: SOCKS5 地址
// mtu: MTU 值
func (e *Tun2SocksEngine) Restart(tunFd int64, socks5Addr string, mtu int64) string {
	e.Stop()
	return e.Start(tunFd, socks5Addr, mtu)
}
