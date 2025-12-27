package command

import (
	"encoding/json"
	"fmt"
	"reflect"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// 示例请求和响应结构体
type ConnectRequest struct {
	ClientID   int64  `json:"client_id"`
	ClientName string `json:"client_name"`
	Protocol   string `json:"protocol"`
}

type ConnectResponse struct {
	Success    bool   `json:"success"`
	SessionID  string `json:"session_id"`
	ServerTime int64  `json:"server_time"`
}

type HeartbeatRequest struct {
	ClientID  int64 `json:"client_id"`
	Timestamp int64 `json:"timestamp"`
}

// 无响应体的命令（单向命令）
type DisconnectRequest struct {
	ClientID int64  `json:"client_id"`
	Reason   string `json:"reason"`
}

// ConnectHandler 连接命令处理器示例
type ConnectHandler struct {
	*BaseCommandHandler[ConnectRequest, ConnectResponse]
}

// NewConnectHandler 创建连接命令处理器
func NewConnectHandler() *ConnectHandler {
	base := NewBaseCommandHandler[ConnectRequest, ConnectResponse](
		packet.Connect,
		DirectionDuplex,
		DuplexMode,
	)

	return &ConnectHandler{
		BaseCommandHandler: base,
	}
}

// Handle 实现CommandHandler接口
func (h *ConnectHandler) Handle(ctx *types.CommandContext) (*types.CommandResponse, error) {
	// 解析请求
	request, err := h.ParseRequest(ctx)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 验证请求
	if err := h.ValidateRequest(request); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 预处理
	if err := h.PreProcess(ctx, request); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 处理请求
	response, err := h.ProcessRequest(ctx, request)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 后处理
	if err := h.PostProcess(ctx, response); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	return h.CreateSuccessResponse(response, ctx.RequestID), nil
}

// ProcessRequest 处理连接请求
func (h *ConnectHandler) ProcessRequest(ctx *types.CommandContext, request *ConnectRequest) (*ConnectResponse, error) {
	// 模拟连接处理逻辑
	response := &ConnectResponse{
		Success:    true,
		SessionID:  fmt.Sprintf("session_%d_%d", request.ClientID, ctx.StartTime.Unix()),
		ServerTime: ctx.StartTime.Unix(),
	}

	return response, nil
}

// ValidateRequest 验证连接请求
func (h *ConnectHandler) ValidateRequest(request *ConnectRequest) error {
	if request.ClientID <= 0 {
		return fmt.Errorf("invalid client ID: %d", request.ClientID)
	}
	if request.ClientName == "" {
		return fmt.Errorf("client name is required")
	}
	return nil
}

// HeartbeatHandler 心跳命令处理器示例（单向命令）
type HeartbeatHandler struct {
	*BaseCommandHandler[HeartbeatRequest, interface{}]
}

// NewHeartbeatHandler 创建心跳命令处理器
func NewHeartbeatHandler() *HeartbeatHandler {
	base := NewBaseCommandHandler[HeartbeatRequest, interface{}](
		packet.HeartbeatCmd,
		DirectionOneway,
		Simplex,
	)

	return &HeartbeatHandler{
		BaseCommandHandler: base,
	}
}

// Handle 实现CommandHandler接口
func (h *HeartbeatHandler) Handle(ctx *types.CommandContext) (*types.CommandResponse, error) {
	// 解析请求
	request, err := h.ParseRequest(ctx)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 处理请求（心跳不需要响应）
	_, err = h.ProcessRequest(ctx, request)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 单向命令返回空响应
	return h.CreateSuccessResponse(nil, ctx.RequestID), nil
}

// ProcessRequest 处理心跳请求
func (h *HeartbeatHandler) ProcessRequest(ctx *types.CommandContext, request *HeartbeatRequest) (interface{}, error) {
	// 更新客户端最后心跳时间
	// 这里可以更新会话中的客户端状态
	return nil, nil
}

// DisconnectHandlerV2 断开连接命令处理器示例（无响应体）- 使用新的泛型设计
type DisconnectHandlerV2 struct {
	*BaseCommandHandler[DisconnectRequest, interface{}]
}

// NewDisconnectHandlerV2 创建断开连接命令处理器
func NewDisconnectHandlerV2() *DisconnectHandlerV2 {
	base := NewBaseCommandHandler[DisconnectRequest, interface{}](
		packet.Disconnect,
		DirectionOneway,
		Simplex,
	)

	return &DisconnectHandlerV2{
		BaseCommandHandler: base,
	}
}

// Handle 实现CommandHandler接口
func (h *DisconnectHandlerV2) Handle(ctx *types.CommandContext) (*types.CommandResponse, error) {
	// 解析请求
	request, err := h.ParseRequest(ctx)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 处理请求
	_, err = h.ProcessRequest(ctx, request)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 单向命令返回空响应
	return h.CreateSuccessResponse(nil, ctx.RequestID), nil
}

// ProcessRequest 处理断开连接请求
func (h *DisconnectHandlerV2) ProcessRequest(ctx *types.CommandContext, request *DisconnectRequest) (interface{}, error) {
	// 关闭连接逻辑
	if err := h.GetSession().CloseConnection(ctx.ConnectionID); err != nil {
		return nil, fmt.Errorf("failed to close connection: %w", err)
	}
	return nil, nil
}

// 类型信息工具函数
func GetHandlerTypeInfo(handler types.CommandHandler) {
	fmt.Printf("Handler Type Info:\n")
	fmt.Printf("  Command Type: %v\n", handler.GetCommandType())
	fmt.Printf("  Direction: %v\n", handler.GetDirection())
	fmt.Printf("  Category: %v\n", handler.GetCategory())

	requestType := handler.GetRequestType()
	if requestType != nil {
		fmt.Printf("  Request Type: %v\n", requestType)
	} else {
		fmt.Printf("  Request Type: nil (no request body)\n")
	}

	responseType := handler.GetResponseType()
	if responseType != nil {
		fmt.Printf("  Response Type: %v\n", responseType)
	} else {
		fmt.Printf("  Response Type: nil (no response body)\n")
	}
}

// 统一的命令体处理函数 - 使用反射进行类型安全的处理
func ProcessCommandBody(handler types.CommandHandler, requestBody string) (interface{}, error) {
	requestType := handler.GetRequestType()
	if requestType == nil {
		// 无请求体的命令
		return nil, nil
	}

	// 使用反射创建请求实例
	requestValue := reflect.New(requestType)
	request := requestValue.Interface()

	// 解析JSON到请求实例
	if err := json.Unmarshal([]byte(requestBody), request); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	return request, nil
}

// 类型安全的响应创建函数
func CreateTypedResponse(handler types.CommandHandler, data interface{}) (*types.CommandResponse, error) {
	responseType := handler.GetResponseType()
	if responseType == nil {
		// 无响应体的命令
		return &types.CommandResponse{
			Success: true,
		}, nil
	}

	// 验证数据类型
	if data != nil && reflect.TypeOf(data) != responseType {
		return nil, fmt.Errorf("response data type mismatch: expected %v, got %T", responseType, data)
	}

	// 序列化响应数据
	var responseData string
	if data != nil {
		if jsonData, err := json.Marshal(data); err == nil {
			responseData = string(jsonData)
		} else {
			return nil, fmt.Errorf("failed to marshal response data: %w", err)
		}
	}

	return &types.CommandResponse{
		Success: true,
		Data:    responseData,
	}, nil
}

// 示例：如何使用类型信息进行统一的命令处理
func Example_usage() {
	// 创建不同类型的处理器
	connectHandler := NewConnectHandler()
	heartbeatHandler := NewHeartbeatHandler()
	disconnectHandler := NewDisconnectHandlerV2()

	// 显示类型信息
	fmt.Println("=== Connect Handler ===")
	GetHandlerTypeInfo(connectHandler)

	fmt.Println("\n=== Heartbeat Handler ===")
	GetHandlerTypeInfo(heartbeatHandler)

	fmt.Println("\n=== Disconnect Handler ===")
	GetHandlerTypeInfo(disconnectHandler)

	// 示例：处理不同类型的请求
	connectRequest := `{"client_id": 12345, "client_name": "test_client", "protocol": "tcp"}`
	heartbeatRequest := `{"client_id": 12345, "timestamp": 1640995200}`
	disconnectRequest := `{"client_id": 12345, "reason": "user_disconnect"}`

	// 处理连接请求
	if data, err := ProcessCommandBody(connectHandler, connectRequest); err == nil {
		fmt.Printf("\nConnect request parsed: %+v\n", data)
	} else {
		fmt.Printf("Failed to parse connect request: %v\n", err)
	}

	// 处理心跳请求
	if data, err := ProcessCommandBody(heartbeatHandler, heartbeatRequest); err == nil {
		fmt.Printf("Heartbeat request parsed: %+v\n", data)
	} else {
		fmt.Printf("Failed to parse heartbeat request: %v\n", err)
	}

	// 处理断开连接请求
	if data, err := ProcessCommandBody(disconnectHandler, disconnectRequest); err == nil {
		fmt.Printf("Disconnect request parsed: %+v\n", data)
	} else {
		fmt.Printf("Failed to parse disconnect request: %v\n", err)
	}
}
