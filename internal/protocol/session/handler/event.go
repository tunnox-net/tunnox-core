package handler

import (
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/events"
	corelog "tunnox-core/internal/core/log"
)

// EventManagerInterface SessionManager的事件最小接口
type EventManagerInterface interface {
	// 事件总线
	GetEventBus() events.EventBus
	SetEventBus(eventBus events.EventBus) error

	// 断开连接
	DisconnectClient(clientID int64) error
}

// EventHandler 事件处理器
type EventHandler struct {
	sessionManager EventManagerInterface
	eventBus       events.EventBus
	logger         corelog.Logger
}

// EventHandlerConfig 事件处理器配置
type EventHandlerConfig struct {
	SessionManager EventManagerInterface
	EventBus       events.EventBus
	Logger         corelog.Logger
}

// NewEventHandler 创建事件处理器
func NewEventHandler(config *EventHandlerConfig) *EventHandler {
	if config == nil {
		config = &EventHandlerConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	handler := &EventHandler{
		sessionManager: config.SessionManager,
		eventBus:       config.EventBus,
		logger:         logger,
	}

	// 订阅事件
	if handler.eventBus != nil {
		if err := handler.subscribeEvents(); err != nil {
			logger.Errorf("Failed to subscribe to events: %v", err)
		}
	}

	return handler
}

// subscribeEvents 订阅事件
func (h *EventHandler) subscribeEvents() error {
	if h.eventBus == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "event bus not configured")
	}

	// 订阅断开连接请求事件
	if err := h.eventBus.Subscribe("DisconnectRequest", h.handleDisconnectRequestEvent); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to subscribe to disconnect request events")
	}

	h.logger.Debug("Event subscriptions configured")
	return nil
}

// handleDisconnectRequestEvent 处理断开连接请求事件
func (h *EventHandler) handleDisconnectRequestEvent(event events.Event) error {
	h.logger.Infof("Handling disconnect request event")

	// 由于无法从 event 获取数据，这里返回nil
	// 实际的断开连接逻辑应该在其他地方处理
	return nil
}
