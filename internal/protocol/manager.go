package protocol

import (
	"context"
	"sync"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/utils"
)

// ProtocolManager 协议管理器
type ProtocolManager struct {
	*dispose.ManagerBase
	dispose  dispose.Dispose
	adapters map[string]adapter.Adapter
	mu       sync.RWMutex
}

// Dispose 实现Disposable接口
func (pm *ProtocolManager) Dispose() error {
	return pm.CloseAll()
}

// NewProtocolManager 创建协议管理器
func NewProtocolManager(parentCtx context.Context) *ProtocolManager {
	manager := &ProtocolManager{
		ManagerBase: dispose.NewManager("ProtocolManager", parentCtx),
		adapters:    make(map[string]adapter.Adapter),
	}
	return manager
}

func (pm *ProtocolManager) Register(adapter adapter.Adapter) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.adapters[adapter.Name()] = adapter
}

func (pm *ProtocolManager) StartAll() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for name, a := range pm.adapters {
		// 精简日志：只在调试模式下输出适配器启动信息
		utils.Debugf("Starting adapter: %s", name)
		if err := a.ListenFrom(a.GetAddr()); err != nil {
			utils.Errorf("Failed to start adapter %s: %v", name, err)
			return err
		}
		// 精简日志：只在调试模式下输出适配器启动完成信息
		utils.Debugf("Successfully started adapter: %s", name)
	}
	return nil
}

func (pm *ProtocolManager) CloseAll() error {
	return pm.dispose.CloseWithError()
}

func (pm *ProtocolManager) onClose() error {
	hasAdapters := len(pm.adapters) > 0

	if hasAdapters {
		var lastErr error
		for _, adapter := range pm.adapters {
			if err := adapter.Close(); err != nil {
				utils.Errorf("Failed to close adapter %s: %v", adapter.Name(), err)
				lastErr = err
			}
		}
		if lastErr != nil {
			return lastErr
		}
	}

	pm.adapters = nil
	return nil
}
