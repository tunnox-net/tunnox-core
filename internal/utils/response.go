package utils

import (
	"time"

	"tunnox-core/internal/constants"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code      int         `json:"code"`                 // 状态码
	Message   string      `json:"message"`              // 响应消息
	Data      interface{} `json:"data"`                 // 响应数据
	Timestamp int64       `json:"timestamp"`            // 时间戳
	RequestID string      `json:"request_id,omitempty"` // 请求ID
}

// PageResponse 分页响应结构
type PageResponse struct {
	Response
	Page     int `json:"page"`      // 当前页码
	PageSize int `json:"page_size"` // 每页大小
	Total    int `json:"total"`     // 总记录数
	Pages    int `json:"pages"`     // 总页数
}

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Response
	Error   string `json:"error,omitempty"`   // 错误详情
	Details string `json:"details,omitempty"` // 错误描述
}

// NewResponse 创建标准响应
func NewResponse(code int, message string, data interface{}) *Response {
	return &Response{
		Code:      code,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
}

// NewSuccessResponse 创建成功响应
func NewSuccessResponse(data interface{}) *Response {
	return NewResponse(constants.HTTPStatusOK, constants.ResponseMsgSuccess, data)
}

// NewCreatedResponse 创建创建成功响应
func NewCreatedResponse(data interface{}) *Response {
	return NewResponse(constants.HTTPStatusCreated, constants.ResponseMsgCreated, data)
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(code int, message string, err error) *ErrorResponse {
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	return &ErrorResponse{
		Response: Response{
			Code:      code,
			Message:   message,
			Data:      nil,
			Timestamp: time.Now().Unix(),
		},
		Error:   errorMsg,
		Details: message,
	}
}

// NewBadRequestResponse 创建400错误响应
func NewBadRequestResponse(message string, err error) *ErrorResponse {
	if message == "" {
		message = constants.ResponseMsgBadRequest
	}
	return NewErrorResponse(constants.HTTPStatusBadRequest, message, err)
}

// NewUnauthorizedResponse 创建401错误响应
func NewUnauthorizedResponse(message string, err error) *ErrorResponse {
	if message == "" {
		message = constants.ResponseMsgUnauthorized
	}
	return NewErrorResponse(constants.HTTPStatusUnauthorized, message, err)
}

// NewForbiddenResponse 创建403错误响应
func NewForbiddenResponse(message string, err error) *ErrorResponse {
	if message == "" {
		message = constants.ResponseMsgForbidden
	}
	return NewErrorResponse(constants.HTTPStatusForbidden, message, err)
}

// NewNotFoundResponse 创建404错误响应
func NewNotFoundResponse(message string, err error) *ErrorResponse {
	if message == "" {
		message = constants.ResponseMsgNotFound
	}
	return NewErrorResponse(constants.HTTPStatusNotFound, message, err)
}

// NewInternalErrorResponse 创建500错误响应
func NewInternalErrorResponse(message string, err error) *ErrorResponse {
	if message == "" {
		message = constants.ResponseMsgInternalError
	}
	return NewErrorResponse(constants.HTTPStatusInternalServerError, message, err)
}

// NewValidationErrorResponse 创建422错误响应
func NewValidationErrorResponse(message string, err error) *ErrorResponse {
	if message == "" {
		message = constants.ResponseMsgValidationFailed
	}
	return NewErrorResponse(constants.HTTPStatusUnprocessableEntity, message, err)
}

// NewPageResponse 创建分页响应
func NewPageResponse(data interface{}, page, pageSize, total int) *PageResponse {
	pages := (total + pageSize - 1) / pageSize // 计算总页数

	return &PageResponse{
		Response: Response{
			Code:      constants.HTTPStatusOK,
			Message:   constants.ResponseMsgSuccess,
			Data:      data,
			Timestamp: time.Now().Unix(),
		},
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Pages:    pages,
	}
}

// SetRequestID 设置请求ID
func (r *Response) SetRequestID(requestID string) {
	r.RequestID = requestID
}

// SendSuccess 发送成功响应
func SendSuccess(c *gin.Context, data interface{}) {
	response := NewSuccessResponse(data)
	if requestID := c.GetString("request_id"); requestID != "" {
		response.SetRequestID(requestID)
	}
	c.JSON(constants.HTTPStatusOK, response)
}

// SendCreated 发送创建成功响应
func SendCreated(c *gin.Context, data interface{}) {
	response := NewCreatedResponse(data)
	if requestID := c.GetString("request_id"); requestID != "" {
		response.SetRequestID(requestID)
	}
	c.JSON(constants.HTTPStatusCreated, response)
}

// SendError 发送错误响应
func SendError(c *gin.Context, code int, message string, err error) {
	response := NewErrorResponse(code, message, err)
	if requestID := c.GetString("request_id"); requestID != "" {
		response.SetRequestID(requestID)
	}
	c.JSON(code, response)
}

// SendBadRequest 发送400错误响应
func SendBadRequest(c *gin.Context, message string, err error) {
	SendError(c, constants.HTTPStatusBadRequest, message, err)
}

// SendUnauthorized 发送401错误响应
func SendUnauthorized(c *gin.Context, message string, err error) {
	SendError(c, constants.HTTPStatusUnauthorized, message, err)
}

// SendForbidden 发送403错误响应
func SendForbidden(c *gin.Context, message string, err error) {
	SendError(c, constants.HTTPStatusForbidden, message, err)
}

// SendNotFound 发送404错误响应
func SendNotFound(c *gin.Context, message string, err error) {
	SendError(c, constants.HTTPStatusNotFound, message, err)
}

// SendInternalError 发送500错误响应
func SendInternalError(c *gin.Context, message string, err error) {
	SendError(c, constants.HTTPStatusInternalServerError, message, err)
}

// SendValidationError 发送422错误响应
func SendValidationError(c *gin.Context, message string, err error) {
	SendError(c, constants.HTTPStatusUnprocessableEntity, message, err)
}

// SendPageResponse 发送分页响应
func SendPageResponse(c *gin.Context, data interface{}, page, pageSize, total int) {
	response := NewPageResponse(data, page, pageSize, total)
	if requestID := c.GetString("request_id"); requestID != "" {
		response.SetRequestID(requestID)
	}
	c.JSON(constants.HTTPStatusOK, response)
}

// SendNoContent 发送204无内容响应
func SendNoContent(c *gin.Context) {
	c.Status(constants.HTTPStatusNoContent)
}

// SendFile 发送文件响应
func SendFile(c *gin.Context, filepath string) {
	c.File(filepath)
}

// SendData 发送二进制数据响应
func SendData(c *gin.Context, data []byte, contentType string) {
	if contentType == "" {
		contentType = constants.ContentTypeOctetStream
	}
	c.Data(constants.HTTPStatusOK, contentType, data)
}
