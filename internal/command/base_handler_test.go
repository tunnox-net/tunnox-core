package command

import (
	"encoding/json"
	"fmt"
	"testing"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequest 测试请求结构
type TestRequest struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Email   string `json:"email"`
	Enabled bool   `json:"enabled"`
}

// TestResponse 测试响应结构
type TestResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// TestHandler 测试处理器
type TestHandler struct {
	*BaseCommandHandler[TestRequest, TestResponse]
}

// NewTestHandler 创建测试处理器
func NewTestHandler() *TestHandler {
	base := NewBaseCommandHandler[TestRequest, TestResponse](
		packet.TcpMapCreate,
		DirectionDuplex, // 替换 Duplex
		DuplexMode,
	)

	return &TestHandler{
		BaseCommandHandler: base,
	}
}

// ProcessRequest 实现核心处理逻辑
func (h *TestHandler) ProcessRequest(ctx *CommandContext, request *TestRequest) (*TestResponse, error) {
	response := &TestResponse{
		ID:      "test_123",
		Status:  "success",
		Message: "Hello " + request.Name,
	}
	return response, nil
}

// Handle 实现命令处理完整流程
func (h *TestHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	request, err := h.ParseRequest(ctx)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	if err := h.ValidateRequest(request); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	if err := h.PreProcess(ctx, request); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	response, err := h.ProcessRequest(ctx, request)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	if err := h.PostProcess(ctx, response); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	return h.CreateSuccessResponse(response, ctx.RequestID), nil
}

func TestNewBaseCommandHandler(t *testing.T) {
	handler := NewTestHandler()

	assert.NotNil(t, handler)
	assert.Equal(t, packet.TcpMapCreate, handler.GetCommandType())
	assert.Equal(t, DirectionDuplex, handler.GetDirection()) // 替换 GetResponseType
	assert.Equal(t, DuplexMode, handler.GetCommunicationMode())
	assert.True(t, handler.IsDuplex())
	assert.False(t, handler.IsSimplex())
}

func TestBaseCommandHandler_ParseRequest(t *testing.T) {
	handler := NewTestHandler()

	ctx := &CommandContext{
		RequestBody: `{"name":"John","age":30,"email":"john@example.com","enabled":true}`,
	}

	request, err := handler.ParseRequest(ctx)
	require.NoError(t, err)
	assert.NotNil(t, request)
	assert.Equal(t, "John", request.Name)
	assert.Equal(t, 30, request.Age)
	assert.Equal(t, "john@example.com", request.Email)
	assert.True(t, request.Enabled)
}

func TestBaseCommandHandler_ParseRequest_EmptyBody(t *testing.T) {
	handler := NewTestHandler()

	ctx := &CommandContext{
		RequestBody: "",
	}

	request, err := handler.ParseRequest(ctx)
	assert.Error(t, err)
	assert.Nil(t, request)
	assert.Contains(t, err.Error(), "request body is empty")
}

func TestBaseCommandHandler_ParseRequest_InvalidJSON(t *testing.T) {
	handler := NewTestHandler()

	ctx := &CommandContext{
		RequestBody: `{"name":"John","age":"invalid"}`,
	}

	request, err := handler.ParseRequest(ctx)
	assert.Error(t, err)
	assert.Nil(t, request)
	assert.Contains(t, err.Error(), "failed to parse request body")
}

func TestBaseCommandHandler_CreateResponse(t *testing.T) {
	handler := NewTestHandler()

	testData := &TestResponse{
		ID:      "test_123",
		Status:  "success",
		Message: "Hello World",
	}

	response := handler.CreateSuccessResponse(testData, "req_456")

	assert.NotNil(t, response)
	assert.True(t, response.Success)
	assert.Equal(t, "req_456", response.RequestID)

	// 验证响应数据是JSON字符串
	var data TestResponse
	err := json.Unmarshal([]byte(response.Data), &data)
	require.NoError(t, err)
	assert.Equal(t, "test_123", data.ID)
	assert.Equal(t, "success", data.Status)
	assert.Equal(t, "Hello World", data.Message)
}

func TestBaseCommandHandler_CreateErrorResponse(t *testing.T) {
	handler := NewTestHandler()

	response := handler.CreateErrorResponse(fmt.Errorf("test error"), "req_456")

	assert.NotNil(t, response)
	assert.False(t, response.Success)
	assert.Equal(t, "req_456", response.RequestID)
	assert.Equal(t, "test error", response.Error)
	assert.Equal(t, "", response.Data) // Data 字段为空字符串
}

func TestBaseCommandHandler_Handle_Success(t *testing.T) {
	handler := NewTestHandler()

	ctx := &CommandContext{
		ConnectionID: "conn_123",
		RequestID:    "req_456",
		RequestBody:  `{"name":"Alice","age":25,"email":"alice@example.com","enabled":true}`,
	}

	response, err := handler.Handle(ctx)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.True(t, response.Success)
	assert.Equal(t, "req_456", response.RequestID)

	// 验证响应数据
	var data TestResponse
	err = json.Unmarshal([]byte(response.Data), &data)
	require.NoError(t, err)
	assert.Equal(t, "test_123", data.ID)
	assert.Equal(t, "success", data.Status)
	assert.Equal(t, "Hello Alice", data.Message)
}

func TestBaseCommandHandler_Handle_ParseError(t *testing.T) {
	handler := NewTestHandler()

	ctx := &CommandContext{
		ConnectionID: "conn_123",
		RequestID:    "req_456",
		RequestBody:  `{"name":"Alice","age":"invalid"}`,
	}

	response, err := handler.Handle(ctx)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.False(t, response.Success)
	assert.Equal(t, "req_456", response.RequestID)
	assert.Contains(t, response.Error, "failed to parse request body")
}

func TestBaseCommandHandler_ValidateContext(t *testing.T) {
	handler := NewTestHandler()

	// 测试有效上下文
	ctx := &CommandContext{
		ConnectionID: "conn_123",
		RequestID:    "req_456",
		CommandType:  packet.TcpMapCreate,
	}

	err := handler.ValidateContext(ctx)
	assert.NoError(t, err)

	// 测试无效上下文
	err = handler.ValidateContext(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command context is nil")

	// 测试缺少连接ID
	ctx.ConnectionID = ""
	err = handler.ValidateContext(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection ID is empty")

	// 测试缺少请求ID
	ctx.ConnectionID = "conn_123"
	ctx.RequestID = ""
	ctx.CommandType = 0 // 设置为无效的命令类型
	err = handler.ValidateContext(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command type is invalid")
}

func TestBaseCommandHandler_CommunicationModes(t *testing.T) {
	// 测试单工模式
	simplexHandler := NewBaseCommandHandler[TestRequest, TestResponse](
		packet.TcpMapCreate,
		DirectionOneway, // 替换 Oneway
		Simplex,
	)

	assert.True(t, simplexHandler.IsSimplex())
	assert.False(t, simplexHandler.IsDuplex())

	// 测试双工模式
	duplexHandler := NewBaseCommandHandler[TestRequest, TestResponse](
		packet.HttpMapCreate,
		DirectionDuplex, // 替换 Duplex
		DuplexMode,
	)

	assert.False(t, duplexHandler.IsSimplex())
	assert.True(t, duplexHandler.IsDuplex())
}

// CustomValidationHandler 自定义验证处理器
type CustomValidationHandler struct {
	*BaseCommandHandler[TestRequest, TestResponse]
}

// NewCustomValidationHandler 创建自定义验证处理器
func NewCustomValidationHandler() *CustomValidationHandler {
	base := NewBaseCommandHandler[TestRequest, TestResponse](
		packet.TcpMapCreate,
		DirectionDuplex, // 替换 Duplex
		DuplexMode,
	)

	return &CustomValidationHandler{
		BaseCommandHandler: base,
	}
}

// ValidateRequest 重写验证方法
func (h *CustomValidationHandler) ValidateRequest(request *TestRequest) error {
	if request.Name == "" {
		return fmt.Errorf("name is required")
	}
	if request.Age < 0 || request.Age > 150 {
		return fmt.Errorf("invalid age: %d", request.Age)
	}
	if request.Email == "" {
		return fmt.Errorf("email is required")
	}
	return nil
}

// ProcessRequest 实现核心处理逻辑
func (h *CustomValidationHandler) ProcessRequest(ctx *CommandContext, request *TestRequest) (*TestResponse, error) {
	response := &TestResponse{
		ID:      "validated_123",
		Status:  "validated",
		Message: "Validation passed for " + request.Name,
	}
	return response, nil
}

// Handle 实现命令处理完整流程
func (h *CustomValidationHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	request, err := h.ParseRequest(ctx)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	if err := h.ValidateRequest(request); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	if err := h.PreProcess(ctx, request); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	response, err := h.ProcessRequest(ctx, request)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	if err := h.PostProcess(ctx, response); err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}
	return h.CreateSuccessResponse(response, ctx.RequestID), nil
}

func TestBaseCommandHandler_CustomValidation(t *testing.T) {
	handler := NewCustomValidationHandler()

	// 测试有效请求
	ctx := &CommandContext{
		ConnectionID: "conn_123",
		RequestID:    "req_456",
		RequestBody:  `{"name":"Bob","age":30,"email":"bob@example.com","enabled":true}`,
	}

	response, err := handler.Handle(ctx)
	require.NoError(t, err)
	assert.True(t, response.Success)

	// 测试无效请求 - 缺少名称
	ctx.RequestBody = `{"age":30,"email":"bob@example.com","enabled":true}`
	response, err = handler.Handle(ctx)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "name is required")

	// 测试无效请求 - 无效年龄
	ctx.RequestBody = `{"name":"Bob","age":200,"email":"bob@example.com","enabled":true}`
	response, err = handler.Handle(ctx)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "invalid age")

	// 测试无效请求 - 缺少邮箱
	ctx.RequestBody = `{"name":"Bob","age":30,"enabled":true}`
	response, err = handler.Handle(ctx)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "email is required")
}
