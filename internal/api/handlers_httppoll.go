package api

import (
	"io"

	"tunnox-core/internal/core/types"
	httppoll "tunnox-core/internal/protocol/httppoll"
)

const (
	httppollMaxRequestSize = 1024 * 1024 // 1MB
	httppollDefaultTimeout = 30          // 默认 30 秒
	httppollMaxTimeout     = 60          // 最大 60 秒
)

// HTTPPushRequest HTTP 推送请求（统一使用 FragmentResponse）
type HTTPPushRequest = httppoll.FragmentResponse

// HTTPPushResponse HTTP 推送响应
type HTTPPushResponse struct {
	Success   bool  `json:"success"`
	Timestamp int64 `json:"timestamp"`
}

// HTTPPollResponse HTTP 轮询响应（统一使用 FragmentResponse）
type HTTPPollResponse = httppoll.FragmentResponse

// SessionManagerWithConnection 扩展的 SessionManager 接口
type SessionManagerWithConnection interface {
	SessionManager
	CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
	GetConnection(connID string) (*types.Connection, bool)
}

// getSessionManagerWithConnection 获取支持 CreateConnection 的 SessionManager
func getSessionManagerWithConnection(sm SessionManager) SessionManagerWithConnection {
	// 尝试直接类型断言
	if smc, ok := sm.(SessionManagerWithConnection); ok {
		return smc
	}
	// 尝试通过接口组合获取
	type createConn interface {
		CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
		GetConnection(connID string) (*types.Connection, bool)
	}
	if cc, ok := sm.(createConn); ok {
		return &sessionManagerAdapter{
			SessionManager: sm,
			createConn:     cc,
		}
	}
	return nil
}

// sessionManagerAdapter 适配器
type sessionManagerAdapter struct {
	SessionManager
	createConn interface {
		CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
		GetConnection(connID string) (*types.Connection, bool)
	}
}

func (a *sessionManagerAdapter) CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error) {
	return a.createConn.CreateConnection(reader, writer)
}

func (a *sessionManagerAdapter) GetConnection(connID string) (*types.Connection, bool) {
	return a.createConn.GetConnection(connID)
}
