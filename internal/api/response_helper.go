package api

import (
	"encoding/json"
	"net/http"
)

// ResponseHelper 响应辅助工具
// 提供统一的API响应格式
type ResponseHelper struct{}

// NewResponseHelper 创建响应辅助工具
func NewResponseHelper() *ResponseHelper {
	return &ResponseHelper{}
}

// Success 返回成功响应
func (h *ResponseHelper) Success(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ResponseData{
		Success: true,
		Data:    data,
	}
	json.NewEncoder(w).Encode(response)
}

// Error 返回错误响应
func (h *ResponseHelper) Error(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ResponseData{
		Success: false,
		Error:   message,
	}
	json.NewEncoder(w).Encode(response)
}

// SuccessFunc 返回成功响应的函数（用于兼容旧代码）
func SuccessFunc(w http.ResponseWriter, statusCode int, data interface{}) {
	helper := NewResponseHelper()
	helper.Success(w, statusCode, data)
}

// ErrorFunc 返回错误响应的函数（用于兼容旧代码）
func ErrorFunc(w http.ResponseWriter, statusCode int, message string) {
	helper := NewResponseHelper()
	helper.Error(w, statusCode, message)
}

