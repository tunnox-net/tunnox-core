package events

import (
	"time"
	"tunnox-core/internal/packet"
)

// Event 事件接口
type Event interface {
	Type() string
	Timestamp() time.Time
	Source() string
}

// EventHandler 事件处理器接口
type EventHandler func(event Event) error

// EventBus 事件总线接口
type EventBus interface {
	// Publish 发布事件
	Publish(event Event) error

	// Subscribe 订阅事件
	Subscribe(eventType string, handler EventHandler) error

	// Unsubscribe 取消订阅
	Unsubscribe(eventType string, handler EventHandler) error

	// Close 关闭事件总线
	Close() error
}

// BaseEvent 基础事件实现
type BaseEvent struct {
	EventType   string    `json:"event_type"`
	EventTime   time.Time `json:"event_time"`
	EventSource string    `json:"event_source"`
}

func (e *BaseEvent) Type() string {
	return e.EventType
}

func (e *BaseEvent) Timestamp() time.Time {
	return e.EventTime
}

func (e *BaseEvent) Source() string {
	return e.EventSource
}

// CommandReceivedEvent 命令接收事件
type CommandReceivedEvent struct {
	BaseEvent
	ConnectionID string             `json:"connection_id"`
	CommandType  packet.CommandType `json:"command_type"`
	CommandId    string             `json:"command_id"`
	RequestID    string             `json:"request_id"`
	SenderID     string             `json:"sender_id"`
	ReceiverID   string             `json:"receiver_id"`
	CommandBody  string             `json:"command_body"`
}

// NewCommandReceivedEvent 创建命令接收事件
func NewCommandReceivedEvent(connectionID string, commandType packet.CommandType, commandId, requestID, senderID, receiverID, commandBody string) *CommandReceivedEvent {
	return &CommandReceivedEvent{
		BaseEvent: BaseEvent{
			EventType:   "CommandReceived",
			EventTime:   time.Now(),
			EventSource: "Session",
		},
		ConnectionID: connectionID,
		CommandType:  commandType,
		CommandId:    commandId,
		RequestID:    requestID,
		SenderID:     senderID,
		ReceiverID:   receiverID,
		CommandBody:  commandBody,
	}
}

// CommandCompletedEvent 命令完成事件
type CommandCompletedEvent struct {
	BaseEvent
	ConnectionID   string        `json:"connection_id"`
	CommandId      string        `json:"command_id"`
	RequestID      string        `json:"request_id"`
	Success        bool          `json:"success"`
	Response       string        `json:"response,omitempty"`
	Error          string        `json:"error,omitempty"`
	ProcessingTime time.Duration `json:"processing_time"`
}

// NewCommandCompletedEvent 创建命令完成事件
func NewCommandCompletedEvent(connectionID, commandId, requestID string, success bool, response, error string, processingTime time.Duration) *CommandCompletedEvent {
	return &CommandCompletedEvent{
		BaseEvent: BaseEvent{
			EventType:   "CommandCompleted",
			EventTime:   time.Now(),
			EventSource: "CommandService",
		},
		ConnectionID:   connectionID,
		CommandId:      commandId,
		RequestID:      requestID,
		Success:        success,
		Response:       response,
		Error:          error,
		ProcessingTime: processingTime,
	}
}

// ConnectionEstablishedEvent 连接建立事件
type ConnectionEstablishedEvent struct {
	BaseEvent
	ConnectionID string `json:"connection_id"`
	ClientInfo   string `json:"client_info"`
	Protocol     string `json:"protocol"`
}

// NewConnectionEstablishedEvent 创建连接建立事件
func NewConnectionEstablishedEvent(connectionID, clientInfo, protocol string) *ConnectionEstablishedEvent {
	return &ConnectionEstablishedEvent{
		BaseEvent: BaseEvent{
			EventType:   "ConnectionEstablished",
			EventTime:   time.Now(),
			EventSource: "Session",
		},
		ConnectionID: connectionID,
		ClientInfo:   clientInfo,
		Protocol:     protocol,
	}
}

// ConnectionClosedEvent 连接关闭事件
type ConnectionClosedEvent struct {
	BaseEvent
	ConnectionID string `json:"connection_id"`
	Reason       string `json:"reason"`
}

// NewConnectionClosedEvent 创建连接关闭事件
func NewConnectionClosedEvent(connectionID, reason string) *ConnectionClosedEvent {
	return &ConnectionClosedEvent{
		BaseEvent: BaseEvent{
			EventType:   "ConnectionClosed",
			EventTime:   time.Now(),
			EventSource: "Session",
		},
		ConnectionID: connectionID,
		Reason:       reason,
	}
}

// HeartbeatEvent 心跳事件
type HeartbeatEvent struct {
	BaseEvent
	ConnectionID string `json:"connection_id"`
}

// NewHeartbeatEvent 创建心跳事件
func NewHeartbeatEvent(connectionID string) *HeartbeatEvent {
	return &HeartbeatEvent{
		BaseEvent: BaseEvent{
			EventType:   "Heartbeat",
			EventTime:   time.Now(),
			EventSource: "Session",
		},
		ConnectionID: connectionID,
	}
}

// DisconnectRequestEvent 断开连接请求事件
type DisconnectRequestEvent struct {
	BaseEvent
	ConnectionID string `json:"connection_id"`
	RequestID    string `json:"request_id"`
	CommandId    string `json:"command_id"`
}

// NewDisconnectRequestEvent 创建断开连接请求事件
func NewDisconnectRequestEvent(connectionID, requestID, commandId string) *DisconnectRequestEvent {
	return &DisconnectRequestEvent{
		BaseEvent: BaseEvent{
			EventType:   "DisconnectRequest",
			EventTime:   time.Now(),
			EventSource: "CommandService",
		},
		ConnectionID: connectionID,
		RequestID:    requestID,
		CommandId:    commandId,
	}
}
