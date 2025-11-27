package api

import (
	"net/http"
)

// handleGetUserStats 获取用户统计
func (s *ManagementAPIServer) handleGetUserStats(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	stats, err := s.cloudControl.GetUserStats(userID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, stats)
}

// handleGetClientStats 获取客户端统计
func (s *ManagementAPIServer) handleGetClientStats(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	stats, err := s.cloudControl.GetClientStats(clientID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, stats)
}

// handleGetSystemStats 获取系统统计
func (s *ManagementAPIServer) handleGetSystemStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.cloudControl.GetSystemStats()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, stats)
}

// handleGetTrafficStats 获取流量时序统计
func (s *ManagementAPIServer) handleGetTrafficStats(w http.ResponseWriter, r *http.Request) {
	// 获取时间范围参数
	timeRange := r.URL.Query().Get("time_range")
	if timeRange == "" {
		timeRange = "24h" // 默认24小时
	}

	stats, err := s.cloudControl.GetTrafficStats(timeRange)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := StatsResponse{
		TimeRange: timeRange,
		Data:      stats,
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handleGetConnectionStats 获取连接数时序统计
func (s *ManagementAPIServer) handleGetConnectionStats(w http.ResponseWriter, r *http.Request) {
	// 获取时间范围参数
	timeRange := r.URL.Query().Get("time_range")
	if timeRange == "" {
		timeRange = "24h" // 默认24小时
	}

	stats, err := s.cloudControl.GetConnectionStats(timeRange)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := StatsResponse{
		TimeRange: timeRange,
		Data:      stats,
	}
	s.respondJSON(w, http.StatusOK, response)
}
