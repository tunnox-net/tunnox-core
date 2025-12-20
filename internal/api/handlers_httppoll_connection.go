package api

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"strings"

	httppoll "tunnox-core/internal/protocol/httppoll"
)

// 注意：此函数不是线程安全的，调用者需要确保在锁保护下调用，或者使用 ConnectionRegistry 的 GetOrCreate 模式
func (s *ManagementAPIServer) createHTTPLongPollingConnection(connID string, pkg *httppoll.TunnelPackage, ctx context.Context) *httppoll.ServerStreamProcessor {
	// 注意：不在这里检查 ConnectionRegistry，因为调用者已经检查过了
	// 如果调用者需要检查，应该在调用前使用 ConnectionRegistry.Get()

	// 1. 获取 SessionManager
	if s.sessionMgr == nil {
		corelog.Errorf("HTTP long polling: SessionManager not available")
		return nil
	}

	// 2. 确定连接类型
	connType := pkg.TunnelType
	if connType == "" {
		// 根据包类型推断
		if pkg.Type == "TunnelOpen" {
			connType = "data"
		} else {
			connType = "control"
		}
	}

	// 3. 使用 server 的 context 而不是请求的 context，避免请求结束后 context 被取消
	serverCtx := s.Ctx()
	if serverCtx == nil {
		serverCtx = context.Background()
	}

	clientID := pkg.ClientID
	if clientID == 0 {
		// 握手阶段，clientID 为 0
		clientID = 0
	}

	// 4. 创建 HTTP 长轮询流处理器（使用新的 ServerStreamProcessor）
	streamProcessor := httppoll.NewServerStreamProcessor(serverCtx, connID, clientID, pkg.MappingID)

	// 5. 在 SessionManager 中注册连接（用于握手等流程）
	// 先检查连接是否已存在，避免重复创建
	sessionMgrWithConn := getSessionManagerWithConnection(s.sessionMgr)
	if sessionMgrWithConn != nil {
		existingConn, exists := sessionMgrWithConn.GetConnection(connID)
		if exists && existingConn != nil {
			corelog.Debugf("HTTP long polling: connection already exists in SessionManager, connID=%s", connID)
		} else {
			// 创建适配器，让 ServerStreamProcessor 可以作为 reader/writer 传递给 CreateConnection
			// StreamManager.CreateStream 会检测到适配器中的 PackageStreamer 并直接使用
			adapter := &httppollStreamAdapter{streamProcessor: streamProcessor}
			_, err := sessionMgrWithConn.CreateConnection(adapter, adapter)
			if err != nil {
				// 如果错误是连接已存在，忽略（可能是并发创建导致的）
				if !strings.Contains(err.Error(), "already exists") {
					corelog.Errorf("HTTP long polling: failed to create connection in SessionManager: %v", err)
				} else {
					corelog.Debugf("HTTP long polling: connection already exists in SessionManager (concurrent creation), connID=%s", connID)
				}
				// 即使注册失败，也返回 streamProcessor，因为连接管理主要通过 ConnectionRegistry
			} else {
				corelog.Infof("HTTP long polling: registered connection in SessionManager, connID=%s", connID)
			}
		}
	}

	corelog.Infof("HTTP long polling: created stream processor connID=%s for clientID=%d, mappingID=%s", connID, clientID, pkg.MappingID)

	return streamProcessor
}

