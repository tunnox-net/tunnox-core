package protocol

import (
	"context"
	"sync"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/protocol/registry"
	"tunnox-core/internal/utils"
	coreErrors "tunnox-core/internal/core/errors"
)

// ProtocolManager 协议管理器
type ProtocolManager struct {
	*dispose.ManagerBase
	registry  *registry.Registry
	container registry.Container
	adapters  map[string]adapter.Adapter
	mu        sync.RWMutex
}

// NewProtocolManager 创建协议管理器（兼容旧版本）
func NewProtocolManager(parentCtx context.Context) *ProtocolManager {
	manager := &ProtocolManager{
		ManagerBase: dispose.NewManager("ProtocolManager", parentCtx),
		registry:    registry.NewRegistry(),
		adapters:    make(map[string]adapter.Adapter),
	}
	manager.AddCleanHandler(manager.onClose)
	return manager
}

// NewProtocolManagerWithContainer 创建协议管理器（新版本，支持依赖注入）
func NewProtocolManagerWithContainer(parentCtx context.Context, container registry.Container) *ProtocolManager {
	manager := &ProtocolManager{
		ManagerBase: dispose.NewManager("ProtocolManager", parentCtx),
		registry:    registry.NewRegistry(),
		container:   container,
		adapters:    make(map[string]adapter.Adapter),
	}
	manager.AddCleanHandler(manager.onClose)
	return manager
}

// RegisterProtocol 注册协议实现
func (pm *ProtocolManager) RegisterProtocol(protocol registry.Protocol) error {
	return pm.registry.Register(protocol)
}

// Register 注册适配器（向后兼容方法）
func (pm *ProtocolManager) Register(adapter adapter.Adapter) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.adapters[adapter.Name()] = adapter
}

func (pm *ProtocolManager) StartAll() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for name, a := range pm.adapters {
		// 保留关键的适配器启动信息
		utils.Infof("Starting %s adapter on %s", name, a.GetAddr())
		if err := a.ListenFrom(a.GetAddr()); err != nil {
			utils.Errorf("Failed to start adapter %s: %v", name, err)
			return err
		}
		// 保留关键的适配器启动完成信息
		utils.Infof("Successfully started %s adapter on %s", name, a.GetAddr())
	}
	return nil
}

// InitializeProtocols 初始化所有启用的协议
func (pm *ProtocolManager) InitializeProtocols(configs map[string]*registry.Config) error {
	// 1. 验证依赖
	if err := pm.validateDependencies(configs); err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "dependency validation failed")
	}

	// 2. 按依赖顺序初始化
	initOrder, err := pm.resolveInitOrder(configs)
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to resolve init order")
	}

	// 3. 依次初始化
	for _, protocolName := range initOrder {
		config := configs[protocolName]
		if !config.Enabled {
			continue
		}

		protocol, err := pm.registry.Get(protocolName)
		if err != nil {
			return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "protocol %s not registered", protocolName)
		}

		// 验证配置
		if err := protocol.ValidateConfig(config); err != nil {
			return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "invalid config for protocol %s", protocolName)
		}

		// 初始化协议（使用 dispose 体系的上下文）
		adapter, err := protocol.Initialize(pm.Ctx(), pm.container, config)
		if err != nil {
			return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to initialize protocol %s", protocolName)
		}

		pm.mu.Lock()
		pm.adapters[protocolName] = adapter
		pm.mu.Unlock()
	}

	return nil
}

// validateDependencies 验证依赖
func (pm *ProtocolManager) validateDependencies(configs map[string]*registry.Config) error {
	if pm.container == nil {
		// 如果没有容器，跳过依赖验证（向后兼容）
		return nil
	}

	for protocolName, config := range configs {
		if !config.Enabled {
			continue
		}

		protocol, err := pm.registry.Get(protocolName)
		if err != nil {
			continue // 未注册的协议跳过
		}

		deps := protocol.Dependencies()
		for _, depName := range deps {
			if !pm.container.HasService(depName) {
				return coreErrors.Newf(coreErrors.ErrorTypePermanent,
					"protocol %s requires service %s, but it's not available", protocolName, depName)
			}
		}
	}
	return nil
}

// resolveInitOrder 解析初始化顺序（拓扑排序）
func (pm *ProtocolManager) resolveInitOrder(configs map[string]*registry.Config) ([]string, error) {
	// 构建协议依赖图（只考虑协议之间的依赖，忽略对服务的依赖）
	graph := make(map[string][]string)
	
	// 收集所有启用的协议名称
	enabledProtocols := make(map[string]bool)
	for protocolName, config := range configs {
		if !config.Enabled {
			continue
		}
		enabledProtocols[protocolName] = true
	}
	
	// 构建依赖图（只包含协议之间的依赖）
	for protocolName, config := range configs {
		if !config.Enabled {
			continue
		}
		protocol, err := pm.registry.Get(protocolName)
		if err != nil {
			continue
		}
		
		// 只保留那些也是协议的依赖（过滤掉服务依赖）
		protocolDeps := make([]string, 0)
		for _, dep := range protocol.Dependencies() {
			// 如果依赖项也是一个启用的协议，则加入依赖图
			if enabledProtocols[dep] {
				protocolDeps = append(protocolDeps, dep)
			}
			// 否则，这是一个服务依赖，不需要参与拓扑排序
		}
		graph[protocolName] = protocolDeps
	}

	// 拓扑排序
	return registry.TopologicalSort(graph)
}

// CloseAll 关闭所有适配器（向后兼容方法）
func (pm *ProtocolManager) CloseAll() error {
	return pm.Close()
}

// onClose 资源清理回调
func (pm *ProtocolManager) onClose() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var lastErr error
	for _, adapter := range pm.adapters {
		if err := adapter.Close(); err != nil {
			utils.LogErrorf(err, "Failed to close adapter %s: %v", adapter.Name(), err)
			lastErr = err
		}
	}

	pm.adapters = nil
	return lastErr
}
