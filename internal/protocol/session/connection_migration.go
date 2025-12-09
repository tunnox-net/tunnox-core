package session

import (
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/utils"
)

// migrateTunnelsOnReconnection 当客户端重连时，迁移现有 tunnels 到新连接
// 这是针对本地单节点客户端重连场景的优化，不同于跨节点迁移
//
// 场景：
// 1. Listen Client 的控制连接超时后重新连接，获得新的 connectionID
// 2. Tunnel (通过 TunnelOpen 建立) 仍然绑定到旧的 connectionID
// 3. 数据被推送到旧连接的 pollDataQueue，但新连接的 poll 请求读不到数据
//
// 解决方案：
// 1. 检测客户端重连（相同 clientID，不同 connectionID）
// 2. 遍历所有 tunnels，找到属于该 clientID 的 source tunnels
// 3. 迁移 pollDataQueue 中的数据到新连接
// 4. 更新 tunnel 的 source connection 引用
func (s *SessionManager) migrateTunnelsOnReconnection(clientID int64, oldConnID, newConnID string) {
	if oldConnID == newConnID {
		return // 没有变化，无需迁移
	}

	utils.Infof("SessionManager: Migrating tunnels for client %d from old connection %s to new connection %s",
		clientID, oldConnID, newConnID)

	// 获取旧连接和新连接
	oldConn := s.getConnectionByConnID(oldConnID)
	newConn := s.getConnectionByConnID(newConnID)

	if newConn == nil {
		utils.Errorf("SessionManager: Cannot migrate tunnels - new connection %s not found", newConnID)
		return
	}

	// 迁移前的验证和数据迁移
	var migratedDataCount int
	if oldConn != nil && oldConn.Stream != nil {
		migratedDataCount = s.migratePollDataQueue(oldConn, newConn)
	}

	// 遍历所有 tunnel bridges，更新绑定的连接
	s.bridgeLock.Lock()
	tunnelsMigrated := 0
	for tunnelID, bridge := range s.tunnelBridges {
		// 检查是否是该客户端的 source tunnel
		// 通过 GetSourceConnectionID 获取当前绑定的连接 ID
		sourceConnID := bridge.GetSourceConnectionID()
		if sourceConnID == oldConnID {
			utils.Infof("SessionManager: Migrating tunnel %s from connection %s to %s",
				tunnelID, oldConnID, newConnID)

			// 创建新的 TunnelConnection 接口实例
			newTunnelConn := CreateTunnelConnection(
				newConnID,
				newConn.RawConn,
				newConn.Stream,
				clientID,
				bridge.GetMappingID(),
				tunnelID,
			)

			// 更新 tunnel 的 source connection
			bridge.SetSourceConnection(newTunnelConn)
			tunnelsMigrated++

			utils.Infof("SessionManager: Tunnel %s migration completed, now using connection %s",
				tunnelID, newConnID)
		}
	}
	s.bridgeLock.Unlock()

	utils.Infof("SessionManager: Migration completed for client %d: %d tunnels migrated, %d data items transferred",
		clientID, tunnelsMigrated, migratedDataCount)
}

// PollDataQueueMigrator 定义可迁移的 poll data queue 接口
// 这个接口避免直接依赖 httppoll 包，防止循环导入
type PollDataQueueMigrator interface {
	// PopFromPollQueue 从 poll 队列中取出数据
	PopFromPollQueue() ([]byte, bool)
	// PushToPollQueue 推送数据到 poll 队列
	PushToPollQueue(data []byte)
	// NotifyPollDataAvailable 通知有新数据可用
	NotifyPollDataAvailable()
}

// migratePollDataQueue 迁移 pollDataQueue 中的数据
// 从旧连接的队列取出所有数据，推送到新连接的队列
// 返回迁移的数据项数量
func (s *SessionManager) migratePollDataQueue(oldConn, newConn *types.Connection) int {
	// 类型断言：检查是否实现了 PollDataQueueMigrator 接口
	oldMigrator, oldOk := oldConn.Stream.(PollDataQueueMigrator)
	newMigrator, newOk := newConn.Stream.(PollDataQueueMigrator)

	if !oldOk || !newOk {
		// 如果不是 HTTP Poll 连接，可能是通过 adapter 包装的
		// 尝试从 adapter 中提取
		oldMigrator = s.extractPollDataQueueMigrator(oldConn.Stream)
		newMigrator = s.extractPollDataQueueMigrator(newConn.Stream)

		if oldMigrator == nil || newMigrator == nil {
			utils.Infof("SessionManager: Connections do not support PollDataQueue migration, skipping")
			return 0
		}
	}

	// 迁移数据队列中的所有数据
	migratedCount := 0
	for {
		data, ok := oldMigrator.PopFromPollQueue()
		if !ok {
			break // 队列为空
		}

		// 推送到新连接的队列
		newMigrator.PushToPollQueue(data)
		migratedCount++
	}

	// 通知新连接有数据可读
	if migratedCount > 0 {
		newMigrator.NotifyPollDataAvailable()
		utils.Infof("SessionManager: Migrated %d data items from old pollDataQueue to new connection",
			migratedCount)
	}

	return migratedCount
}

// extractPollDataQueueMigrator 从 stream adapter 中提取 PollDataQueueMigrator
// 用于处理通过 adapter 包装的 HTTP Poll stream
func (s *SessionManager) extractPollDataQueueMigrator(stream interface{}) PollDataQueueMigrator {
	// 定义接口来访问内部的 stream processor
	type streamProcessorGetter interface {
		GetStreamProcessor() interface{}
	}

	if adapter, ok := stream.(streamProcessorGetter); ok {
		sp := adapter.GetStreamProcessor()
		if migrator, ok := sp.(PollDataQueueMigrator); ok {
			return migrator
		}
	}

	return nil
}
