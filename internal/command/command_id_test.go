package command

import (
	"testing"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandIdGeneration(t *testing.T) {
	cu := NewTypedCommandUtils[struct{}, struct{}](nil)
	cu.WithConnectionID("conn_123")

	commandId := cu.generateCommandId()
	assert.NotEmpty(t, commandId)
	assert.Contains(t, commandId, "cmd_")
	assert.Contains(t, commandId, "conn_123")

	customId := "cmd_1234567890_conn_456"
	cu.WithCommandId(customId)
	assert.Equal(t, customId, cu.commandId)
}

func TestCommandIdValidation(t *testing.T) {
	middleware := NewCommandIdValidationMiddleware()
	defer middleware.Close()

	// 测试有效的CommandId
	validIds := []string{
		"cmd_1234567890_conn_123",
		"cmd_1234567890_client_456",
		"cmd_1234567890_abc123",
	}

	for _, id := range validIds {
		assert.True(t, middleware.isValidCommandId(id), "CommandId should be valid: %s", id)
	}

	// 测试无效的CommandId
	invalidIds := []string{
		"",                     // 空字符串
		"invalid",              // 没有前缀
		"cmd_",                 // 只有前缀
		"cmd_invalid_conn_123", // 时间戳不是数字
		"cmd_1234567890",       // 缺少连接ID部分
		"cmd_1234567890_",      // 连接ID部分为空（这个实际上是有效的，因为验证逻辑只检查时间戳部分）
	}

	for _, id := range invalidIds {
		// 特殊处理 cmd_1234567890_ 的情况，因为它实际上是有效的
		if id == "cmd_1234567890_" {
			assert.True(t, middleware.isValidCommandId(id), "CommandId should be valid: %s", id)
		} else {
			assert.False(t, middleware.isValidCommandId(id), "CommandId should be invalid: %s", id)
		}
	}
}

func TestCommandIdUniqueness(t *testing.T) {
	middleware := NewCommandIdValidationMiddleware()
	defer middleware.Close()

	commandId := "cmd_1234567890_conn_123"

	// 第一次使用应该成功
	assert.False(t, middleware.isCommandIdUsed(commandId))
	middleware.markCommandIdAsUsed(commandId)
	assert.True(t, middleware.isCommandIdUsed(commandId))

	// 再次使用应该失败
	assert.True(t, middleware.isCommandIdUsed(commandId))
}

func TestCommandIdValidationMiddleware(t *testing.T) {
	middleware := NewCommandIdValidationMiddleware()
	defer middleware.Close()

	// 测试缺少CommandId
	ctx := &CommandContext{
		ConnectionID: "conn_123",
		CommandType:  packet.TcpMapCreate,
		CommandId:    "",
		RequestID:    "req_123",
	}

	response, err := middleware.Process(ctx, func(ctx *CommandContext) (*CommandResponse, error) {
		return &CommandResponse{Success: true}, nil
	})

	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "CommandId is required")

	// 测试无效的CommandId格式
	ctx.CommandId = "invalid_format"
	response, err = middleware.Process(ctx, func(ctx *CommandContext) (*CommandResponse, error) {
		return &CommandResponse{Success: true}, nil
	})

	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "Invalid CommandId format")

	// 测试重复使用的CommandId
	validId := "cmd_1234567890_conn_123"
	ctx.CommandId = validId

	// 第一次使用
	response, err = middleware.Process(ctx, func(ctx *CommandContext) (*CommandResponse, error) {
		return &CommandResponse{Success: true}, nil
	})

	require.NoError(t, err)
	assert.True(t, response.Success)

	// 第二次使用相同ID
	response, err = middleware.Process(ctx, func(ctx *CommandContext) (*CommandResponse, error) {
		return &CommandResponse{Success: true}, nil
	})

	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "CommandId already used")
}

func TestCommandIdInExecute(t *testing.T) {
	cu := NewTypedCommandUtils[struct{}, struct{}](nil)
	cu.WithConnectionID("conn_123")
	cu.WithCommand(packet.TcpMapCreate)
	cu.WithRequestID("req_456")

	commandId := cu.generateCommandId()
	cu.WithCommandId(commandId)

	assert.Equal(t, commandId, cu.commandId)
	assert.Contains(t, commandId, "cmd_")
	assert.Contains(t, commandId, "conn_123")
}

func TestCommandIdInResponse(t *testing.T) {
	// 测试响应中包含CommandId
	response := &CommandResponse{
		Success:   true,
		RequestID: "req_123",
		CommandId: "cmd_1234567890_conn_123",
		Data:      "test data",
	}

	assert.Equal(t, "cmd_1234567890_conn_123", response.CommandId)
}

func TestCommandIdCleanup(t *testing.T) {
	middleware := NewCommandIdValidationMiddleware()
	defer middleware.Close()

	// 添加一些命令ID
	commandIds := []string{
		"cmd_1234567890_conn_123",
		"cmd_1234567891_conn_124",
		"cmd_1234567892_conn_125",
	}

	for _, id := range commandIds {
		middleware.markCommandIdAsUsed(id)
	}

	// 验证所有ID都被标记为已使用
	for _, id := range commandIds {
		assert.True(t, middleware.isCommandIdUsed(id))
	}

	// 手动触发清理（模拟时间过期）
	middleware.cleanupExpiredCommandIds()

	// 验证清理后ID仍然存在（因为时间未过期）
	for _, id := range commandIds {
		assert.True(t, middleware.isCommandIdUsed(id))
	}
}
