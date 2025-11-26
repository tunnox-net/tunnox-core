package command

import (
	"encoding/json"
	"fmt"
	"reflect"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// BaseHandler 基础处理器
type BaseHandler struct {
	commandType packet.CommandType
	category    CommandCategory
	direction   CommandDirection
	name        string
	description string
}

// NewBaseHandler 创建基础处理器
func NewBaseHandler(cmdType packet.CommandType, category CommandCategory, direction CommandDirection, name, description string) *BaseHandler {
	return &BaseHandler{
		commandType: cmdType,
		category:    category,
		direction:   direction,
		name:        name,
		description: description,
	}
}

func (h *BaseHandler) GetCommandType() packet.CommandType { return h.commandType }
func (h *BaseHandler) GetCategory() CommandCategory       { return h.category }
func (h *BaseHandler) GetDirection() CommandDirection     { return h.direction }

// GetRequestType 获取请求类型（向后兼容，返回nil）
func (h *BaseHandler) GetRequestType() reflect.Type { return nil }

// GetResponseType 获取响应类型（向后兼容，返回nil）
func (h *BaseHandler) GetResponseType() reflect.Type { return nil }

// TcpMapHandler TCP映射处理器
type TcpMapHandler struct {
	*BaseHandler
}

func NewTcpMapHandler() *TcpMapHandler {
	return &TcpMapHandler{
		BaseHandler: NewBaseHandler(
			packet.TcpMapCreate,
			CategoryMapping,
			DirectionOneway,
			"tcp_map",
			"TCP端口映射",
		),
	}
}

func (h *TcpMapHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Infof("Handling TCP mapping command for connection: %s", ctx.ConnectionID)

	// TCP端口映射逻辑尚未实现
	// 1. 解析请求体中的端口映射配置
	// 2. 验证权限和配额
	// 3. 创建端口映射
	// 4. 返回映射结果

	data, _ := json.Marshal(map[string]interface{}{"message": "TCP mapping created"})
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// HttpMapHandler HTTP映射处理器
type HttpMapHandler struct {
	*BaseHandler
}

func NewHttpMapHandler() *HttpMapHandler {
	return &HttpMapHandler{
		BaseHandler: NewBaseHandler(
			packet.HttpMapCreate,
			CategoryMapping,
			DirectionOneway,
			"http_map",
			"HTTP端口映射",
		),
	}
}

func (h *HttpMapHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Infof("Handling HTTP mapping command for connection: %s", ctx.ConnectionID)

	// HTTP端口映射逻辑尚未实现

	data, _ := json.Marshal(map[string]interface{}{"message": "HTTP mapping created"})
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// SocksMapHandler SOCKS映射处理器
type SocksMapHandler struct {
	*BaseHandler
}

func NewSocksMapHandler() *SocksMapHandler {
	return &SocksMapHandler{
		BaseHandler: NewBaseHandler(
			packet.SocksMapCreate,
			CategoryMapping,
			DirectionOneway,
			"socks_map",
			"SOCKS代理映射",
		),
	}
}

func (h *SocksMapHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Infof("Handling SOCKS mapping command for connection: %s", ctx.ConnectionID)

	// SOCKS代理映射逻辑尚未实现

	data, _ := json.Marshal(map[string]interface{}{"message": "SOCKS mapping created"})
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// DataInHandler 数据输入处理器
type DataInHandler struct {
	*BaseHandler
}

func NewDataInHandler() *DataInHandler {
	return &DataInHandler{
		BaseHandler: NewBaseHandler(
			packet.DataTransferStart,
			CategoryTransport,
			DirectionOneway,
			"data_in",
			"数据输入通知",
		),
	}
}

func (h *DataInHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Infof("Handling DataIn command for connection: %s", ctx.ConnectionID)

	// 数据输入处理逻辑尚未实现
	// 1. 解析数据输入请求
	// 2. 准备数据传输通道
	// 3. 通知相关组件

	data, _ := json.Marshal(map[string]interface{}{"message": "Data input ready"})
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// DataOutHandler 数据输出处理器
type DataOutHandler struct {
	*BaseHandler
}

func NewDataOutHandler() *DataOutHandler {
	return &DataOutHandler{
		BaseHandler: NewBaseHandler(
			packet.DataTransferOut,
			CategoryTransport,
			DirectionOneway,
			"data_out",
			"数据输出通知",
		),
	}
}

func (h *DataOutHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Infof("Handling DataOut command for connection: %s", ctx.ConnectionID)

	// 数据输出处理逻辑尚未实现

	data, _ := json.Marshal(map[string]interface{}{"message": "Data output ready"})
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// ForwardHandler 转发处理器
type ForwardHandler struct {
	*BaseHandler
}

func NewForwardHandler() *ForwardHandler {
	return &ForwardHandler{
		BaseHandler: NewBaseHandler(
			packet.ProxyForward,
			CategoryTransport,
			DirectionOneway,
			"forward",
			"服务端间转发",
		),
	}
}

func (h *ForwardHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Infof("Handling Forward command for connection: %s", ctx.ConnectionID)

	// 服务端间转发逻辑尚未实现

	data, _ := json.Marshal(map[string]interface{}{"message": "Forward setup complete"})
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// DisconnectHandler 断开连接处理器
type DisconnectHandler struct {
	*BaseHandler
}

func NewDisconnectHandler() *DisconnectHandler {
	return &DisconnectHandler{
		BaseHandler: NewBaseHandler(
			packet.Disconnect,
			CategoryConnection,
			DirectionOneway,
			"disconnect",
			"连接断开",
		),
	}
}

func (h *DisconnectHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Infof("Handling Disconnect command for connection: %s", ctx.ConnectionID)

	// 在事件驱动架构中，连接关闭应该通过事件总线处理
	// Session会监听命令完成事件，然后执行实际的连接关闭操作
	// 这里我们只返回成功响应，实际的关闭操作由Session处理

	data, _ := json.Marshal(map[string]interface{}{"message": "Connection disconnect command received"})
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// RpcInvokeHandler RPC调用处理器
type RpcInvokeHandler struct {
	*BaseHandler
}

func NewRpcInvokeHandler() *RpcInvokeHandler {
	return &RpcInvokeHandler{
		BaseHandler: NewBaseHandler(
			packet.RpcInvoke,
			CategoryRPC,
			DirectionDuplex,
			"rpc_invoke",
			"RPC调用",
		),
	}
}

func (h *RpcInvokeHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Infof("Handling RPC invoke command for connection: %s", ctx.ConnectionID)

	// RPC调用逻辑尚未实现
	// 1. 解析RPC请求
	// 2. 查找对应的RPC服务
	// 3. 执行RPC调用
	// 4. 返回结果

	var rpcRequest map[string]interface{}
	if err := json.Unmarshal([]byte(ctx.RequestBody), &rpcRequest); err != nil {
		return &CommandResponse{
			Success:   false,
			Error:     fmt.Sprintf("Invalid RPC request: %v", err),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 模拟RPC调用结果
	result := map[string]interface{}{
		"method": rpcRequest["method"],
		"result": "RPC call successful",
	}

	data, _ := json.Marshal(result)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// DefaultHandler 默认处理器
type DefaultHandler struct {
	*BaseHandler
}

func NewDefaultHandler() *DefaultHandler {
	return &DefaultHandler{
		BaseHandler: NewBaseHandler(
			0, // 未知命令类型
			CategoryManagement,
			DirectionOneway,
			"unknown",
			"未知命令",
		),
	}
}

func (h *DefaultHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	utils.Warnf("Unknown command type for connection %s: %v", ctx.ConnectionID, ctx.CommandType)

	return &CommandResponse{
		Success:   false,
		Error:     fmt.Sprintf("Unknown command type: %v", ctx.CommandType),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}
