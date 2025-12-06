package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"tunnox-core/internal/health"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	healthManager      *health.HealthManager
	compositeChecker   *health.CompositeHealthChecker
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(healthManager *health.HealthManager) *HealthHandler {
	// 创建组合健康检查器（5秒超时）
	compositeChecker := health.NewCompositeHealthChecker(5 * time.Second)

	return &HealthHandler{
		healthManager:    healthManager,
		compositeChecker: compositeChecker,
	}
}

// RegisterCheckers 注册子系统健康检查器
func (h *HealthHandler) RegisterCheckers(
	storageChecker health.StorageChecker,
	brokerChecker health.BrokerChecker,
	sessionManagerChecker health.SessionManagerChecker,
) {
	if storageChecker != nil {
		h.compositeChecker.RegisterChecker("storage", health.NewStorageHealthChecker(storageChecker))
	}
	if brokerChecker != nil {
		h.compositeChecker.RegisterChecker("broker", health.NewBrokerHealthChecker(brokerChecker))
	}
	if sessionManagerChecker != nil {
		h.compositeChecker.RegisterChecker("protocol", health.NewProtocolHealthChecker(sessionManagerChecker))
	}
}

// HandleHealthz 处理 /healthz 请求
// 检查所有子系统的健康状态
func (h *HealthHandler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 获取基础健康信息
	var baseInfo *health.HealthInfo
	if h.healthManager != nil {
		baseInfo = h.healthManager.GetHealthInfo()
	} else {
		// 如果没有 HealthManager，创建基础信息
		baseInfo = &health.HealthInfo{
			Status:            health.HealthStatusHealthy,
			ActiveConnections: 0,
			ActiveTunnels:     0,
			Uptime:            0,
			AcceptingNewConns: true,
		}
	}

	// 检查所有子系统
	components := h.compositeChecker.CheckAll(ctx)
	overallStatus := h.compositeChecker.GetOverallStatus(ctx)

	// 构建响应
	response := HealthzResponse{
		Status:    string(overallStatus),
		Timestamp: time.Now(),
		Uptime:    baseInfo.Uptime,
		NodeID:    baseInfo.NodeID,
		Version:   baseInfo.Version,
		Components: make(map[string]ComponentStatusResponse),
		Summary: HealthzSummary{
			ActiveConnections: baseInfo.ActiveConnections,
			ActiveTunnels:     baseInfo.ActiveTunnels,
		},
	}

	// 转换组件状态
	for name, comp := range components {
		response.Components[name] = ComponentStatusResponse{
			Status:    string(comp.Status),
			Message:   comp.Message,
			LastCheck: comp.LastCheck,
		}
	}

	// 根据整体状态设置 HTTP 状态码
	var statusCode int
	switch overallStatus {
	case health.ComponentStatusHealthy:
		statusCode = http.StatusOK
	case health.ComponentStatusDegraded:
		statusCode = http.StatusOK // 降级但仍然可用
	case health.ComponentStatusUnhealthy:
		statusCode = http.StatusServiceUnavailable
	default:
		statusCode = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// HealthzResponse 健康检查响应
type HealthzResponse struct {
	Status     string                        `json:"status"`
	Timestamp  time.Time                     `json:"timestamp"`
	Uptime     int64                         `json:"uptime_seconds"`
	NodeID     string                        `json:"node_id,omitempty"`
	Version    string                        `json:"version,omitempty"`
	Components map[string]ComponentStatusResponse `json:"components"`
	Summary    HealthzSummary                `json:"summary"`
}

// ComponentStatusResponse 组件状态响应
type ComponentStatusResponse struct {
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	LastCheck time.Time `json:"last_check"`
}

// HealthzSummary 健康检查摘要
type HealthzSummary struct {
	ActiveConnections int `json:"active_connections"`
	ActiveTunnels     int `json:"active_tunnels"`
}

