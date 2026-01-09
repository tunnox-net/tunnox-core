package management

import (
	"net/http"
	"strconv"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

func (m *ManagementModule) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	webhookMgr := m.getWebhookManager()
	if webhookMgr == nil {
		m.respondError(w, http.StatusServiceUnavailable, "webhook manager not configured")
		return
	}

	var req struct {
		UserID         string   `json:"user_id"`
		Name           string   `json:"name"`
		URL            string   `json:"url"`
		Secret         string   `json:"secret"`
		Events         []string `json:"events"`
		RetryCount     int      `json:"retry_count"`
		TimeoutSeconds int      `json:"timeout_seconds"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.URL == "" {
		m.respondError(w, http.StatusBadRequest, "url is required")
		return
	}

	webhook := models.DefaultWebhook()
	webhook.UserID = req.UserID
	webhook.Name = req.Name
	webhook.URL = req.URL
	webhook.Secret = req.Secret
	webhook.Events = req.Events
	if req.RetryCount > 0 {
		webhook.RetryCount = req.RetryCount
	}
	if req.TimeoutSeconds > 0 {
		webhook.TimeoutSeconds = req.TimeoutSeconds
	}

	if err := webhookMgr.CreateWebhook(webhook); err != nil {
		corelog.Errorf("ManagementModule: failed to create webhook: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusCreated, webhook)
}

func (m *ManagementModule) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	webhookMgr := m.getWebhookManager()
	if webhookMgr == nil {
		m.respondError(w, http.StatusServiceUnavailable, "webhook manager not configured")
		return
	}

	userID := r.URL.Query().Get("user_id")

	var webhooks []*models.Webhook
	var err error

	if userID != "" {
		webhooks, err = webhookMgr.ListUserWebhooks(userID)
	} else {
		webhooks, err = webhookMgr.ListWebhooks()
	}

	if err != nil {
		corelog.Errorf("ManagementModule: failed to list webhooks: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if webhooks == nil {
		webhooks = []*models.Webhook{}
	}

	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"webhooks": webhooks,
	})
}

func (m *ManagementModule) handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	webhookMgr := m.getWebhookManager()
	if webhookMgr == nil {
		m.respondError(w, http.StatusServiceUnavailable, "webhook manager not configured")
		return
	}

	webhookID, err := getStringPathVar(r, "webhook_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	webhook, err := webhookMgr.GetWebhook(webhookID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, webhook)
}

func (m *ManagementModule) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	webhookMgr := m.getWebhookManager()
	if webhookMgr == nil {
		m.respondError(w, http.StatusServiceUnavailable, "webhook manager not configured")
		return
	}

	webhookID, err := getStringPathVar(r, "webhook_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	webhook, err := webhookMgr.GetWebhook(webhookID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		Name           string   `json:"name"`
		URL            string   `json:"url"`
		Secret         string   `json:"secret"`
		Events         []string `json:"events"`
		Enabled        *bool    `json:"enabled"`
		RetryCount     int      `json:"retry_count"`
		TimeoutSeconds int      `json:"timeout_seconds"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name != "" {
		webhook.Name = req.Name
	}
	if req.URL != "" {
		webhook.URL = req.URL
	}
	if req.Secret != "" {
		webhook.Secret = req.Secret
	}
	if req.Events != nil {
		webhook.Events = req.Events
	}
	if req.Enabled != nil {
		webhook.Enabled = *req.Enabled
	}
	if req.RetryCount > 0 {
		webhook.RetryCount = req.RetryCount
	}
	if req.TimeoutSeconds > 0 {
		webhook.TimeoutSeconds = req.TimeoutSeconds
	}

	if err := webhookMgr.UpdateWebhook(webhook); err != nil {
		corelog.Errorf("ManagementModule: failed to update webhook %s: %v", webhookID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, webhook)
}

func (m *ManagementModule) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	webhookMgr := m.getWebhookManager()
	if webhookMgr == nil {
		m.respondError(w, http.StatusServiceUnavailable, "webhook manager not configured")
		return
	}

	webhookID, err := getStringPathVar(r, "webhook_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := webhookMgr.DeleteWebhook(webhookID); err != nil {
		corelog.Errorf("ManagementModule: failed to delete webhook %s: %v", webhookID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, httpservice.MessageResponse{Message: "webhook deleted"})
}

func (m *ManagementModule) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	webhookMgr := m.getWebhookManager()
	if webhookMgr == nil {
		m.respondError(w, http.StatusServiceUnavailable, "webhook manager not configured")
		return
	}

	webhookID, err := getStringPathVar(r, "webhook_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := webhookMgr.TestWebhook(webhookID); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, httpservice.MessageResponse{Message: "webhook test successful"})
}

func (m *ManagementModule) handleListWebhookLogs(w http.ResponseWriter, r *http.Request) {
	webhookMgr := m.getWebhookManager()
	if webhookMgr == nil {
		m.respondError(w, http.StatusServiceUnavailable, "webhook manager not configured")
		return
	}

	webhookID, err := getStringPathVar(r, "webhook_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	logs, err := webhookMgr.ListWebhookLogs(webhookID, limit)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list webhook logs: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if logs == nil {
		logs = []*models.WebhookLog{}
	}

	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"logs": logs,
	})
}

func (m *ManagementModule) getWebhookManager() managers.WebhookManagerAPI {
	if m.deps != nil && m.deps.WebhookManager != nil {
		return m.deps.WebhookManager
	}
	return nil
}
