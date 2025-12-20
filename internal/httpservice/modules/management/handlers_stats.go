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

	m.respondJSON(w, http.StatusOK, stats)
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

	m.respondJSON(w, http.StatusOK, nodes)
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

	m.respondJSON(w, http.StatusOK, node)
}
