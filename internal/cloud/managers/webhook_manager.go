package managers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

type WebhookManager struct {
	*dispose.ServiceBase
	repo       repos.IWebhookRepository
	httpClient *http.Client
	mu         sync.RWMutex
	webhooks   map[string]*models.Webhook

	// lastTriggeredCache 用于减少 LastTriggered 的持久化更新频率
	// key: webhookID, value: 上次更新时间
	lastTriggeredMu    sync.RWMutex
	lastTriggeredCache map[string]time.Time
}

func NewWebhookManager(repo repos.IWebhookRepository, ctx context.Context) *WebhookManager {
	m := &WebhookManager{
		ServiceBase: dispose.NewService("WebhookManager", ctx),
		repo:        repo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		webhooks:           make(map[string]*models.Webhook),
		lastTriggeredCache: make(map[string]time.Time),
	}

	m.loadWebhooks()
	return m
}

func (m *WebhookManager) loadWebhooks() {
	if m.repo == nil {
		return
	}

	webhooks, err := m.repo.ListWebhooks()
	if err != nil {
		corelog.Warnf("WebhookManager: failed to load webhooks: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, w := range webhooks {
		m.webhooks[w.ID] = w
	}
	corelog.Infof("WebhookManager: loaded %d webhooks", len(webhooks))
}

func (m *WebhookManager) CreateWebhook(webhook *models.Webhook) error {
	// 去重检查：如果已存在同名且同URL的webhook，则更新而不是创建新的
	if webhook.Name != "" && webhook.URL != "" {
		m.mu.RLock()
		for _, existing := range m.webhooks {
			if existing.Name == webhook.Name && existing.URL == webhook.URL {
				m.mu.RUnlock()
				// 复用已存在的 ID，更新其他字段
				webhook.ID = existing.ID
				corelog.Infof("WebhookManager: webhook with name=%s url=%s already exists (id=%s), updating instead of creating", webhook.Name, webhook.URL, existing.ID)
				return m.UpdateWebhook(webhook)
			}
		}
		m.mu.RUnlock()
	}

	if webhook.ID == "" {
		webhook.ID = uuid.New().String()
	}
	if webhook.TimeoutSeconds <= 0 {
		webhook.TimeoutSeconds = 30
	}
	if webhook.RetryCount <= 0 {
		webhook.RetryCount = 3
	}

	if err := m.repo.CreateWebhook(webhook); err != nil {
		return err
	}

	m.mu.Lock()
	m.webhooks[webhook.ID] = webhook
	m.mu.Unlock()

	corelog.Infof("WebhookManager: created webhook %s for user %s", webhook.ID, webhook.UserID)
	return nil
}

func (m *WebhookManager) GetWebhook(webhookID string) (*models.Webhook, error) {
	m.mu.RLock()
	if w, ok := m.webhooks[webhookID]; ok {
		m.mu.RUnlock()
		return w, nil
	}
	m.mu.RUnlock()

	return m.repo.GetWebhook(webhookID)
}

func (m *WebhookManager) UpdateWebhook(webhook *models.Webhook) error {
	if err := m.repo.UpdateWebhook(webhook); err != nil {
		return err
	}

	m.mu.Lock()
	m.webhooks[webhook.ID] = webhook
	m.mu.Unlock()

	return nil
}

func (m *WebhookManager) DeleteWebhook(webhookID string) error {
	if err := m.repo.DeleteWebhook(webhookID); err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.webhooks, webhookID)
	m.mu.Unlock()

	corelog.Infof("WebhookManager: deleted webhook %s", webhookID)
	return nil
}

func (m *WebhookManager) ListWebhooks() ([]*models.Webhook, error) {
	return m.repo.ListWebhooks()
}

func (m *WebhookManager) ListUserWebhooks(userID string) ([]*models.Webhook, error) {
	return m.repo.ListUserWebhooks(userID)
}

func (m *WebhookManager) Dispatch(event string, data interface{}) {
	m.mu.RLock()
	var targets []*models.Webhook
	for _, w := range m.webhooks {
		if w.Enabled && w.HasEvent(event) {
			targets = append(targets, w)
		}
	}
	webhookCount := len(m.webhooks)
	m.mu.RUnlock()

	// 如果内存中没有 webhook，尝试从存储重新加载
	// 这处理了其他节点创建 webhook 后，当前节点内存未更新的情况
	if len(targets) == 0 && webhookCount == 0 {
		m.loadWebhooks()
		m.mu.RLock()
		for _, w := range m.webhooks {
			if w.Enabled && w.HasEvent(event) {
				targets = append(targets, w)
			}
		}
		m.mu.RUnlock()
	}

	if len(targets) == 0 {
		corelog.Debugf("WebhookManager: no webhook targets for event %s", event)
		return
	}

	payload := &models.WebhookPayload{
		ID:        uuid.New().String(),
		Event:     event,
		Timestamp: time.Now().UnixMilli(),
		Data:      data,
	}

	corelog.Infof("WebhookManager: dispatching event %s to %d webhooks", event, len(targets))
	for _, webhook := range targets {
		go m.sendWebhook(webhook, payload)
	}
}

func (m *WebhookManager) sendWebhook(webhook *models.Webhook, payload *models.WebhookPayload) {
	start := time.Now()

	payloadCopy := *payload
	if webhook.Secret != "" {
		payloadCopy.Signature = m.sign(payload, webhook.Secret)
	}

	body, err := json.Marshal(payloadCopy)
	if err != nil {
		corelog.Errorf("WebhookManager: failed to marshal payload: %v", err)
		return
	}

	var lastErr error
	var responseStatus int
	var responseBody string

	for attempt := 0; attempt <= webhook.RetryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		ctx, cancel := context.WithTimeout(m.Ctx(), time.Duration(webhook.TimeoutSeconds)*time.Second)
		req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewReader(body))
		if err != nil {
			cancel()
			lastErr = err
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Event", payload.Event)
		req.Header.Set("X-Webhook-ID", payload.ID)
		if webhook.Secret != "" {
			req.Header.Set("X-Webhook-Signature", payloadCopy.Signature)
		}

		resp, err := m.httpClient.Do(req)
		cancel()

		if err != nil {
			lastErr = err
			continue
		}

		responseStatus = resp.StatusCode
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		responseBody = string(respBody)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			m.logWebhookSend(webhook, payload, responseStatus, responseBody, true, start)
			return
		}

		lastErr = coreerrors.Newf(coreerrors.CodeNetworkError, "webhook returned status %d", resp.StatusCode)
	}

	m.logWebhookSend(webhook, payload, responseStatus, responseBody, false, start)
	corelog.Warnf("WebhookManager: failed to send webhook %s after %d attempts: %v",
		webhook.ID, webhook.RetryCount+1, lastErr)
}

func (m *WebhookManager) sign(payload *models.WebhookPayload, secret string) string {
	data, _ := json.Marshal(struct {
		ID        string      `json:"id"`
		Event     string      `json:"event"`
		Timestamp int64       `json:"timestamp"`
		Data      interface{} `json:"data"`
	}{
		ID:        payload.ID,
		Event:     payload.Event,
		Timestamp: payload.Timestamp,
		Data:      payload.Data,
	})

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

const lastTriggeredUpdateInterval = 60 * time.Second

func (m *WebhookManager) logWebhookSend(webhook *models.Webhook, payload *models.WebhookPayload, status int, respBody string, success bool, start time.Time) {
	payloadJSON, _ := json.Marshal(payload)

	log := &models.WebhookLog{
		ID:             uuid.New().String(),
		WebhookID:      webhook.ID,
		EventType:      payload.Event,
		Payload:        string(payloadJSON),
		ResponseStatus: status,
		ResponseBody:   respBody,
		Success:        success,
		SentAt:         start,
		Duration:       time.Since(start).Milliseconds(),
	}

	go func() {
		if err := m.repo.CreateWebhookLog(log); err != nil {
			corelog.Warnf("WebhookManager: failed to save webhook log: %v", err)
		}
	}()

	m.updateLastTriggeredDebounced(webhook)
}

func (m *WebhookManager) updateLastTriggeredDebounced(webhook *models.Webhook) {
	m.lastTriggeredMu.RLock()
	lastUpdate, exists := m.lastTriggeredCache[webhook.ID]
	m.lastTriggeredMu.RUnlock()

	now := time.Now()
	if exists && now.Sub(lastUpdate) < lastTriggeredUpdateInterval {
		return
	}

	m.lastTriggeredMu.Lock()
	if lastUpdate, exists = m.lastTriggeredCache[webhook.ID]; exists && now.Sub(lastUpdate) < lastTriggeredUpdateInterval {
		m.lastTriggeredMu.Unlock()
		return
	}
	m.lastTriggeredCache[webhook.ID] = now
	m.lastTriggeredMu.Unlock()

	go func() {
		webhook.LastTriggered = &now
		m.repo.UpdateWebhook(webhook)
	}()
}

func (m *WebhookManager) TestWebhook(webhookID string) error {
	webhook, err := m.GetWebhook(webhookID)
	if err != nil {
		return err
	}

	payload := &models.WebhookPayload{
		ID:        uuid.New().String(),
		Event:     "test",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]string{
			"message": "This is a test webhook delivery",
		},
	}

	if webhook.Secret != "" {
		payload.Signature = m.sign(payload, webhook.Secret)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(m.Ctx(), time.Duration(webhook.TimeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", "test")
	req.Header.Set("X-Webhook-ID", payload.ID)
	if webhook.Secret != "" {
		req.Header.Set("X-Webhook-Signature", payload.Signature)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "webhook test failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return coreerrors.Newf(coreerrors.CodeNetworkError, "webhook returned status %d", resp.StatusCode)
}

func (m *WebhookManager) ListWebhookLogs(webhookID string, limit int) ([]*models.WebhookLog, error) {
	return m.repo.ListWebhookLogs(webhookID, limit)
}

func (m *WebhookManager) DispatchClientOnline(clientID int64, userID, ipAddress, nodeID string) {
	m.Dispatch(string(models.WebhookEventClientOnline), &models.WebhookClientEventData{
		ClientID:  clientID,
		UserID:    userID,
		Status:    "online",
		IPAddress: ipAddress,
		NodeID:    nodeID,
	})
}

func (m *WebhookManager) DispatchClientOffline(clientID int64, userID string) {
	m.Dispatch(string(models.WebhookEventClientOffline), &models.WebhookClientEventData{
		ClientID: clientID,
		UserID:   userID,
		Status:   "offline",
	})
}

func (m *WebhookManager) DispatchMappingCreated(mapping *models.PortMapping) {
	m.Dispatch(string(models.WebhookEventMappingCreated), &models.WebhookMappingEventData{
		MappingID:      mapping.ID,
		UserID:         mapping.UserID,
		Protocol:       string(mapping.Protocol),
		ListenClientID: mapping.ListenClientID,
		TargetClientID: mapping.TargetClientID,
		Status:         string(mapping.Status),
	})
}

func (m *WebhookManager) DispatchMappingDeleted(mappingID, userID string) {
	m.Dispatch(string(models.WebhookEventMappingDeleted), &models.WebhookMappingEventData{
		MappingID: mappingID,
		UserID:    userID,
	})
}

func (m *WebhookManager) DispatchTunnelOpened(tunnelID, mappingID string, clientID int64, targetHost string, targetPort int) {
	m.Dispatch(string(models.WebhookEventTunnelOpened), &models.WebhookTunnelEventData{
		TunnelID:   tunnelID,
		MappingID:  mappingID,
		ClientID:   clientID,
		TargetHost: targetHost,
		TargetPort: targetPort,
	})
}

func (m *WebhookManager) DispatchTunnelClosed(tunnelID, mappingID string, clientID int64) {
	m.Dispatch(string(models.WebhookEventTunnelClosed), &models.WebhookTunnelEventData{
		TunnelID:  tunnelID,
		MappingID: mappingID,
		ClientID:  clientID,
	})
}

func (m *WebhookManager) DispatchTrafficQuotaWarning(userID string, usedBytes, limitBytes int64, usedPercent int) {
	m.Dispatch(string(models.WebhookEventTrafficQuotaWarning), &models.WebhookTrafficEventData{
		UserID:      userID,
		UsedBytes:   usedBytes,
		LimitBytes:  limitBytes,
		UsedPercent: usedPercent,
	})
}

type WebhookManagerAPI interface {
	CreateWebhook(webhook *models.Webhook) error
	GetWebhook(webhookID string) (*models.Webhook, error)
	UpdateWebhook(webhook *models.Webhook) error
	DeleteWebhook(webhookID string) error
	ListWebhooks() ([]*models.Webhook, error)
	ListUserWebhooks(userID string) ([]*models.Webhook, error)
	TestWebhook(webhookID string) error
	ListWebhookLogs(webhookID string, limit int) ([]*models.WebhookLog, error)

	Dispatch(event string, data interface{})
	DispatchClientOnline(clientID int64, userID, ipAddress, nodeID string)
	DispatchClientOffline(clientID int64, userID string)
	DispatchMappingCreated(mapping *models.PortMapping)
	DispatchMappingDeleted(mappingID, userID string)
	DispatchTunnelOpened(tunnelID, mappingID string, clientID int64, targetHost string, targetPort int)
	DispatchTunnelClosed(tunnelID, mappingID string, clientID int64)
	DispatchTrafficQuotaWarning(userID string, usedBytes, limitBytes int64, usedPercent int)
}

var _ WebhookManagerAPI = (*WebhookManager)(nil)

func VerifyWebhookSignature(payload []byte, signature, secret string) bool {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expected := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func CreateSystemWebhook(mgr WebhookManagerAPI, name, url, secret string, events []string) (*models.Webhook, error) {
	webhook := &models.Webhook{
		ID:             uuid.New().String(),
		UserID:         "",
		Name:           name,
		URL:            url,
		Secret:         secret,
		Events:         events,
		Enabled:        true,
		RetryCount:     3,
		TimeoutSeconds: 30,
	}

	if err := mgr.CreateWebhook(webhook); err != nil {
		return nil, fmt.Errorf("failed to create system webhook: %w", err)
	}

	return webhook, nil
}
