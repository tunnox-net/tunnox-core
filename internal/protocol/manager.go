package protocol

import (
	"context"
	"sync"
	"tunnox-core/internal/utils"
)

type Manager struct {
	dispose  utils.Dispose
	adapters []Adapter
	lock     sync.Mutex
}

// Dispose 实现Disposable接口
func (pm *Manager) Dispose() error {
	// 直接调用onClose逻辑，避免递归调用
	return pm.onClose()
}

func NewManager(parentCtx context.Context) *Manager {
	pm := &Manager{}
	pm.dispose.SetCtx(parentCtx, pm.onClose)
	return pm
}

func (pm *Manager) Register(adapter Adapter) {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.adapters = append(pm.adapters, adapter)
}

func (pm *Manager) StartAll() error {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	for _, a := range pm.adapters {
		if err := a.ListenFrom(a.GetAddr()); err != nil {
			return err
		}
	}
	return nil
}

func (pm *Manager) CloseAll() error {
	return pm.dispose.CloseWithError()
}

func (pm *Manager) onClose() error {
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
