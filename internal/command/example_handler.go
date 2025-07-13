package command

import (
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// BaseHandler 基础命令处理器
type BaseHandler struct {
	commandType  packet.CommandType
	responseType ResponseType
}

// NewBaseHandler 创建基础处理器
func NewBaseHandler(commandType packet.CommandType, responseType ResponseType) *BaseHandler {
	return &BaseHandler{
		commandType:  commandType,
		responseType: responseType,
	}
}

// GetCommandType 获取命令类型
func (bh *BaseHandler) GetCommandType() packet.CommandType {
	return bh.commandType
}

// GetResponseType 获取响应类型
func (bh *BaseHandler) GetResponseType() ResponseType {
	return bh.responseType
}

// TcpMapHandler TCP映射命令处理器
type TcpMapHandler struct {
	*BaseHandler
}

// NewTcpMapHandler 创建TCP映射处理器
func NewTcpMapHandler() *TcpMapHandler {
	return &TcpMapHandler{
		BaseHandler: NewBaseHandler(packet.TcpMap, Duplex),
	}
}

// Handle 处理TCP映射命令
func (h *TcpMapHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	// 解析请求数据
	var requestData map[string]interface{}
	if err := json.Unmarshal([]byte(ctx.RequestBody), &requestData); err != nil {
		return &CommandResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to parse request: %v", err),
		}, nil
	}

	// 记录处理信息
	utils.Debugf("Processing TCP mapping for connection: %s, request: %+v",
		ctx.ConnectionID, requestData)

	// TODO: 实现具体的TCP映射逻辑
	// 这里只是示例，实际实现需要根据业务需求

	// 返回成功响应
	return &CommandResponse{
		Success: true,
		Data: map[string]interface{}{
			"mapping_id": fmt.Sprintf("tcp_%s_%d", ctx.ConnectionID, time.Now().Unix()),
			"status":     "created",
			"request":    requestData,
		},
		Metadata: map[string]interface{}{
			"connection_id": ctx.ConnectionID,
			"command_type":  ctx.CommandType,
		},
	}, nil
}

// HttpMapHandler HTTP映射命令处理器
type HttpMapHandler struct {
	*BaseHandler
}

// NewHttpMapHandler 创建HTTP映射处理器
func NewHttpMapHandler() *HttpMapHandler {
	return &HttpMapHandler{
		BaseHandler: NewBaseHandler(packet.HttpMap, Duplex),
	}
}

// Handle 处理HTTP映射命令
func (h *HttpMapHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	// 解析请求数据
	var requestData map[string]interface{}
	if err := json.Unmarshal([]byte(ctx.RequestBody), &requestData); err != nil {
		return &CommandResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to parse request: %v", err),
		}, nil
	}

	// 记录处理信息
	utils.Debugf("Processing HTTP mapping for connection: %s, request: %+v",
		ctx.ConnectionID, requestData)

	// TODO: 实现具体的HTTP映射逻辑

	// 返回成功响应
	return &CommandResponse{
		Success: true,
		Data: map[string]interface{}{
			"mapping_id": fmt.Sprintf("http_%s_%d", ctx.ConnectionID, time.Now().Unix()),
			"status":     "created",
			"request":    requestData,
		},
		Metadata: map[string]interface{}{
			"connection_id": ctx.ConnectionID,
			"command_type":  ctx.CommandType,
		},
	}, nil
}

// DisconnectHandler 断开连接命令处理器
type DisconnectHandler struct {
	*BaseHandler
}

// NewDisconnectHandler 创建断开连接处理器
func NewDisconnectHandler() *DisconnectHandler {
	return &DisconnectHandler{
		BaseHandler: NewBaseHandler(packet.Disconnect, Oneway),
	}
}

// Handle 处理断开连接命令
func (h *DisconnectHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	// 解析请求数据
	var requestData map[string]interface{}
	if err := json.Unmarshal([]byte(ctx.RequestBody), &requestData); err != nil {
		utils.Warnf("Failed to parse disconnect request: %v", err)
		// 单向命令，即使解析失败也继续处理
	}

	// 记录处理信息
	utils.Infof("Processing disconnect for connection: %s, reason: %v",
		ctx.ConnectionID, requestData)

	// TODO: 实现具体的断开连接逻辑
	// 可能需要通知其他组件关闭相关资源

	// 单向命令不需要返回响应
	return nil, nil
}

// DataInHandler 数据输入命令处理器
type DataInHandler struct {
	*BaseHandler
}

// NewDataInHandler 创建数据输入处理器
func NewDataInHandler() *DataInHandler {
	return &DataInHandler{
		BaseHandler: NewBaseHandler(packet.DataIn, Oneway),
	}
}

// Handle 处理数据输入命令
func (h *DataInHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	// 解析请求数据
	var requestData map[string]interface{}
	if err := json.Unmarshal([]byte(ctx.RequestBody), &requestData); err != nil {
		utils.Warnf("Failed to parse data in request: %v", err)
	}

	// 记录处理信息
	utils.Debugf("Processing data in for connection: %s, data: %+v",
		ctx.ConnectionID, requestData)

	// TODO: 实现具体的数据输入处理逻辑

	// 单向命令不需要返回响应
	return nil, nil
}

// ForwardHandler 转发命令处理器
type ForwardHandler struct {
	*BaseHandler
}

// NewForwardHandler 创建转发处理器
func NewForwardHandler() *ForwardHandler {
	return &ForwardHandler{
		BaseHandler: NewBaseHandler(packet.Forward, Duplex),
	}
}

// Handle 处理转发命令
func (h *ForwardHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	// 解析请求数据
	var requestData map[string]interface{}
	if err := json.Unmarshal([]byte(ctx.RequestBody), &requestData); err != nil {
		return &CommandResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to parse request: %v", err),
		}, nil
	}

	// 记录处理信息
	utils.Debugf("Processing forward for connection: %s, request: %+v",
		ctx.ConnectionID, requestData)

	// TODO: 实现具体的转发逻辑

	// 返回成功响应
	return &CommandResponse{
		Success: true,
		Data: map[string]interface{}{
			"forward_id": fmt.Sprintf("forward_%s_%d", ctx.ConnectionID, time.Now().Unix()),
			"status":     "ready",
			"request":    requestData,
		},
		Metadata: map[string]interface{}{
			"connection_id": ctx.ConnectionID,
			"command_type":  ctx.CommandType,
		},
	}, nil
}

// DataOutHandler 数据输出命令处理器
type DataOutHandler struct {
	*BaseHandler
}

// NewDataOutHandler 创建数据输出处理器
func NewDataOutHandler() *DataOutHandler {
	return &DataOutHandler{
		BaseHandler: NewBaseHandler(packet.DataOut, Oneway),
	}
}

// Handle 处理数据输出命令
func (h *DataOutHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	// 解析请求数据
	var requestData map[string]interface{}
	if err := json.Unmarshal([]byte(ctx.RequestBody), &requestData); err != nil {
		utils.Warnf("Failed to parse data out request: %v", err)
	}

	// 记录处理信息
	utils.Debugf("Processing data out for connection: %s, data: %+v",
		ctx.ConnectionID, requestData)

	// TODO: 实现具体的数据输出处理逻辑

	// 单向命令不需要返回响应
	return nil, nil
}

// SocksMapHandler SOCKS映射命令处理器
type SocksMapHandler struct {
	*BaseHandler
}

// NewSocksMapHandler 创建SOCKS映射处理器
func NewSocksMapHandler() *SocksMapHandler {
	return &SocksMapHandler{
		BaseHandler: NewBaseHandler(packet.SocksMap, Duplex),
	}
}

// Handle 处理SOCKS映射命令
func (h *SocksMapHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	// 解析请求数据
	var requestData map[string]interface{}
	if err := json.Unmarshal([]byte(ctx.RequestBody), &requestData); err != nil {
		return &CommandResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to parse request: %v", err),
		}, nil
	}

	// 记录处理信息
	utils.Debugf("Processing SOCKS mapping for connection: %s, request: %+v",
		ctx.ConnectionID, requestData)

	// TODO: 实现具体的SOCKS映射逻辑

	// 返回成功响应
	return &CommandResponse{
		Success: true,
		Data: map[string]interface{}{
			"mapping_id": fmt.Sprintf("socks_%s_%d", ctx.ConnectionID, time.Now().Unix()),
			"status":     "created",
			"request":    requestData,
		},
		Metadata: map[string]interface{}{
			"connection_id": ctx.ConnectionID,
			"command_type":  ctx.CommandType,
		},
	}, nil
}
