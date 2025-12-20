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

// setCachedResponse 缓存响应（带容量限制）
func (sp *StreamProcessor) setCachedResponse(requestID string, pkt *packet.TransferPacket) {
	sp.responseCacheMu.Lock()
	defer sp.responseCacheMu.Unlock()

	// 如果缓存已满，先清理过期项
	if len(sp.responseCache) >= responseCacheMaxSize {
		now := time.Now()
		for id, cached := range sp.responseCache {
			if now.After(cached.expiresAt) {
				delete(sp.responseCache, id)
			}
		}
	}

	// 如果清理后仍然满，删除最旧的项（FIFO策略）
	if len(sp.responseCache) >= responseCacheMaxSize {
		// 找到最旧的项（expiresAt 最早的）
		var oldestID string
		var oldestTime time.Time
		first := true
		for id, cached := range sp.responseCache {
			if first || cached.expiresAt.Before(oldestTime) {
				oldestID = id
				oldestTime = cached.expiresAt
				first = false
			}
		}
		if oldestID != "" {
			delete(sp.responseCache, oldestID)
		}
	}

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
