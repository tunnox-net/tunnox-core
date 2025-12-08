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

// ISessionManagerWithConnection 扩展的 ISessionManager 接口
// 遵循编码规范：接口使用 I 前缀
type ISessionManagerWithConnection interface {
	ISessionManager
	CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
	GetConnection(connID string) (*types.Connection, error)
}

// getSessionManagerWithConnection 获取支持 CreateConnection 的 ISessionManager
func getSessionManagerWithConnection(sm ISessionManager) ISessionManagerWithConnection {
	// 尝试直接类型断言
	if smc, ok := sm.(ISessionManagerWithConnection); ok {
		return smc
	}
	// 尝试通过接口组合获取
	type createConn interface {
		CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
		GetConnection(connID string) (*types.Connection, error)
	}
	if cc, ok := sm.(createConn); ok {
		return &sessionManagerAdapter{
			ISessionManager: sm,
			createConn:      cc,
		}
	}
	return nil
}

// sessionManagerAdapter 适配器
type sessionManagerAdapter struct {
	ISessionManager
	createConn interface {
		CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
		GetConnection(connID string) (*types.Connection, error)
	}
}

func (a *sessionManagerAdapter) CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error) {
	return a.createConn.CreateConnection(reader, writer)
}

func (a *sessionManagerAdapter) GetConnection(connID string) (*types.Connection, error) {
	return a.createConn.GetConnection(connID)
}
