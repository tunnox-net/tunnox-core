package protocol

import (
	"context"
	"strings"
	"sync"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/protocol/registry"
	"tunnox-core/internal/utils"
	coreErrors "tunnox-core/internal/core/errors"
)

// ProtocolManager 协议管理器
// 职责：
// 1. 管理协议的注册（通过 Registry）
// 2. 管理协议的初始化（依赖验证、拓扑排序、初始化）
// 3. 管理适配器的生命周期（启动、关闭）
type ProtocolManager struct {
	*dispose.ManagerBase
	registry  *registry.Registry  // 协议注册表（职责：协议注册和查询）
	container registry.Container   // 依赖注入容器（职责：服务解析）
	adapters  map[string]adapter.Adapter // 适配器映射（职责：适配器生命周期管理）
	mu        sync.RWMutex        // 保护 adapters 的并发访问
}

// NewProtocolManager 创建协议管理器（支持依赖注入）
// parentCtx: 从 dispose 体系分配的上下文
// container: 依赖注入容器（可选，如果为 nil 则跳过依赖验证）
func NewProtocolManager(parentCtx context.Context, container registry.Container) *ProtocolManager {
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

// StartAll 启动所有适配器
// 注意：此方法会启动所有通过 InitializeProtocols 初始化的适配器
// 对于通过 InitializeProtocols 初始化的适配器，如果协议实现已经在 Initialize 时启动，
// 则 ListenFrom 可能会返回错误，这是正常的（适配器已经启动）
func (pm *ProtocolManager) StartAll() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	for name, a := range pm.adapters {
		// 检查适配器是否已经启动（通过检查地址是否已设置）
		if a.GetAddr() == "" {
			utils.Warnf("Adapter %s has no address configured, skipping", name)
			continue
		}
		
		// 尝试启动适配器
		// 注意：如果适配器已经在 Initialize 时启动，ListenFrom 可能会返回错误
		// 这里忽略"already listening"类型的错误
		utils.Infof("Starting %s adapter on %s", name, a.GetAddr())
		if err := a.ListenFrom(a.GetAddr()); err != nil {
			// 检查是否是"already listening"错误
			if isAlreadyListeningError(err) {
				utils.Infof("Adapter %s already started, skipping", name)
				continue
			}
			utils.Errorf("Failed to start adapter %s: %v", name, err)
			return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to start adapter %s", name)
		}
		utils.Infof("Successfully started %s adapter on %s", name, a.GetAddr())
	}
	return nil
}

// isAlreadyListeningError 检查是否是"already listening"错误
func isAlreadyListeningError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// 检查常见的"already listening"错误消息
	return strings.Contains(errStr, "already listening") ||
		strings.Contains(errStr, "address already in use") ||
		strings.Contains(errStr, "bind: address already in use")
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
