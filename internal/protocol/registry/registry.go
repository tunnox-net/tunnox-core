package registry

import (
	"context"
	"sync"

	"tunnox-core/internal/protocol/adapter"
	coreErrors "tunnox-core/internal/core/errors"
)

// Protocol 协议接口
type Protocol interface {
	// Name 返回协议名称
	Name() string

	// Initialize 初始化协议
	// ctx: 从 dispose 体系分配的上下文
	// container: 依赖注入容器
	// config: 协议配置
	Initialize(ctx context.Context, container Container, config *Config) (adapter.Adapter, error)

	// Dependencies 返回协议依赖的服务名称列表
	Dependencies() []string

	// ValidateConfig 验证配置
	ValidateConfig(config *Config) error
}

// Config 协议配置
type Config struct {
	Name    string
	Enabled bool
	Host    string
	Port    int
	Options map[string]interface{} // 协议特定选项
}

// Registry 协议注册表
type Registry struct {
	protocols map[string]Protocol
	mu        sync.RWMutex
}

// NewRegistry 创建协议注册表
func NewRegistry() *Registry {
	return &Registry{
		protocols: make(map[string]Protocol),
	}
}

// Register 注册协议
func (r *Registry) Register(protocol Protocol) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := protocol.Name()
	if name == "" {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "protocol name cannot be empty")
	}

	if _, exists := r.protocols[name]; exists {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "protocol %s already registered", name)
	}

	r.protocols[name] = protocol
	return nil
}

// Get 获取协议
func (r *Registry) Get(name string) (Protocol, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protocol, exists := r.protocols[name]
	if !exists {
		return nil, coreErrors.Newf(coreErrors.ErrorTypePermanent, "protocol %s not registered", name)
	}
	return protocol, nil
}

// List 列出所有已注册的协议
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.protocols))
	for name := range r.protocols {
		names = append(names, name)
	}
	return names
}

// HasProtocol 检查协议是否已注册
func (r *Registry) HasProtocol(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.protocols[name]
	return exists
}

