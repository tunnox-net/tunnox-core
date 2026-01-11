package management

import (
	"net/http"

	corelog "tunnox-core/internal/core/log"
)

// handleGetSystemStats 获取系统统计
func (m *ManagementModule) handleGetSystemStats(w http.ResponseWriter, r *http.Request) {
	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	stats, err := m.cloudControl.GetSystemStats()
	if err != nil {
		corelog.Errorf("ManagementModule: failed to get system stats: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, stats)
}

// handleGetUserStats 获取用户统计
func (m *ManagementModule) handleGetUserStats(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	stats, err := m.cloudControl.GetUserStats(userID)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to get user stats: %v", err)
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, stats)
}

// handleListNodes 列出节点
func (m *ManagementModule) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取所有节点服务信息
	nodes, err := m.cloudControl.GetAllNodeServiceInfo()
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list nodes: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, nodes)
}

// handleGetNode 获取节点
func (m *ManagementModule) handleGetNode(w http.ResponseWriter, r *http.Request) {
	nodeID, err := getStringPathVar(r, "node_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	node, err := m.cloudControl.GetNodeServiceInfo(nodeID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, node)
}

// handleGetTrafficStats 获取流量统计图表数据
func (m *ManagementModule) handleGetTrafficStats(w http.ResponseWriter, r *http.Request) {
	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "1h"
	}

	stats, err := m.cloudControl.GetTrafficStats(timeRange)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to get traffic stats: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, map[string]interface{}{"traffic": stats})
}
