package events

import (
	"context"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/packet"
)

func TestEventBus(t *testing.T) {
	// 创建事件总线
	ctx := context.Background()
	bus := NewEventBus(ctx)
	defer bus.Close()

	// 测试事件计数器
	var eventCount int
	var mu sync.Mutex

	// 创建事件处理器
	handler := func(event Event) error {
		mu.Lock()
		eventCount++
		mu.Unlock()
		return nil
	}

	// 订阅事件
	if err := bus.Subscribe("TestEvent", handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 发布事件
	testEvent := &BaseEvent{
		EventType:   "TestEvent",
		EventTime:   time.Now(),
		EventSource: "Test",
	}

	if err := bus.Publish(testEvent); err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// 等待事件处理
	time.Sleep(100 * time.Millisecond)

	// 验证事件被处理
	mu.Lock()
	if eventCount != 1 {
		t.Errorf("Expected 1 event, got %d", eventCount)
	}
	mu.Unlock()
}

func TestCommandEvents(t *testing.T) {
	// 创建事件总线
	ctx := context.Background()
	bus := NewEventBus(ctx)
	defer bus.Close()

	// 测试命令事件
	var receivedEvent *CommandReceivedEvent
	var mu sync.Mutex

	// 创建命令事件处理器
	handler := func(event Event) error {
		mu.Lock()
		if cmdEvent, ok := event.(*CommandReceivedEvent); ok {
			receivedEvent = cmdEvent
		}
		mu.Unlock()
		return nil
	}

	// 订阅命令事件
	if err := bus.Subscribe("CommandReceived", handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 创建命令事件
	cmdEvent := NewCommandReceivedEvent(
		"conn_123",
		packet.TcpMapCreate,
		"cmd_456",
		"req_789",
		"sender_1",
		"receiver_1",
		`{"port": 8080}`,
	)

	// 发布事件
	if err := bus.Publish(cmdEvent); err != nil {
		t.Fatalf("Failed to publish command event: %v", err)
	}

	// 等待事件处理
	time.Sleep(100 * time.Millisecond)

	// 验证事件被正确处理
	mu.Lock()
	if receivedEvent == nil {
		t.Error("Command event was not received")
	} else {
		if receivedEvent.ConnectionID != "conn_123" {
			t.Errorf("Expected connection ID 'conn_123', got '%s'", receivedEvent.ConnectionID)
		}
		if receivedEvent.CommandType != packet.TcpMapCreate {
			t.Errorf("Expected command type %v, got %v", packet.TcpMapCreate, receivedEvent.CommandType)
		}
		if receivedEvent.CommandBody != `{"port": 8080}` {
			t.Errorf("Expected command body '{\"port\": 8080}', got '%s'", receivedEvent.CommandBody)
		}
	}
	mu.Unlock()
}

func TestConnectionEvents(t *testing.T) {
	// 创建事件总线
	ctx := context.Background()
	bus := NewEventBus(ctx)
	defer bus.Close()

	// 测试连接事件
	var establishedEvent *ConnectionEstablishedEvent
	var closedEvent *ConnectionClosedEvent
	var mu sync.Mutex

	// 创建连接事件处理器
	handler := func(event Event) error {
		mu.Lock()
		switch e := event.(type) {
		case *ConnectionEstablishedEvent:
			establishedEvent = e
		case *ConnectionClosedEvent:
			closedEvent = e
		}
		mu.Unlock()
		return nil
	}

	// 订阅连接事件
	if err := bus.Subscribe("ConnectionEstablished", handler); err != nil {
		t.Fatalf("Failed to subscribe to ConnectionEstablished: %v", err)
	}
	if err := bus.Subscribe("ConnectionClosed", handler); err != nil {
		t.Fatalf("Failed to subscribe to ConnectionClosed: %v", err)
	}

	// 发布连接建立事件
	estEvent := NewConnectionEstablishedEvent("conn_123", "client_info", "tcp")
	if err := bus.Publish(estEvent); err != nil {
		t.Fatalf("Failed to publish connection established event: %v", err)
	}

	// 发布连接关闭事件
	closeEvent := NewConnectionClosedEvent("conn_123", "timeout")
	if err := bus.Publish(closeEvent); err != nil {
		t.Fatalf("Failed to publish connection closed event: %v", err)
	}

	// 等待事件处理
	time.Sleep(100 * time.Millisecond)

	// 验证事件被正确处理
	mu.Lock()
	if establishedEvent == nil {
		t.Error("Connection established event was not received")
	} else {
		if establishedEvent.ConnectionID != "conn_123" {
			t.Errorf("Expected connection ID 'conn_123', got '%s'", establishedEvent.ConnectionID)
		}
		if establishedEvent.Protocol != "tcp" {
			t.Errorf("Expected protocol 'tcp', got '%s'", establishedEvent.Protocol)
		}
	}

	if closedEvent == nil {
		t.Error("Connection closed event was not received")
	} else {
		if closedEvent.ConnectionID != "conn_123" {
			t.Errorf("Expected connection ID 'conn_123', got '%s'", closedEvent.ConnectionID)
		}
		if closedEvent.Reason != "timeout" {
			t.Errorf("Expected reason 'timeout', got '%s'", closedEvent.Reason)
		}
	}
	mu.Unlock()
}

func TestEventBusConcurrency(t *testing.T) {
	// 创建事件总线
	ctx := context.Background()
	bus := NewEventBus(ctx)
	defer bus.Close()

	// 测试并发事件处理
	var eventCount int
	var mu sync.Mutex

	// 创建事件处理器
	handler := func(event Event) error {
		mu.Lock()
		eventCount++
		mu.Unlock()
		time.Sleep(10 * time.Millisecond) // 模拟处理时间
		return nil
	}

	// 订阅事件
	if err := bus.Subscribe("ConcurrentEvent", handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 并发发布多个事件
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			event := &BaseEvent{
				EventType:   "ConcurrentEvent",
				EventTime:   time.Now(),
				EventSource: "Test",
			}
			if err := bus.Publish(event); err != nil {
				t.Errorf("Failed to publish event %d: %v", index, err)
			}
		}(i)
	}

	wg.Wait()

	// 等待所有事件处理完成
	time.Sleep(200 * time.Millisecond)

	// 验证所有事件都被处理
	mu.Lock()
	if eventCount != 10 {
		t.Errorf("Expected 10 events, got %d", eventCount)
	}
	mu.Unlock()
}

func TestEventBusUnsubscribe(t *testing.T) {
	// 创建事件总线
	ctx := context.Background()
	bus := NewEventBus(ctx)
	defer bus.Close()

	// 测试取消订阅
	var eventCount int
	var mu sync.Mutex

	// 创建事件处理器
	handler := func(event Event) error {
		mu.Lock()
		eventCount++
		mu.Unlock()
		return nil
	}

	// 订阅事件
	if err := bus.Subscribe("UnsubscribeEvent", handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 发布事件
	event := &BaseEvent{
		EventType:   "UnsubscribeEvent",
		EventTime:   time.Now(),
		EventSource: "Test",
	}

	if err := bus.Publish(event); err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// 等待事件处理
	time.Sleep(100 * time.Millisecond)

	// 验证事件被处理
	mu.Lock()
	if eventCount != 1 {
		t.Errorf("Expected 1 event before unsubscribe, got %d", eventCount)
	}
	mu.Unlock()

	// 取消订阅
	if err := bus.Unsubscribe("UnsubscribeEvent", handler); err != nil {
		t.Fatalf("Failed to unsubscribe: %v", err)
	}

	// 再次发布事件
	if err := bus.Publish(event); err != nil {
		t.Fatalf("Failed to publish event after unsubscribe: %v", err)
	}

	// 等待事件处理
	time.Sleep(100 * time.Millisecond)

	// 验证事件没有被处理（因为已取消订阅）
	mu.Lock()
	if eventCount != 1 {
		t.Errorf("Expected 1 event after unsubscribe, got %d", eventCount)
	}
	mu.Unlock()
}
