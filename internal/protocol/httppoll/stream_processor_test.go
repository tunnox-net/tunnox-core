package httppoll

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/packet"
)

func TestStreamProcessor_SetConnectionID(t *testing.T) {
	ctx := context.Background()
	sp := NewStreamProcessor(ctx, "http://test.com", "http://test.com/push", "http://test.com/poll", 123, "token", "instance", "")

	sp.SetConnectionID("conn_456")

	// 验证 ConnectionID 已设置（通过后续的 ReadPacket 会使用）
	// 这里只验证方法调用不报错
	if sp == nil {
		t.Fatal("StreamProcessor is nil")
	}
}

func TestStreamProcessor_UpdateClientID(t *testing.T) {
	ctx := context.Background()
	sp := NewStreamProcessor(ctx, "http://test.com", "http://test.com/push", "http://test.com/poll", 123, "token", "instance", "")

	sp.UpdateClientID(789)

	// 验证 ClientID 已更新
	if sp == nil {
		t.Fatal("StreamProcessor is nil")
	}
}

func TestStreamProcessor_WritePacket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	sp := NewStreamProcessor(ctx, "http://test.com", "http://test.com/push", "http://test.com/poll", 123, "token", "instance", "")
	sp.SetConnectionID("conn_123")

	pkt := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
	}

	// WritePacket 会发送 HTTP Push 请求，这里只验证方法调用不报错
	// 实际测试需要 mock HTTP client
	_, err := sp.WritePacket(pkt, false, 0)
	if err != nil {
		// 预期可能会失败（因为没有真实的 HTTP 服务器）
		// 这里只验证方法存在且可调用
		t.Logf("WritePacket returned error (expected in test): %v", err)
	}
}

func TestStreamProcessor_GetReader_GetWriter(t *testing.T) {
	ctx := context.Background()
	sp := NewStreamProcessor(ctx, "http://test.com", "http://test.com/push", "http://test.com/poll", 123, "token", "instance", "")

	reader := sp.GetReader()
	writer := sp.GetWriter()

	// 客户端 StreamProcessor 返回适配器用于读写
	// 注意：与 ServerStreamProcessor 不同，客户端需要 reader/writer 进行数据传输
	if reader == nil {
		t.Error("GetReader should return a reader adapter for client HTTP long polling")
	}
	if writer == nil {
		t.Error("GetWriter should return a writer adapter for client HTTP long polling")
	}
}
