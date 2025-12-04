package httppoll

import (
	"time"

	"tunnox-core/internal/packet"
)

// cachedResponse 缓存的响应
type cachedResponse struct {
	pkt       *packet.TransferPacket
	expiresAt time.Time
}

// cleanupExpiredResponses 清理过期的响应缓存
func (sp *StreamProcessor) cleanupExpiredResponses() {
	now := time.Now()
	sp.responseCacheMu.Lock()
	defer sp.responseCacheMu.Unlock()

	for requestID, cached := range sp.responseCache {
		if now.After(cached.expiresAt) {
			delete(sp.responseCache, requestID)
		}
	}
}

// getCachedResponse 从缓存中获取响应
func (sp *StreamProcessor) getCachedResponse(requestID string) (*packet.TransferPacket, bool) {
	sp.responseCacheMu.RLock()
	defer sp.responseCacheMu.RUnlock()

	cached, exists := sp.responseCache[requestID]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(cached.expiresAt) {
		return nil, false
	}

	return cached.pkt, true
}

// setCachedResponse 缓存响应
func (sp *StreamProcessor) setCachedResponse(requestID string, pkt *packet.TransferPacket) {
	sp.responseCacheMu.Lock()
	defer sp.responseCacheMu.Unlock()

	sp.responseCache[requestID] = &cachedResponse{
		pkt:       pkt,
		expiresAt: time.Now().Add(responseCacheTTL),
	}
}

// removeCachedResponse 从缓存中移除响应
func (sp *StreamProcessor) removeCachedResponse(requestID string) {
	sp.responseCacheMu.Lock()
	defer sp.responseCacheMu.Unlock()

	delete(sp.responseCache, requestID)
}

