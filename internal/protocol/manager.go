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

func (pm *Manager) CloseAll() {
	pm.adapters = nil
	pm.Ctx().Done()
}

func (pm *Manager) onClose() {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	if pm.IsClosed() {
		return
	}

	hasAdapters := len(pm.adapters) > 0

	if hasAdapters {
		pm.CloseAll()
	}
}
