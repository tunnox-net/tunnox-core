package api

import (
	"context"
	"tunnox-core/internal/cloud/models"
)

// CloudControlAPI 云控制API接口
type CloudControlAPI interface {
	// 用户管理
	CreateUser(ctx context.Context, user *models.User) error
	GetUser(ctx context.Context, userID string) (*models.User, error)
	UpdateUser(ctx context.Context, user *models.User) error
	DeleteUser(ctx context.Context, userID string) error
	ListUsers(ctx context.Context) ([]*models.User, error)

	// 客户端管理
	CreateClient(ctx context.Context, client *models.Client) error
	GetClient(ctx context.Context, clientID int64) (*models.Client, error)
	UpdateClient(ctx context.Context, client *models.Client) error
	DeleteClient(ctx context.Context, clientID int64) error
	ListClients(ctx context.Context) ([]*models.Client, error)

	// 端口映射管理
	CreatePortMapping(ctx context.Context, mapping *models.PortMapping) error
	GetPortMapping(ctx context.Context, mappingID string) (*models.PortMapping, error)
	UpdatePortMapping(ctx context.Context, mapping *models.PortMapping) error
	DeletePortMapping(ctx context.Context, mappingID string) error
	ListPortMappings(ctx context.Context) ([]*models.PortMapping, error)

	// 节点管理
	RegisterNode(ctx context.Context, node *models.Node) error
	GetNode(ctx context.Context, nodeID string) (*models.Node, error)
	UpdateNode(ctx context.Context, node *models.Node) error
	UnregisterNode(ctx context.Context, nodeID string) error
	ListNodes(ctx context.Context) ([]*models.Node, error)

	// 认证管理
	AuthenticateClient(ctx context.Context, req *models.AuthRequest) (*models.AuthResponse, error)
	ValidateToken(ctx context.Context, token string) (bool, error)
	RevokeToken(ctx context.Context, token string) error

	// 连接管理
	RegisterConnection(ctx context.Context, conn *models.ConnectionInfo) error
	GetConnection(ctx context.Context, connID string) (*models.ConnectionInfo, error)
	UpdateConnection(ctx context.Context, conn *models.ConnectionInfo) error
	UnregisterConnection(ctx context.Context, connID string) error
	ListConnections(ctx context.Context) ([]*models.ConnectionInfo, error)

	// 统计信息
	GetSystemStats(ctx context.Context) (interface{}, error)
	GetClientStats(ctx context.Context, clientID int64) (interface{}, error)
	GetNodeStats(ctx context.Context, nodeID string) (interface{}, error)

	// 资源清理
	Close() error
}

// APIError API错误类型
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}

// NewAPIError 创建新的API错误
func NewAPIError(code int, message string) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
	}
}

// NewAPIErrorWithDetails 创建带详细信息的API错误
func NewAPIErrorWithDetails(code int, message, details string) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
		Details: details,
	}
}
