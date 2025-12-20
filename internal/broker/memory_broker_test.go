package broker

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryBroker_PublishSubscribe(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker(ctx, "test-node")
	defer broker.Close()

	// 订阅主题
	msgChan, err := broker.Subscribe(ctx, TopicClientOnline)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// 发布消息
	testMsg := ClientOnlineMessage{
		ClientID:  601234567,
		NodeID:    "test-node",
		IPAddress: "192.168.1.100",
		Timestamp: time.Now().Unix(),
	}

	msgData, _ := json.Marshal(testMsg)
	if err := broker.Publish(ctx, TopicClientOnline, msgData); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// 接收消息
	select {
	case msg := <-msgChan:
		if msg.Topic != TopicClientOnline {
			t.Errorf("expected topic %s, got %s", TopicClientOnline, msg.Topic)
		}
		if msg.NodeID != "test-node" {
			t.Errorf("expected nodeID test-node, got %s", msg.NodeID)
		}

		// 解析消息内容
		var received ClientOnlineMessage
		if err := json.Unmarshal(msg.Payload, &received); err != nil {
			t.Fatalf("failed to unmarshal message: %v", err)
		}

		if received.ClientID != testMsg.ClientID {
			t.Errorf("expected clientID %d, got %d", testMsg.ClientID, received.ClientID)
		}

	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestMemoryBroker_MultipleSubscribers(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker(ctx, "test-node")
	defer broker.Close()

	// 创建3个订阅者
	sub1, _ := broker.Subscribe(ctx, TopicConfigUpdate)
	sub2, _ := broker.Subscribe(ctx, TopicConfigUpdate)
	sub3, _ := broker.Subscribe(ctx, TopicConfigUpdate)

	// 发布消息
	msg := []byte("test message")
	if err := broker.Publish(ctx, TopicConfigUpdate, msg); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// 验证所有订阅者都收到消息
	receivers := 0
	timeout := time.After(1 * time.Second)

	for i := 0; i < 3; i++ {
		select {
		case <-sub1:
			receivers++
		case <-sub2:
			receivers++
		case <-sub3:
			receivers++
		case <-timeout:
			t.Fatal("timeout waiting for messages")
		}
	}

	if receivers != 3 {
		t.Errorf("expected 3 receivers, got %d", receivers)
	}
}

func TestMemoryBroker_PublishWithoutSubscribers(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker(ctx, "test-node")
	defer broker.Close()

	// 发布消息到无订阅者的主题（应该成功，消息被丢弃）
	msg := []byte("test message")
	if err := broker.Publish(ctx, "non-existent-topic", msg); err != nil {
		t.Errorf("publish without subscribers should succeed: %v", err)
	}
}

func TestMemoryBroker_Unsubscribe(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker(ctx, "test-node")
	defer broker.Close()

	// 订阅
	msgChan, err := broker.Subscribe(ctx, TopicBridgeRequest)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// 取消订阅
	if err := broker.Unsubscribe(ctx, TopicBridgeRequest); err != nil {
		t.Fatalf("failed to unsubscribe: %v", err)
	}

	// 验证通道已关闭
	select {
	case _, ok := <-msgChan:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel should be closed immediately")
	}

	// 再次取消订阅应该失败
	if err := broker.Unsubscribe(ctx, TopicBridgeRequest); err == nil {
		t.Error("expected error when unsubscribing non-existent topic")
	}
}

func TestMemoryBroker_Close(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker(ctx, "test-node")

	// 订阅多个主题
	ch1, _ := broker.Subscribe(ctx, TopicClientOnline)
	ch2, _ := broker.Subscribe(ctx, TopicClientOffline)

	// 关闭 broker
	if err := broker.Close(); err != nil {
		t.Fatalf("failed to close broker: %v", err)
	}

	// 验证所有通道都被关闭
	_, ok1 := <-ch1
	_, ok2 := <-ch2

	if ok1 || ok2 {
		t.Error("all channels should be closed")
	}

	// 再次关闭应该成功（幂等）
	if err := broker.Close(); err != nil {
		t.Errorf("close should be idempotent: %v", err)
	}

	// 关闭后发布应该失败
	if err := broker.Publish(ctx, TopicClientOnline, []byte("test")); err == nil {
		t.Error("publish after close should fail")
	}
}

func TestMemoryBroker_ConcurrentPublish(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker(ctx, "test-node")
	defer broker.Close()

	// 订阅主题
	msgChan, _ := broker.Subscribe(ctx, TopicNodeHeartbeat)

	// 并发发布100条消息
	messageCount := 100
	done := make(chan bool)

	go func() {
		for i := 0; i < messageCount; i++ {
			msg := NodeHeartbeatMessage{
				NodeID:    "test-node",
				Address:   "localhost:8080",
				Timestamp: time.Now().Unix(),
			}
			data, _ := json.Marshal(msg)
			broker.Publish(ctx, TopicNodeHeartbeat, data)
		}
		done <- true
	}()

	// 接收消息
	received := 0
	timeout := time.After(3 * time.Second)

receiveLoop:
	for {
		select {
		case <-msgChan:
			received++
			if received == messageCount {
				break receiveLoop
			}
		case <-done:
			// 发布完成，等待剩余消息
		case <-timeout:
			break receiveLoop
		}
	}

	if received != messageCount {
		t.Errorf("expected %d messages, received %d", messageCount, received)
	}
}
