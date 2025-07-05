package cloud

import (
	"strings"

	"tunnox-core/internal/constants"
	"tunnox-core/internal/utils"

	"github.com/gin-gonic/gin"
)

// RESTHandler REST API处理器
type RESTHandler struct {
	cloudControl CloudControlAPI
}

// NewRESTHandler 创建REST处理器
func NewRESTHandler(cloudControl CloudControlAPI) *RESTHandler {
	return &RESTHandler{
		cloudControl: cloudControl,
	}
}

// RegisterRoutes 注册REST路由
func (h *RESTHandler) RegisterRoutes(router *gin.Engine) {
	v1 := router.Group(constants.APIPathV1)
	{
		// 健康检查
		v1.GET(constants.APIPathHealth, h.HealthCheck)

		// 认证相关
		auth := v1.Group(constants.APIPathAuth)
		{
			auth.POST("/login", h.Authenticate)
			auth.POST("/validate", h.ValidateToken)
			auth.POST("/refresh", h.RefreshToken)
			auth.POST("/revoke", h.RevokeToken)
		}

		// 需要认证的接口
		users := v1.Group(constants.APIPathUsers)
		users.Use(utils.AuthMiddleware(h.cloudControl))
		{
			users.POST("/create", h.CreateUser)
			users.GET("/list", h.ListUsers)
			users.GET("/:id", h.GetUser)
			users.PUT("/:id/update", h.UpdateUser)
			users.DELETE("/:id/delete", h.DeleteUser)
			users.GET("/:id/stats", h.GetUserStats)
		}

		clients := v1.Group(constants.APIPathClients)
		clients.Use(utils.AuthMiddleware(h.cloudControl))
		{
			clients.POST("/create", h.CreateClient)
			clients.GET("/list", h.ListClients)
			clients.GET("/:id", h.GetClient)
			clients.PUT("/:id/update", h.UpdateClient)
			clients.DELETE("/:id/delete", h.DeleteClient)
			clients.GET("/:id/mappings", h.GetClientMappings)
			clients.GET("/:id/stats", h.GetClientStats)
		}

		nodes := v1.Group(constants.APIPathNodes)
		nodes.Use(utils.AuthMiddleware(h.cloudControl))
		{
			nodes.POST("/register", h.RegisterNode)
			nodes.POST("/unregister", h.UnregisterNode)
			nodes.POST("/heartbeat", h.NodeHeartbeat)
			nodes.GET("/list", h.ListNodes)
			nodes.GET("/:id", h.GetNode)
		}

		mappings := v1.Group(constants.APIPathMappings)
		mappings.Use(utils.AuthMiddleware(h.cloudControl))
		{
			mappings.POST("/create", h.CreateMapping)
			mappings.GET("/list", h.ListMappings)
			mappings.GET("/:id", h.GetMapping)
			mappings.PUT("/:id/update", h.UpdateMapping)
			mappings.DELETE("/:id/delete", h.DeleteMapping)
			mappings.PUT("/:id/status", h.UpdateMappingStatus)
		}

		stats := v1.Group(constants.APIPathStats)
		stats.Use(utils.AuthMiddleware(h.cloudControl))
		{
			stats.GET("/system", h.GetSystemStats)
			stats.GET("/traffic", h.GetTrafficStats)
			stats.GET("/connections", h.GetConnectionStats)
		}

		// 匿名服务（可选认证）
		anonymous := v1.Group(constants.APIPathAnonymous)
		{
			anonymous.POST("/credentials", h.GenerateAnonymousCredentials)
			anonymous.GET("/clients", h.ListAnonymousClients)
			anonymous.DELETE("/clients/:id", h.DeleteAnonymousClient)
			anonymous.POST("/mappings", h.CreateAnonymousMapping)
			anonymous.GET("/mappings", h.GetAnonymousMappings)
		}
	}
}

// HealthCheck 健康检查
func (h *RESTHandler) HealthCheck(c *gin.Context) {
	utils.SendSuccess(c, gin.H{
		"status":    "ok",
		"timestamp": utils.GetCurrentTimestamp(),
		"version":   "1.0.0",
	})
}

// Authenticate 用户认证
func (h *RESTHandler) Authenticate(c *gin.Context) {
	var req AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	// 获取客户端IP
	req.IPAddress = h.getClientIP(c)

	resp, err := h.cloudControl.Authenticate(c.Request.Context(), &req)
	if err != nil {
		utils.SendInternalError(c, "Authentication failed", err)
		return
	}

	if !resp.Success {
		utils.SendUnauthorized(c, resp.Message, nil)
		return
	}

	utils.SendSuccess(c, resp)
}

// ValidateToken 验证令牌
func (h *RESTHandler) ValidateToken(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	resp, err := h.cloudControl.ValidateToken(c.Request.Context(), req.Token)
	if err != nil {
		utils.SendInternalError(c, "Token validation failed", err)
		return
	}

	if !resp.Success {
		utils.SendUnauthorized(c, resp.Message, nil)
		return
	}

	utils.SendSuccess(c, resp)
}

// RefreshToken 刷新令牌
func (h *RESTHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	resp, err := h.cloudControl.RefreshJWTToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		utils.SendUnauthorized(c, "Token refresh failed", err)
		return
	}

	utils.SendSuccess(c, resp)
}

// RevokeToken 撤销令牌
func (h *RESTHandler) RevokeToken(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	err := h.cloudControl.RevokeJWTToken(c.Request.Context(), req.Token)
	if err != nil {
		utils.SendInternalError(c, "Token revocation failed", err)
		return
	}

	utils.SendNoContent(c)
}

// CreateUser 创建用户
func (h *RESTHandler) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	user, err := h.cloudControl.CreateUser(c.Request.Context(), req.Username, req.Email)
	if err != nil {
		utils.SendInternalError(c, "Failed to create user", err)
		return
	}

	utils.SendCreated(c, user)
}

// ListUsers 列出用户
func (h *RESTHandler) ListUsers(c *gin.Context) {
	userType := c.Query("type")

	users, err := h.cloudControl.ListUsers(c.Request.Context(), UserType(userType))
	if err != nil {
		utils.SendInternalError(c, "Failed to list users", err)
		return
	}

	utils.SendSuccess(c, users)
}

// GetUser 获取用户
func (h *RESTHandler) GetUser(c *gin.Context) {
	userID := c.Param("id")

	user, err := h.cloudControl.GetUser(c.Request.Context(), userID)
	if err != nil {
		utils.SendNotFound(c, "User not found", err)
		return
	}

	utils.SendSuccess(c, user)
}

// UpdateUser 更新用户
func (h *RESTHandler) UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	user.ID = userID
	err := h.cloudControl.UpdateUser(c.Request.Context(), &user)
	if err != nil {
		utils.SendInternalError(c, "Failed to update user", err)
		return
	}

	utils.SendSuccess(c, user)
}

// DeleteUser 删除用户
func (h *RESTHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	err := h.cloudControl.DeleteUser(c.Request.Context(), userID)
	if err != nil {
		utils.SendInternalError(c, "Failed to delete user", err)
		return
	}

	utils.SendNoContent(c)
}

// GetUserStats 获取用户统计
func (h *RESTHandler) GetUserStats(c *gin.Context) {
	userID := c.Param("id")

	stats, err := h.cloudControl.GetUserStats(c.Request.Context(), userID)
	if err != nil {
		utils.SendInternalError(c, "Failed to get user stats", err)
		return
	}

	utils.SendSuccess(c, stats)
}

// CreateClient 创建客户端
func (h *RESTHandler) CreateClient(c *gin.Context) {
	var req struct {
		UserID     string `json:"user_id" binding:"required"`
		ClientName string `json:"client_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	client, err := h.cloudControl.CreateClient(c.Request.Context(), req.UserID, req.ClientName)
	if err != nil {
		utils.SendInternalError(c, "Failed to create client", err)
		return
	}

	utils.SendCreated(c, client)
}

// ListClients 列出客户端
func (h *RESTHandler) ListClients(c *gin.Context) {
	userID := c.Query("user_id")
	clientType := c.Query("type")

	clients, err := h.cloudControl.ListClients(c.Request.Context(), userID, ClientType(clientType))
	if err != nil {
		utils.SendInternalError(c, "Failed to list clients", err)
		return
	}

	utils.SendSuccess(c, clients)
}

// GetClient 获取客户端
func (h *RESTHandler) GetClient(c *gin.Context) {
	clientID := c.Param("id")

	client, err := h.cloudControl.GetClient(c.Request.Context(), clientID)
	if err != nil {
		utils.SendNotFound(c, "Client not found", err)
		return
	}

	utils.SendSuccess(c, client)
}

// UpdateClient 更新客户端
func (h *RESTHandler) UpdateClient(c *gin.Context) {
	clientID := c.Param("id")

	var client Client
	if err := c.ShouldBindJSON(&client); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	client.ID = clientID
	err := h.cloudControl.UpdateClient(c.Request.Context(), &client)
	if err != nil {
		utils.SendInternalError(c, "Failed to update client", err)
		return
	}

	utils.SendSuccess(c, client)
}

// DeleteClient 删除客户端
func (h *RESTHandler) DeleteClient(c *gin.Context) {
	clientID := c.Param("id")

	err := h.cloudControl.DeleteClient(c.Request.Context(), clientID)
	if err != nil {
		utils.SendInternalError(c, "Failed to delete client", err)
		return
	}

	utils.SendNoContent(c)
}

// GetClientMappings 获取客户端映射
func (h *RESTHandler) GetClientMappings(c *gin.Context) {
	clientID := c.Param("id")

	mappings, err := h.cloudControl.GetClientPortMappings(c.Request.Context(), clientID)
	if err != nil {
		utils.SendInternalError(c, "Failed to get client mappings", err)
		return
	}

	utils.SendSuccess(c, mappings)
}

// GetClientStats 获取客户端统计
func (h *RESTHandler) GetClientStats(c *gin.Context) {
	clientID := c.Param("id")

	stats, err := h.cloudControl.GetClientStats(c.Request.Context(), clientID)
	if err != nil {
		utils.SendInternalError(c, "Failed to get client stats", err)
		return
	}

	utils.SendSuccess(c, stats)
}

// RegisterNode 注册节点
func (h *RESTHandler) RegisterNode(c *gin.Context) {
	var req NodeRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	resp, err := h.cloudControl.NodeRegister(c.Request.Context(), &req)
	if err != nil {
		utils.SendInternalError(c, "Failed to register node", err)
		return
	}

	utils.SendCreated(c, resp)
}

// UnregisterNode 注销节点
func (h *RESTHandler) UnregisterNode(c *gin.Context) {
	var req NodeUnregisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	err := h.cloudControl.NodeUnregister(c.Request.Context(), &req)
	if err != nil {
		utils.SendInternalError(c, "Failed to unregister node", err)
		return
	}

	utils.SendNoContent(c)
}

// NodeHeartbeat 节点心跳
func (h *RESTHandler) NodeHeartbeat(c *gin.Context) {
	var req NodeHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	resp, err := h.cloudControl.NodeHeartbeat(c.Request.Context(), &req)
	if err != nil {
		utils.SendInternalError(c, "Failed to process heartbeat", err)
		return
	}

	utils.SendSuccess(c, resp)
}

// ListNodes 列出节点
func (h *RESTHandler) ListNodes(c *gin.Context) {
	nodes, err := h.cloudControl.GetAllNodeServiceInfo(c.Request.Context())
	if err != nil {
		utils.SendInternalError(c, "Failed to list nodes", err)
		return
	}

	utils.SendSuccess(c, nodes)
}

// GetNode 获取节点
func (h *RESTHandler) GetNode(c *gin.Context) {
	nodeID := c.Param("id")

	node, err := h.cloudControl.GetNodeServiceInfo(c.Request.Context(), nodeID)
	if err != nil {
		utils.SendNotFound(c, "Node not found", err)
		return
	}

	utils.SendSuccess(c, node)
}

// CreateMapping 创建映射
func (h *RESTHandler) CreateMapping(c *gin.Context) {
	var mapping PortMapping
	if err := c.ShouldBindJSON(&mapping); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	result, err := h.cloudControl.CreatePortMapping(c.Request.Context(), &mapping)
	if err != nil {
		utils.SendInternalError(c, "Failed to create mapping", err)
		return
	}

	utils.SendCreated(c, result)
}

// ListMappings 列出映射
func (h *RESTHandler) ListMappings(c *gin.Context) {
	userID := c.Query("user_id")
	mappingType := c.Query("type")

	mappings, err := h.cloudControl.ListPortMappings(c.Request.Context(), MappingType(mappingType))
	if err != nil {
		utils.SendInternalError(c, "Failed to list mappings", err)
		return
	}

	// 如果指定了用户ID，过滤结果
	if userID != "" {
		var filtered []*PortMapping
		for _, mapping := range mappings {
			if mapping.UserID == userID {
				filtered = append(filtered, mapping)
			}
		}
		mappings = filtered
	}

	utils.SendSuccess(c, mappings)
}

// GetMapping 获取映射
func (h *RESTHandler) GetMapping(c *gin.Context) {
	mappingID := c.Param("id")

	mapping, err := h.cloudControl.GetPortMapping(c.Request.Context(), mappingID)
	if err != nil {
		utils.SendNotFound(c, "Mapping not found", err)
		return
	}

	utils.SendSuccess(c, mapping)
}

// UpdateMapping 更新映射
func (h *RESTHandler) UpdateMapping(c *gin.Context) {
	mappingID := c.Param("id")

	var mapping PortMapping
	if err := c.ShouldBindJSON(&mapping); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	mapping.ID = mappingID
	err := h.cloudControl.UpdatePortMapping(c.Request.Context(), &mapping)
	if err != nil {
		utils.SendInternalError(c, "Failed to update mapping", err)
		return
	}

	utils.SendSuccess(c, mapping)
}

// DeleteMapping 删除映射
func (h *RESTHandler) DeleteMapping(c *gin.Context) {
	mappingID := c.Param("id")

	err := h.cloudControl.DeletePortMapping(c.Request.Context(), mappingID)
	if err != nil {
		utils.SendInternalError(c, "Failed to delete mapping", err)
		return
	}

	utils.SendNoContent(c)
}

// UpdateMappingStatus 更新映射状态
func (h *RESTHandler) UpdateMappingStatus(c *gin.Context) {
	mappingID := c.Param("id")

	var req struct {
		Status string `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	status := MappingStatus(req.Status)
	err := h.cloudControl.UpdatePortMappingStatus(c.Request.Context(), mappingID, status)
	if err != nil {
		utils.SendInternalError(c, "Failed to update mapping status", err)
		return
	}

	utils.SendSuccess(c, gin.H{"status": status})
}

// GetSystemStats 获取系统统计
func (h *RESTHandler) GetSystemStats(c *gin.Context) {
	stats, err := h.cloudControl.GetSystemStats(c.Request.Context())
	if err != nil {
		utils.SendInternalError(c, "Failed to get system stats", err)
		return
	}

	utils.SendSuccess(c, stats)
}

// GetTrafficStats 获取流量统计
func (h *RESTHandler) GetTrafficStats(c *gin.Context) {
	timeRange := c.Query("time_range")
	if timeRange == "" {
		timeRange = "24h"
	}

	stats, err := h.cloudControl.GetTrafficStats(c.Request.Context(), timeRange)
	if err != nil {
		utils.SendInternalError(c, "Failed to get traffic stats", err)
		return
	}

	utils.SendSuccess(c, stats)
}

// GetConnectionStats 获取连接统计
func (h *RESTHandler) GetConnectionStats(c *gin.Context) {
	timeRange := c.Query("time_range")
	if timeRange == "" {
		timeRange = "24h"
	}

	stats, err := h.cloudControl.GetConnectionStats(c.Request.Context(), timeRange)
	if err != nil {
		utils.SendInternalError(c, "Failed to get connection stats", err)
		return
	}

	utils.SendSuccess(c, stats)
}

// GenerateAnonymousCredentials 生成匿名凭据
func (h *RESTHandler) GenerateAnonymousCredentials(c *gin.Context) {
	client, err := h.cloudControl.GenerateAnonymousCredentials(c.Request.Context())
	if err != nil {
		utils.SendInternalError(c, "Failed to generate anonymous credentials", err)
		return
	}

	utils.SendCreated(c, client)
}

// ListAnonymousClients 列出匿名客户端
func (h *RESTHandler) ListAnonymousClients(c *gin.Context) {
	clients, err := h.cloudControl.ListAnonymousClients(c.Request.Context())
	if err != nil {
		utils.SendInternalError(c, "Failed to list anonymous clients", err)
		return
	}

	utils.SendSuccess(c, clients)
}

// DeleteAnonymousClient 删除匿名客户端
func (h *RESTHandler) DeleteAnonymousClient(c *gin.Context) {
	clientID := c.Param("id")

	err := h.cloudControl.DeleteAnonymousClient(c.Request.Context(), clientID)
	if err != nil {
		utils.SendInternalError(c, "Failed to delete anonymous client", err)
		return
	}

	utils.SendNoContent(c)
}

// CreateAnonymousMapping 创建匿名映射
func (h *RESTHandler) CreateAnonymousMapping(c *gin.Context) {
	var req struct {
		SourceClientID string `json:"source_client_id" binding:"required"`
		TargetClientID string `json:"target_client_id" binding:"required"`
		Protocol       string `json:"protocol" binding:"required"`
		SourcePort     int    `json:"source_port" binding:"required"`
		TargetPort     int    `json:"target_port" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendBadRequest(c, "Invalid request body", err)
		return
	}

	mapping, err := h.cloudControl.CreateAnonymousMapping(
		c.Request.Context(),
		req.SourceClientID,
		req.TargetClientID,
		Protocol(req.Protocol),
		req.SourcePort,
		req.TargetPort,
	)
	if err != nil {
		utils.SendInternalError(c, "Failed to create anonymous mapping", err)
		return
	}

	utils.SendCreated(c, mapping)
}

// GetAnonymousMappings 获取匿名映射
func (h *RESTHandler) GetAnonymousMappings(c *gin.Context) {
	mappings, err := h.cloudControl.GetAnonymousMappings(c.Request.Context())
	if err != nil {
		utils.SendInternalError(c, "Failed to get anonymous mappings", err)
		return
	}

	utils.SendSuccess(c, mappings)
}

// getClientIP 获取客户端真实IP
func (h *RESTHandler) getClientIP(c *gin.Context) string {
	// 优先从X-Real-IP获取
	if ip := c.GetHeader(constants.HTTPHeaderXRealIP); ip != "" {
		return ip
	}

	// 从X-Forwarded-For获取
	if ip := c.GetHeader(constants.HTTPHeaderXForwardedFor); ip != "" {
		// 取第一个IP（客户端真实IP）
		if commaIndex := strings.Index(ip, ","); commaIndex != -1 {
			return strings.TrimSpace(ip[:commaIndex])
		}
		return ip
	}

	// 从RemoteAddr获取
	return c.ClientIP()
}
