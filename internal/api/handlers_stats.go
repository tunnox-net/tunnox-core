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

