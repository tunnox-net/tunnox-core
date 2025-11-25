package broker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRedis 创建一个测试用的 Redis 实例
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *RedisBrokerConfig) {
	mr := miniredis.RunT(t)
	config := &RedisBrokerConfig{
		Addrs:       []string{mr.Addr()},
		Password:    "",
		DB:          0,
		ClusterMode: false,
		PoolSize:    10,
	}
	return mr, config
}

func TestRedisBroker_PublishSubscribe(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-1"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic := TopicClientOnline
	msg := ClientOnlineMessage{
		ClientID:  12345,
		NodeID:    nodeID,
		IPAddress: "192.168.1.100",
		Timestamp: time.Now().Unix(),
	}

	payload, err := json.Marshal(msg)
	require.NoError(t, err)

	// 订阅主题
	subChan, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)
	assert.NotNil(t, subChan)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	err = rb.Publish(ctx, topic, payload)
	require.NoError(t, err)

	// 接收消息
	select {
	case receivedMsg := <-subChan:
		assert.Equal(t, topic, receivedMsg.Topic)
		assert.Equal(t, payload, receivedMsg.Payload)
		assert.Equal(t, nodeID, receivedMsg.NodeID)
		assert.WithinDuration(t, time.Now(), receivedMsg.Timestamp, 1*time.Second)

		// 验证消息内容
		var receivedOnlineMsg ClientOnlineMessage
		err = json.Unmarshal(receivedMsg.Payload, &receivedOnlineMsg)
		require.NoError(t, err)
		assert.Equal(t, msg.ClientID, receivedOnlineMsg.ClientID)
		assert.Equal(t, msg.NodeID, receivedOnlineMsg.NodeID)
		assert.Equal(t, msg.IPAddress, receivedOnlineMsg.IPAddress)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestRedisBroker_MultipleSubscribers(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-2"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic := TopicConfigUpdate
	messagePayload := []byte(`{"config":"test"}`)

	// 创建多个订阅者（模拟多个节点）
	rb2, err := NewRedisBroker(ctx, config, "test-node-3")
	require.NoError(t, err)
	defer rb2.Close()

	subChan1, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)

	subChan2, err := rb2.Subscribe(ctx, topic)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	err = rb.Publish(ctx, topic, messagePayload)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)

	// 第一个订阅者接收消息
	go func() {
		defer wg.Done()
		select {
		case msg := <-subChan1:
			assert.Equal(t, topic, msg.Topic)
			assert.Equal(t, messagePayload, msg.Payload)
			assert.Equal(t, nodeID, msg.NodeID) // 发布者是 test-node-2
		case <-time.After(2 * time.Second):
			t.Error("timeout waiting for message on subChan1")
		}
	}()

	// 第二个订阅者接收消息
	go func() {
		defer wg.Done()
		select {
		case msg := <-subChan2:
			assert.Equal(t, topic, msg.Topic)
			assert.Equal(t, messagePayload, msg.Payload)
			assert.Equal(t, nodeID, msg.NodeID) // 发布者是 test-node-2
		case <-time.After(2 * time.Second):
			t.Error("timeout waiting for message on subChan2")
		}
	}()

	wg.Wait()
}

func TestRedisBroker_Unsubscribe(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-4"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic := TopicBridgeRequest
	messagePayload := []byte(`{"request_id":"test-123"}`)

	subChan, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 显式取消订阅
	err = rb.Unsubscribe(ctx, topic)
	require.NoError(t, err)

	// 验证通道已关闭
	select {
	case _, ok := <-subChan:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel should be closed immediately")
	}

	// 发布消息（不应该有订阅者收到）
	err = rb.Publish(ctx, topic, messagePayload)
	require.NoError(t, err)

	// 再次取消订阅应该失败
	err = rb.Unsubscribe(ctx, topic)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not subscribed")
}

func TestRedisBroker_Close(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-5"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)

	topic := TopicClientOffline
	subChan, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 关闭 Broker
	err = rb.Close()
	require.NoError(t, err)

	// 发布消息到已关闭的 Broker（不应该导致 panic）
	err = rb.Publish(ctx, topic, []byte("after close"))
	// Redis client 可能返回错误，也可能成功但无法投递
	// 不做强制要求，只要不 panic 即可
	t.Logf("Publish after close returned: %v", err)

	// 订阅者应该收不到消息
	select {
	case <-subChan:
		// 可能收到关闭前的消息，这是可以接受的
	case <-time.After(300 * time.Millisecond):
		// Expected
	}
}

func TestRedisBroker_ConcurrentPublish(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-6"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic := TopicNodeHeartbeat
	numMessages := 50

	subChan, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 并发发布消息
	var publishWg sync.WaitGroup
	for i := 0; i < numMessages; i++ {
		publishWg.Add(1)
		go func(idx int) {
			defer publishWg.Done()
			payload := []byte(`{"index":` + string(rune(idx+'0')) + `}`)
			err := rb.Publish(ctx, topic, payload)
			if err != nil {
				t.Errorf("failed to publish message %d: %v", idx, err)
			}
		}(i)
	}

	// 接收消息
	receivedCount := 0
	timeout := time.After(3 * time.Second)

receiveLoop:
	for {
		select {
		case msg := <-subChan:
			if msg != nil && msg.Topic == topic {
				receivedCount++
				if receivedCount >= numMessages {
					break receiveLoop
				}
			}
		case <-timeout:
			break receiveLoop
		}
	}

	publishWg.Wait()

	// 允许少量消息丢失（由于缓冲区满等原因）
	// 但至少应该收到大部分消息
	assert.GreaterOrEqual(t, receivedCount, numMessages*8/10, "should receive at least 80%% of messages")
	t.Logf("Received %d out of %d messages", receivedCount, numMessages)
}

func TestRedisBroker_CrossNode(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建两个节点的 Broker
	node1 := "node-1"
	node2 := "node-2"

	rb1, err := NewRedisBroker(ctx, config, node1)
	require.NoError(t, err)
	defer rb1.Close()

	rb2, err := NewRedisBroker(ctx, config, node2)
	require.NoError(t, err)
	defer rb2.Close()

	topic := TopicMappingCreated
	msg := []byte(`{"mapping_id":12345}`)

	// node2 订阅
	subChan2, err := rb2.Subscribe(ctx, topic)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// node1 发布
	err = rb1.Publish(ctx, topic, msg)
	require.NoError(t, err)

	// node2 应该收到消息
	select {
	case receivedMsg := <-subChan2:
		assert.Equal(t, topic, receivedMsg.Topic)
		assert.Equal(t, msg, receivedMsg.Payload)
		assert.Equal(t, node1, receivedMsg.NodeID) // 发布者是 node1
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cross-node message")
	}
}

func TestRedisBroker_DoubleSubscribe(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-7"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic := TopicClientOnline

	// 第一次订阅应该成功
	subChan1, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)
	assert.NotNil(t, subChan1)

	// 同一 topic 第二次订阅应该失败
	subChan2, err := rb.Subscribe(ctx, topic)
	assert.Error(t, err)
	assert.Nil(t, subChan2)
	assert.Contains(t, err.Error(), "already subscribed")
}

func TestRedisBroker_ConnectionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 使用无效的地址
	config := &RedisBrokerConfig{
		Addrs:       []string{"localhost:9999"},
		Password:    "",
		DB:          0,
		ClusterMode: false,
		PoolSize:    10,
	}

	rb, err := NewRedisBroker(ctx, config, "test-node")
	assert.Error(t, err)
	assert.Nil(t, rb)
	assert.Contains(t, err.Error(), "failed to connect to Redis")
}

func TestRedisBroker_MalformedMessage(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-8"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic := TopicConfigUpdate

	subChan, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 直接通过 Redis 客户端发布一个格式错误的消息
	client := redis.NewClient(&redis.Options{
		Addr: config.Addrs[0],
	})
	defer client.Close()

	err = client.Publish(ctx, topic, "invalid json {not a valid message}").Err()
	require.NoError(t, err)

	// RedisBroker 应该记录错误但不崩溃
	// 订阅者不应该收到该消息
	select {
	case msg := <-subChan:
		t.Fatalf("should not receive malformed message, got: %+v", msg)
	case <-time.After(500 * time.Millisecond):
		// Expected: no message received
	}

	// 发布一个正常的消息，确保 Broker 仍在工作
	validPayload := []byte(`{"valid":"message"}`)
	err = rb.Publish(ctx, topic, validPayload)
	require.NoError(t, err)

	select {
	case msg := <-subChan:
		assert.Equal(t, validPayload, msg.Payload)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for valid message after malformed one")
	}
}

func TestRedisBroker_MultipleTopics(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-9"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic1 := TopicClientOnline
	topic2 := TopicClientOffline
	payload1 := []byte(`{"client_id":100}`)
	payload2 := []byte(`{"client_id":200}`)

	// 订阅两个不同的主题
	subChan1, err := rb.Subscribe(ctx, topic1)
	require.NoError(t, err)

	subChan2, err := rb.Subscribe(ctx, topic2)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布到 topic1
	err = rb.Publish(ctx, topic1, payload1)
	require.NoError(t, err)

	// 只有 subChan1 应该收到消息
	select {
	case msg := <-subChan1:
		assert.Equal(t, topic1, msg.Topic)
		assert.Equal(t, payload1, msg.Payload)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message on topic1")
	}

	// subChan2 不应该收到消息
	select {
	case msg := <-subChan2:
		t.Fatalf("subChan2 should not receive message from topic1, got: %+v", msg)
	case <-time.After(200 * time.Millisecond):
		// Expected
	}

	// 发布到 topic2
	err = rb.Publish(ctx, topic2, payload2)
	require.NoError(t, err)

	// 只有 subChan2 应该收到消息
	select {
	case msg := <-subChan2:
		assert.Equal(t, topic2, msg.Topic)
		assert.Equal(t, payload2, msg.Payload)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message on topic2")
	}

	// subChan1 不应该收到消息
	select {
	case msg := <-subChan1:
		t.Fatalf("subChan1 should not receive message from topic2, got: %+v", msg)
	case <-time.After(200 * time.Millisecond):
		// Expected
	}
}

func TestRedisBroker_HighThroughput(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-10"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic := TopicNodeHeartbeat
	numMessages := 1000

	subChan, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 并发发布大量消息
	var publishWg sync.WaitGroup
	for i := 0; i < numMessages; i++ {
		publishWg.Add(1)
		go func(idx int) {
			defer publishWg.Done()
			payload := []byte(`{"heartbeat":` + string(rune(idx%10+'0')) + `}`)
			if err := rb.Publish(ctx, topic, payload); err != nil {
				t.Errorf("failed to publish message %d: %v", idx, err)
			}
		}(i)
	}

	// 接收消息
	receivedCount := 0
	timeout := time.After(5 * time.Second)

receiveLoop:
	for {
		select {
		case msg := <-subChan:
			if msg != nil && msg.Topic == topic {
				receivedCount++
				if receivedCount >= numMessages {
					break receiveLoop
				}
			}
		case <-timeout:
			break receiveLoop
		}
	}

	publishWg.Wait()

	// 高吞吐量下可能有一些消息丢失，但应该收到大部分
	assert.GreaterOrEqual(t, receivedCount, numMessages*7/10, "should receive at least 70%% of messages in high throughput test")
	t.Logf("High throughput test: received %d out of %d messages (%.2f%%)", 
		receivedCount, numMessages, float64(receivedCount)/float64(numMessages)*100)
}

func TestRedisBroker_MessageOrdering(t *testing.T) {
	mr, config := setupTestRedis(t)
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := "test-node-11"
	rb, err := NewRedisBroker(ctx, config, nodeID)
	require.NoError(t, err)
	defer rb.Close()

	topic := TopicMappingDeleted
	numMessages := 10

	subChan, err := rb.Subscribe(ctx, topic)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 顺序发布消息
	for i := 0; i < numMessages; i++ {
		payload := []byte(`{"sequence":` + string(rune(i+'0')) + `}`)
		err := rb.Publish(ctx, topic, payload)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // 小延迟确保顺序
	}

	// 接收消息
	receivedSequences := []int{}
	timeout := time.After(2 * time.Second)

receiveLoop:
	for len(receivedSequences) < numMessages {
		select {
		case msg := <-subChan:
			var data map[string]int
			err := json.Unmarshal(msg.Payload, &data)
			require.NoError(t, err)
			receivedSequences = append(receivedSequences, data["sequence"])
		case <-timeout:
			break receiveLoop
		}
	}

	// 验证消息顺序
	assert.Equal(t, numMessages, len(receivedSequences), "should receive all messages")
	for i := 0; i < numMessages; i++ {
		assert.Equal(t, i, receivedSequences[i], "messages should be in order")
	}
}

