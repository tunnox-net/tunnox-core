package events

import (
	"context"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// eventBus 事件总线实现
type eventBus struct {
	subscribers map[string][]EventHandler
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	dispose.Dispose
}

// NewEventBus 创建新的事件总线
func NewEventBus(parentCtx context.Context) EventBus {
	ctx, cancel := context.WithCancel(parentCtx)

	bus := &eventBus{
		subscribers: make(map[string][]EventHandler),
		ctx:         ctx,
		cancel:      cancel,
	}

	bus.SetCtx(parentCtx, bus.onClose)
	return bus
}

// onClose 资源清理回调
func (bus *eventBus) onClose() error {
	utils.Infof("Cleaning up event bus resources...")

	// 取消上下文
	if bus.cancel != nil {
		bus.cancel()
	}

	// 清空处理器
	bus.mu.Lock()
	bus.subscribers = make(map[string][]EventHandler)
	bus.mu.Unlock()

	utils.Infof("Event bus resources cleanup completed")
	return nil
}

// Publish 发布事件
func (bus *eventBus) Publish(event Event) error {
	if bus.IsClosed() {
		return fmt.Errorf("event bus is closed")
	}

	eventType := event.Type()

	bus.mu.RLock()
	handlers, exists := bus.subscribers[eventType]
	if !exists {
		bus.mu.RUnlock()
		utils.Debugf("No handlers for event type: %s", eventType)
		return nil
	}

	// 创建处理器副本以避免并发修改
	handlersCopy := make([]EventHandler, len(handlers))
	copy(handlersCopy, handlers)
	bus.mu.RUnlock()

	utils.Debugf("Publishing event: %s, handlers count: %d", eventType, len(handlersCopy))

	// 异步处理事件
	go func() {
		for _, handler := range handlersCopy {
			select {
			case <-bus.ctx.Done():
				utils.Debugf("Event bus context cancelled, stopping event processing")
				return
			default:
				if err := handler(event); err != nil {
					utils.Errorf("Event handler failed for event %s: %v", eventType, err)
				}
			}
		}
	}()

	return nil
}

// Subscribe 订阅事件
func (bus *eventBus) Subscribe(eventType string, handler EventHandler) error {
	if bus.IsClosed() {
		return fmt.Errorf("event bus is closed")
	}

	if eventType == "" {
		return fmt.Errorf("event type cannot be empty")
	}

	if handler == nil {
		return fmt.Errorf("event handler cannot be nil")
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()

	if bus.subscribers[eventType] == nil {
		bus.subscribers[eventType] = make([]EventHandler, 0)
	}

	// 检查是否已经订阅
	for _, existingHandler := range bus.subscribers[eventType] {
		if fmt.Sprintf("%p", existingHandler) == fmt.Sprintf("%p", handler) {
			utils.Warnf("Handler already subscribed for event type: %s", eventType)
			return nil
		}
	}

	bus.subscribers[eventType] = append(bus.subscribers[eventType], handler)
	utils.Infof("Subscribed handler for event type: %s, total handlers: %d", eventType, len(bus.subscribers[eventType]))

	return nil
}

// Unsubscribe 取消订阅
func (bus *eventBus) Unsubscribe(eventType string, handler EventHandler) error {
	if bus.IsClosed() {
		return fmt.Errorf("event bus is closed")
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()

	handlers, exists := bus.subscribers[eventType]
	if !exists {
		return fmt.Errorf("no handlers found for event type: %s", eventType)
	}

	handlerPtr := fmt.Sprintf("%p", handler)
	for i, existingHandler := range handlers {
		if fmt.Sprintf("%p", existingHandler) == handlerPtr {
			// 移除处理器
			bus.subscribers[eventType] = append(handlers[:i], handlers[i+1:]...)
			utils.Infof("Unsubscribed handler for event type: %s, remaining handlers: %d", eventType, len(bus.subscribers[eventType]))
			return nil
		}
	}

	return fmt.Errorf("handler not found for event type: %s", eventType)
}

// Close 关闭事件总线
func (bus *eventBus) Close() error {
	return bus.Dispose.CloseWithError()
}

// GetHandlerCount 获取指定事件类型的处理器数量
func (bus *eventBus) GetHandlerCount(eventType string) int {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	handlers, exists := bus.subscribers[eventType]
	if !exists {
		return 0
	}
	return len(handlers)
}

// GetEventTypes 获取所有已注册的事件类型
func (bus *eventBus) GetEventTypes() []string {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	eventTypes := make([]string, 0, len(bus.subscribers))
	for eventType := range bus.subscribers {
		eventTypes = append(eventTypes, eventType)
	}
	return eventTypes
}

// WaitForEvent 等待指定类型的事件（用于测试）
func (bus *eventBus) WaitForEvent(eventType string, timeout time.Duration) (Event, error) {
	eventChan := make(chan Event, 1)

	// 创建临时处理器
	tempHandler := func(event Event) error {
		if event.Type() == eventType {
			select {
			case eventChan <- event:
			default:
			}
		}
		return nil
	}

	// 订阅事件
	if err := bus.Subscribe(eventType, tempHandler); err != nil {
		return nil, err
	}

	// 等待事件或超时
	select {
	case event := <-eventChan:
		// 取消订阅
		bus.Unsubscribe(eventType, tempHandler)
		return event, nil
	case <-time.After(timeout):
		// 取消订阅
		bus.Unsubscribe(eventType, tempHandler)
		return nil, fmt.Errorf("timeout waiting for event type: %s", eventType)
	case <-bus.ctx.Done():
		// 取消订阅
		bus.Unsubscribe(eventType, tempHandler)
		return nil, fmt.Errorf("event bus context cancelled")
	}
}
