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

func (pm *Manager) StartAll(ctx context.Context) error {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	for _, a := range pm.adapters {
		if err := a.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (pm *Manager) CloseAll() {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	for _, a := range pm.adapters {
		a.Close()
	}
	pm.Dispose.Close()
}

func (pm *Manager) onClose() {
	pm.CloseAll()
}
