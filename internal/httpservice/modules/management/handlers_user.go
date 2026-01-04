package management

import (
	"net/http"

	"tunnox-core/internal/cloud/models"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/utils"
)

// enrichClientsWithIPRegion 为客户端列表填充 IP 地区信息
func enrichClientsWithIPRegion(clients []*models.Client) {
	if len(clients) == 0 {
		return
	}

	// 收集所有 IP 地址
	ips := make([]string, 0, len(clients))
	for _, c := range clients {
		if c.IPAddress != "" {
			ips = append(ips, c.IPAddress)
		}
	}

	// 批量查询 IP 地区
	regions := utils.LookupIPRegionBatch(ips)

	// 填充到客户端
	for _, c := range clients {
		if c.IPAddress != "" {
			c.IPRegion = regions[c.IPAddress]
		}
	}
}

// handleCreateUser 创建用户
func (m *ManagementModule) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	user, err := m.cloudControl.CreateUser(req.Username, req.Email)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to create user: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusCreated, user)
}

// handleGetUser 获取用户
func (m *ManagementModule) handleGetUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	user, err := m.cloudControl.GetUser(userID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, user)
}

// handleUpdateUser 更新用户
func (m *ManagementModule) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		Username string            `json:"username,omitempty"`
		Email    string            `json:"email,omitempty"`
		Status   models.UserStatus `json:"status,omitempty"`
		Plan     models.UserPlan   `json:"plan,omitempty"`
		Quota    *models.UserQuota `json:"quota,omitempty"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取现有用户
	user, err := m.cloudControl.GetUser(userID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 更新字段
	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Status != "" {
		user.Status = req.Status
	}
	if req.Plan != "" {
		user.Plan = req.Plan
	}
	if req.Quota != nil {
		user.Quota = *req.Quota
	}

	// 保存更新
	if err := m.cloudControl.UpdateUser(user); err != nil {
		corelog.Errorf("ManagementModule: failed to update user %s: %v", userID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, user)
}

// handleDeleteUser 删除用户
func (m *ManagementModule) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	if err := m.cloudControl.DeleteUser(userID); err != nil {
		corelog.Errorf("ManagementModule: failed to delete user %s: %v", userID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, httpservice.MessageResponse{Message: "user deleted"})
}

// handleListUsers 列出用户
func (m *ManagementModule) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取查询参数
	userType := models.UserType(r.URL.Query().Get("type"))

	users, err := m.cloudControl.ListUsers(userType)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list users: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 确保返回空数组而非 nil
	if users == nil {
		users = []*models.User{}
	}

	// 包装成对象返回，符合 platform 期望的格式
	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"users": users,
	})
}

// handleListUserClients 列出用户的客户端
func (m *ManagementModule) handleListUserClients(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	clients, err := m.cloudControl.ListUserClients(userID)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list user clients: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 填充 IP 地区信息（GeoIP 解析）
	enrichClientsWithIPRegion(clients)

	// 包装成对象返回，符合 platform 期望的格式
	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"clients": clients,
	})
}

// handleListUserMappings 列出用户的映射
func (m *ManagementModule) handleListUserMappings(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	mappings, err := m.cloudControl.GetUserPortMappings(userID)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list user mappings: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 包装成对象返回，符合 platform 期望的格式
	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"mappings": mappings,
	})
}

// UserQuotaResponse 用户配额响应
type UserQuotaResponse struct {
	Quota *models.UserQuota `json:"quota"` // 配额限制
	Usage *QuotaUsage       `json:"usage"` // 当前使用量
}

// QuotaUsage 配额使用量
type QuotaUsage struct {
	TotalMappings  int   `json:"total_mappings"`  // 总隧道数
	HTTPMappings   int   `json:"http_mappings"`   // HTTP 隧道数
	ActiveConns    int   `json:"active_conns"`    // 活跃连接数
	MonthlyTraffic int64 `json:"monthly_traffic"` // 当月流量
}

// handleGetUserQuota 获取用户配额和使用量
func (m *ManagementModule) handleGetUserQuota(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 配额检查器未配置时返回无限配额
	if m.quotaChecker == nil {
		respondJSONTyped(w, http.StatusOK, UserQuotaResponse{
			Quota: &models.UserQuota{
				MaxMappings:    0, // 0 表示无限制
				MaxHTTPDomains: 0,
			},
			Usage: &QuotaUsage{},
		})
		return
	}

	// 获取配额
	quota, err := m.quotaChecker.GetUserQuota(userID)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to get user quota: %v", err)
		m.respondError(w, http.StatusInternalServerError, "failed to get quota")
		return
	}

	// 获取使用量
	usage, err := m.quotaChecker.GetUserUsage(userID)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to get user usage: %v", err)
		m.respondError(w, http.StatusInternalServerError, "failed to get usage")
		return
	}

	respondJSONTyped(w, http.StatusOK, UserQuotaResponse{
		Quota: quota,
		Usage: &QuotaUsage{
			TotalMappings:  usage.TotalMappings,
			HTTPMappings:   usage.HTTPMappings,
			ActiveConns:    usage.ActiveConns,
			MonthlyTraffic: usage.MonthlyTraffic,
		},
	})
}
