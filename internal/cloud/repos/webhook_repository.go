package repos

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/storage"
)

const (
	webhookKeyPrefix    = "webhook"
	webhookListKey      = "webhooks:list"
	webhookUserListKey  = "webhooks:user:%s"
	webhookLogKeyPrefix = "webhook_log"
	webhookLogListKey   = "webhook_logs:%s"
	webhookDefaultTTL   = 0
	webhookLogTTL       = 7 * 24 * time.Hour
)

type IWebhookRepository interface {
	CreateWebhook(webhook *models.Webhook) error
	GetWebhook(webhookID string) (*models.Webhook, error)
	UpdateWebhook(webhook *models.Webhook) error
	DeleteWebhook(webhookID string) error
	ListWebhooks() ([]*models.Webhook, error)
	ListUserWebhooks(userID string) ([]*models.Webhook, error)
	ListWebhooksByEvent(event string) ([]*models.Webhook, error)

	CreateWebhookLog(log *models.WebhookLog) error
	ListWebhookLogs(webhookID string, limit int) ([]*models.WebhookLog, error)
}

type WebhookRepository struct {
	*GenericRepositoryImpl[*models.Webhook]
	storage storage.Storage
}

func NewWebhookRepository(store storage.Storage) *WebhookRepository {
	baseRepo := NewRepository(store)
	genericRepo := NewGenericRepository(baseRepo, func(w *models.Webhook) (string, error) {
		if w == nil {
			return "", fmt.Errorf("webhook is nil")
		}
		return w.ID, nil
	})

	return &WebhookRepository{
		GenericRepositoryImpl: genericRepo,
		storage:               store,
	}
}

func (r *WebhookRepository) CreateWebhook(webhook *models.Webhook) error {
	if webhook.CreatedAt.IsZero() {
		webhook.CreatedAt = time.Now()
	}
	webhook.UpdatedAt = time.Now()

	if err := r.Save(webhook, webhookKeyPrefix, webhookDefaultTTL); err != nil {
		return err
	}

	if listStore, ok := r.storage.(storage.ListStore); ok {
		listStore.AppendToList(webhookListKey, webhook.ID)
		if webhook.UserID != "" {
			userListKey := fmt.Sprintf(webhookUserListKey, webhook.UserID)
			listStore.AppendToList(userListKey, webhook.ID)
		}
	}

	return nil
}

func (r *WebhookRepository) GetWebhook(webhookID string) (*models.Webhook, error) {
	return r.Get(webhookID, webhookKeyPrefix)
}

func (r *WebhookRepository) UpdateWebhook(webhook *models.Webhook) error {
	webhook.UpdatedAt = time.Now()
	return r.Save(webhook, webhookKeyPrefix, webhookDefaultTTL)
}

func (r *WebhookRepository) DeleteWebhook(webhookID string) error {
	webhook, err := r.GetWebhook(webhookID)
	if err != nil {
		return err
	}

	if err := r.Delete(webhookID, webhookKeyPrefix); err != nil {
		return err
	}

	if listStore, ok := r.storage.(storage.ListStore); ok {
		listStore.RemoveFromList(webhookListKey, webhookID)
		if webhook.UserID != "" {
			userListKey := fmt.Sprintf(webhookUserListKey, webhook.UserID)
			listStore.RemoveFromList(userListKey, webhookID)
		}
	}

	return nil
}

func (r *WebhookRepository) ListWebhooks() ([]*models.Webhook, error) {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return []*models.Webhook{}, nil
	}

	ids, err := listStore.GetList(webhookListKey)
	if err != nil {
		return []*models.Webhook{}, nil
	}

	var webhooks []*models.Webhook
	for _, item := range ids {
		if id, ok := item.(string); ok {
			webhook, err := r.GetWebhook(id)
			if err == nil && webhook != nil {
				webhooks = append(webhooks, webhook)
			}
		}
	}

	return webhooks, nil
}

func (r *WebhookRepository) ListUserWebhooks(userID string) ([]*models.Webhook, error) {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return []*models.Webhook{}, nil
	}

	userListKey := fmt.Sprintf(webhookUserListKey, userID)
	ids, err := listStore.GetList(userListKey)
	if err != nil {
		return []*models.Webhook{}, nil
	}

	var webhooks []*models.Webhook
	for _, item := range ids {
		if id, ok := item.(string); ok {
			webhook, err := r.GetWebhook(id)
			if err == nil && webhook != nil {
				webhooks = append(webhooks, webhook)
			}
		}
	}

	return webhooks, nil
}

func (r *WebhookRepository) ListWebhooksByEvent(event string) ([]*models.Webhook, error) {
	webhooks, err := r.ListWebhooks()
	if err != nil {
		return nil, err
	}

	var matched []*models.Webhook
	for _, w := range webhooks {
		if w.Enabled && w.HasEvent(event) {
			matched = append(matched, w)
		}
	}

	return matched, nil
}

func (r *WebhookRepository) CreateWebhookLog(log *models.WebhookLog) error {
	key := fmt.Sprintf("%s:%s", webhookLogKeyPrefix, log.ID)
	if err := r.storage.Set(key, log, webhookLogTTL); err != nil {
		return err
	}

	if listStore, ok := r.storage.(storage.ListStore); ok {
		listKey := fmt.Sprintf(webhookLogListKey, log.WebhookID)
		listStore.AppendToList(listKey, log.ID)
	}

	return nil
}

func (r *WebhookRepository) ListWebhookLogs(webhookID string, limit int) ([]*models.WebhookLog, error) {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return []*models.WebhookLog{}, nil
	}

	listKey := fmt.Sprintf(webhookLogListKey, webhookID)
	ids, err := listStore.GetList(listKey)
	if err != nil {
		return []*models.WebhookLog{}, nil
	}

	var logs []*models.WebhookLog
	count := 0
	for i := len(ids) - 1; i >= 0 && (limit <= 0 || count < limit); i-- {
		if id, ok := ids[i].(string); ok {
			key := fmt.Sprintf("%s:%s", webhookLogKeyPrefix, id)
			data, err := r.storage.Get(key)
			if err == nil && data != nil {
				if log, ok := data.(*models.WebhookLog); ok {
					logs = append(logs, log)
					count++
				}
			}
		}
	}

	return logs, nil
}
