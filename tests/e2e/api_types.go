package e2e

// 定义强类型的API请求/响应结构，避免使用 map[string]interface{}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

// UserResponse 用户响应
type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
}

// CreateClientRequest 创建客户端请求
type CreateClientRequest struct {
	UserID     string `json:"user_id"`
	ClientName string `json:"client_name"`
	ClientDesc string `json:"client_desc,omitempty"`
}

// ClientResponse 客户端响应
type ClientResponse struct {
	ID         int64  `json:"id"`
	UserID     string `json:"user_id"`
	Name       string `json:"name"` // Client名称（对应API中的client_name或匿名的name）
	ClientName string `json:"client_name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
}

// CreateMappingRequest 创建映射请求
type CreateMappingRequest struct {
	UserID         string `json:"user_id"`
	SourceClientID int64  `json:"source_client_id"`
	TargetClientID int64  `json:"target_client_id"`
	Protocol       string `json:"protocol"`
	SourcePort     int    `json:"source_port"`
	TargetHost     string `json:"target_host"`
	TargetPort     int    `json:"target_port"`
	MappingName    string `json:"mapping_name,omitempty"`
}

// MappingResponse 映射响应
type MappingResponse struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	SourceClientID int64  `json:"source_client_id"`
	TargetClientID int64  `json:"target_client_id"`
	Protocol       string `json:"protocol"`
	SourcePort     int    `json:"source_port"`
	TargetHost     string `json:"target_host"`
	TargetPort     int    `json:"target_port"`
	Status         string `json:"status"`
}

// APIResponse 通用API响应包装
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

