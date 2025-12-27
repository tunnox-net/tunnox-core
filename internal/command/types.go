package command

import (
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// 使用 core/types 包中定义的接口和类型
type CommandHandler = types.CommandHandler
type CommandContext = types.CommandContext
type CommandResponse = types.CommandResponse
type CommandCategory = types.CommandCategory
type CommandDirection = types.CommandDirection

// 移除 CommandResponseType = types.CommandResponseType
type Middleware = types.Middleware

// 导出常量
const (
	CategoryConnection   = types.CategoryConnection
	CategoryMapping      = types.CategoryMapping
	CategoryTransport    = types.CategoryTransport
	CategoryManagement   = types.CategoryManagement
	CategoryRPC          = types.CategoryRPC
	CategoryNotification = types.CategoryNotification

	DirectionOneway = types.DirectionOneway
	DirectionDuplex = types.DirectionDuplex
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
		Direction:   DirectionOneway,
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

	// ==================== 通知类命令 ====================
	NotifyClientCmd = CommandType{
		ID:          packet.NotifyClient,
		Category:    CategoryNotification,
		Direction:   DirectionOneway,
		Name:        "notify_client",
		Description: "服务端推送通知到客户端",
	}

	NotifyClientAckCmd = CommandType{
		ID:          packet.NotifyClientAck,
		Category:    CategoryNotification,
		Direction:   DirectionOneway,
		Name:        "notify_client_ack",
		Description: "客户端确认通知",
	}

	SendNotifyToClientCmd = CommandType{
		ID:          packet.SendNotifyToClient,
		Category:    CategoryNotification,
		Direction:   DirectionDuplex,
		Name:        "send_notify_to_client",
		Description: "C2C通知（客户端到客户端）",
	}
)

// MiddlewareFunc 中间件函数类型
type MiddlewareFunc func(*CommandContext, func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error)

func (f MiddlewareFunc) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	return f(ctx, next)
}
