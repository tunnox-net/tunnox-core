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
	*dispose.ResourceBase
	dispose  utils.Dispose
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
		ResourceBase: dispose.NewResourceBase("ProtocolManager"),
		adapters:     make(map[string]adapter.Adapter),
	}
	manager.Initialize(parentCtx)
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
		utils.Infof("Starting adapter: %s, type: %T", name, a)
		if err := a.ListenFrom(a.GetAddr()); err != nil {
			utils.Errorf("Failed to start adapter %s: %v", name, err)
			return err
		}
		utils.Infof("Successfully started adapter: %s", name)
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
