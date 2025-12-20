package protocol

import (
	"context"
	"sync"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/adapter"
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
		// 保留关键的适配器启动信息
		corelog.Infof("Starting %s adapter on %s", name, a.GetAddr())
		if err := a.ListenFrom(a.GetAddr()); err != nil {
			corelog.Errorf("Failed to start adapter %s: %v", name, err)
			return err
		}
		// 保留关键的适配器启动完成信息
		corelog.Infof("Successfully started %s adapter on %s", name, a.GetAddr())
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
				corelog.Errorf("Failed to close adapter %s: %v", adapter.Name(), err)
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
