package httppoll

import (
	"time"
	"tunnox-core/internal/utils"
)

// idleCleanupLoop 定期清理长时间空闲后的资源状态
// 每 2 分钟检查一次，如果超过 10 分钟没有活动，则清理过期资源
func (sp *ServerStreamProcessor) idleCleanupLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	lastActivity := time.Now()
	lastCleanup := time.Now()

	for {
		select {
		case <-sp.Ctx().Done():
			utils.Debugf("ServerStreamProcessor[%s]: idleCleanupLoop exiting", sp.connectionID)
			return

		case <-ticker.C:
			// 更新最后活动时间：检查队列是否有数据
			queueLen := sp.pollDataQueue.Len()
			if queueLen > 0 {
				lastActivity = time.Now()
				utils.Debugf("ServerStreamProcessor[%s]: queue active, length=%d", sp.connectionID, queueLen)
				continue
			}

			// 检查是否长时间没有活动
			idleDuration := time.Since(lastActivity)
			if idleDuration > 10*time.Minute {
				// 避免频繁清理，至少间隔 5 分钟
				if time.Since(lastCleanup) < 5*time.Minute {
					continue
				}

				utils.Infof("ServerStreamProcessor[%s]: detected long idle (%.1f min), cleaning up stale state",
					sp.connectionID, idleDuration.Minutes())

				// 清理分片重组器中的过期数据
				removedGroups := sp.fragmentReassembler.CleanupStaleGroups(5 * time.Minute)
				if removedGroups > 0 {
					utils.Infof("ServerStreamProcessor[%s]: removed %d stale fragment groups",
						sp.connectionID, removedGroups)
				}

				lastCleanup = time.Now()

				// 重置最后活动时间，避免连续清理
				lastActivity = time.Now()
			}
		}
	}
}
