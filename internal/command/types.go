package command

import (
	"context"
	"time"
	"tunnox-core/internal/common"
	"tunnox-core/internal/packet"
)

// CommandCategory 命令分类
type CommandCategory int

const (
	CategoryConnection CommandCategory = iota // 连接管理类命令
	CategoryMapping                           // 端口映射类命令
	CategoryTransport                         // 数据传输类命令
	CategoryManagement                        // 系统管理类命令
	CategoryRPC                               // RPC调用类命令
)

func (c CommandCategory) String() string {
	switch c {
	case CategoryConnection:
		return "connection"
	case CategoryMapping:
		return "mapping"
	case CategoryTransport:
		return "transport"
	case CategoryManagement:
		return "management"
	case CategoryRPC:
		return "rpc"
	default:
		return "unknown"
	}
}

// CommandDirection 命令流向
type CommandDirection int

const (
	DirectionOneway CommandDirection = iota // 单向命令，不等待响应
	DirectionDuplex                         // 双工命令，需要等待响应
)

func (d CommandDirection) String() string {
	switch d {
	case DirectionOneway:
		return "oneway"
	case DirectionDuplex:
		return "duplex"
	default:
		return "unknown"
	}
}

// ResponseType 响应类型（保持向后兼容）
type ResponseType = CommandDirection

const (
	Oneway ResponseType = DirectionOneway
	Duplex ResponseType = DirectionDuplex
)

// CommandType 重新定义的命令类型
type CommandType struct {
	ID          packet.CommandType // 原始命令ID
	Category    CommandCategory    // 命令分类
	Direction   CommandDirection   // 命令流向
	Name        string             // 命令名称
	Description string             // 命令描述
}

// 预定义命令类型
var (
	// ==================== 连接管理类命令 ====================
	ConnectCmd = CommandType{
		ID:          packet.Connect,
		Category:    CategoryConnection,
		Direction:   DirectionDuplex,
		Name:        "connect",
		Description: "建立连接",
	}

	DisconnectCmd = CommandType{
		ID:          packet.Disconnect,
		Category:    CategoryConnection,
		Direction:   DirectionOneway,
		Name:        "disconnect",
		Description: "断开连接",
	}

	ReconnectCmd = CommandType{
		ID:          packet.Reconnect,
		Category:    CategoryConnection,
		Direction:   DirectionDuplex,
		Name:        "reconnect",
		Description: "重新连接",
	}

	HeartbeatCmd = CommandType{
		ID:          packet.HeartbeatCmd,
		Category:    CategoryConnection,
		Direction:   DirectionOneway,
		Name:        "heartbeat",
		Description: "心跳保活",
	}

	// ==================== 端口映射类命令 ====================
	TcpMapCreateCmd = CommandType{
		ID:          packet.TcpMapCreate,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "tcp_map_create",
		Description: "创建TCP端口映射",
	}

	TcpMapDeleteCmd = CommandType{
		ID:          packet.TcpMapDelete,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "tcp_map_delete",
		Description: "删除TCP端口映射",
	}

	TcpMapUpdateCmd = CommandType{
		ID:          packet.TcpMapUpdate,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "tcp_map_update",
		Description: "更新TCP端口映射",
	}

	TcpMapListCmd = CommandType{
		ID:          packet.TcpMapList,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "tcp_map_list",
		Description: "列出TCP端口映射",
	}

	TcpMapStatusCmd = CommandType{
		ID:          packet.TcpMapStatus,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "tcp_map_status",
		Description: "获取TCP端口映射状态",
	}

	HttpMapCreateCmd = CommandType{
		ID:          packet.HttpMapCreate,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "http_map_create",
		Description: "创建HTTP端口映射",
	}

	HttpMapDeleteCmd = CommandType{
		ID:          packet.HttpMapDelete,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "http_map_delete",
		Description: "删除HTTP端口映射",
	}

	HttpMapUpdateCmd = CommandType{
		ID:          packet.HttpMapUpdate,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "http_map_update",
		Description: "更新HTTP端口映射",
	}

	HttpMapListCmd = CommandType{
		ID:          packet.HttpMapList,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "http_map_list",
		Description: "列出HTTP端口映射",
	}

	HttpMapStatusCmd = CommandType{
		ID:          packet.HttpMapStatus,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "http_map_status",
		Description: "获取HTTP端口映射状态",
	}

	SocksMapCreateCmd = CommandType{
		ID:          packet.SocksMapCreate,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "socks_map_create",
		Description: "创建SOCKS代理映射",
	}

	SocksMapDeleteCmd = CommandType{
		ID:          packet.SocksMapDelete,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "socks_map_delete",
		Description: "删除SOCKS代理映射",
	}

	SocksMapUpdateCmd = CommandType{
		ID:          packet.SocksMapUpdate,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "socks_map_update",
		Description: "更新SOCKS代理映射",
	}

	SocksMapListCmd = CommandType{
		ID:          packet.SocksMapList,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "socks_map_list",
		Description: "列出SOCKS代理映射",
	}

	SocksMapStatusCmd = CommandType{
		ID:          packet.SocksMapStatus,
		Category:    CategoryMapping,
		Direction:   DirectionDuplex,
		Name:        "socks_map_status",
		Description: "获取SOCKS代理映射状态",
	}

	// ==================== 数据传输类命令 ====================
	DataTransferStartCmd = CommandType{
		ID:          packet.DataTransferStart,
		Category:    CategoryTransport,
		Direction:   DirectionDuplex,
		Name:        "data_transfer_start",
		Description: "开始数据传输",
	}

	DataTransferStopCmd = CommandType{
		ID:          packet.DataTransferStop,
		Category:    CategoryTransport,
		Direction:   DirectionOneway,
		Name:        "data_transfer_stop",
		Description: "停止数据传输",
	}

	DataTransferStatusCmd = CommandType{
		ID:          packet.DataTransferStatus,
		Category:    CategoryTransport,
		Direction:   DirectionDuplex,
		Name:        "data_transfer_status",
		Description: "获取数据传输状态",
	}

	ProxyForwardCmd = CommandType{
		ID:          packet.ProxyForward,
		Category:    CategoryTransport,
		Direction:   DirectionOneway,
		Name:        "proxy_forward",
		Description: "代理转发数据",
	}

	// ==================== 系统管理类命令 ====================
	ConfigGetCmd = CommandType{
		ID:          packet.ConfigGet,
		Category:    CategoryManagement,
		Direction:   DirectionDuplex,
		Name:        "config_get",
		Description: "获取配置信息",
	}

	ConfigSetCmd = CommandType{
		ID:          packet.ConfigSet,
		Category:    CategoryManagement,
		Direction:   DirectionDuplex,
		Name:        "config_set",
		Description: "设置配置信息",
	}

	StatsGetCmd = CommandType{
		ID:          packet.StatsGet,
		Category:    CategoryManagement,
		Direction:   DirectionDuplex,
		Name:        "stats_get",
		Description: "获取统计信息",
	}

	LogGetCmd = CommandType{
		ID:          packet.LogGet,
		Category:    CategoryManagement,
		Direction:   DirectionDuplex,
		Name:        "log_get",
		Description: "获取日志信息",
	}

	HealthCheckCmd = CommandType{
		ID:          packet.HealthCheck,
		Category:    CategoryManagement,
		Direction:   DirectionDuplex,
		Name:        "health_check",
		Description: "健康检查",
	}

	// ==================== RPC类命令 ====================
	RpcInvokeCmd = CommandType{
		ID:          packet.RpcInvoke,
		Category:    CategoryRPC,
		Direction:   DirectionDuplex,
		Name:        "rpc_invoke",
		Description: "RPC调用",
	}

	RpcRegisterCmd = CommandType{
		ID:          packet.RpcRegister,
		Category:    CategoryRPC,
		Direction:   DirectionDuplex,
		Name:        "rpc_register",
		Description: "注册RPC服务",
	}

	RpcUnregisterCmd = CommandType{
		ID:          packet.RpcUnregister,
		Category:    CategoryRPC,
		Direction:   DirectionDuplex,
		Name:        "rpc_unregister",
		Description: "注销RPC服务",
	}

	RpcListCmd = CommandType{
		ID:          packet.RpcList,
		Category:    CategoryRPC,
		Direction:   DirectionDuplex,
		Name:        "rpc_list",
		Description: "列出RPC服务",
	}
)

// CommandContext 命令上下文，包含所有必要信息
type CommandContext struct {
	ConnectionID string             // 连接ID
	CommandType  packet.CommandType // 命令类型
	CommandId    string             // 客户端生成的唯一命令ID
	RequestID    string             // 请求ID（Token）
	SenderID     string             // 发送者ID
	ReceiverID   string             // 接收者ID
	RequestBody  string             // JSON请求字符串
	Session      common.Session     // 会话对象
	Context      context.Context    // 上下文
	// 移除 Metadata map[string]interface{}，添加具体的字段
	IsAuthenticated bool      // 是否已认证
	UserID          string    // 用户ID
	StartTime       time.Time // 开始时间
	EndTime         time.Time // 结束时间
}

// CommandResponse 命令响应
type CommandResponse struct {
	Success   bool   `json:"success"`
	Data      string `json:"data,omitempty"` // JSON字符串，避免数据丢失
	Error     string `json:"error,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	CommandId string `json:"command_id,omitempty"` // 对应的命令ID
	// 移除 Metadata map[string]interface{}，添加具体的字段
	ProcessingTime time.Duration `json:"processing_time,omitempty"` // 处理时间
	HandlerName    string        `json:"handler_name,omitempty"`    // 处理器名称
}

// CommandHandler 命令处理器接口
type CommandHandler interface {
	// Handle 处理命令
	Handle(ctx *CommandContext) (*CommandResponse, error)

	// GetResponseType 获取响应类型
	GetResponseType() ResponseType

	// GetCommandType 获取命令类型
	GetCommandType() packet.CommandType

	// GetCategory 获取命令分类
	GetCategory() CommandCategory

	// GetDirection 获取命令流向
	GetDirection() CommandDirection
}

// Middleware 中间件接口
type Middleware interface {
	// Process 处理中间件逻辑
	Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error)
}

// MiddlewareFunc 中间件函数类型
type MiddlewareFunc func(*CommandContext, func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error)

// Process 实现Middleware接口
func (f MiddlewareFunc) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	return f(ctx, next)
}
