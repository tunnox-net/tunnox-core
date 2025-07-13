package protocol

import (
	"context"
	"sync"
	"tunnox-core/internal/utils"
)

type Manager struct {
	utils.Dispose
	adapters []Adapter
	lock     sync.Mutex
}

func NewManager(parentCtx context.Context) *Manager {
	pm := &Manager{}
	pm.SetCtx(parentCtx, pm.onClose)
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
	return pm.Dispose.Close()
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
