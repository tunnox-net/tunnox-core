package protocol

import (
	"context"
	"sync"
	"tunnox-core/internal/utils"
)

type ProtocolManager struct {
	utils.Dispose
	adapters []ProtocolAdapter
	lock     sync.Mutex
}

func NewProtocolManager(parentCtx context.Context) *ProtocolManager {
	pm := &ProtocolManager{}
	pm.SetCtx(parentCtx, pm.onClose)
	return pm
}

func (pm *ProtocolManager) Register(adapter ProtocolAdapter) {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.adapters = append(pm.adapters, adapter)
}

func (pm *ProtocolManager) StartAll(ctx context.Context) error {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	for _, a := range pm.adapters {
		if err := a.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (pm *ProtocolManager) CloseAll() {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	for _, a := range pm.adapters {
		a.Close()
	}
	pm.Dispose.Close()
}

func (pm *ProtocolManager) onClose() {
	pm.CloseAll()
}
