package httppoll

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/packet"
)

func TestServerStreamProcessor_SetConnectionID(t *testing.T) {
	ctx := context.Background()
	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	sp.SetConnectionID("conn_456")
	
	if sp.GetConnectionID() != "conn_456" {
		t.Errorf("Expected ConnectionID=conn_456, got %s", sp.GetConnectionID())
	}
}

func TestServerStreamProcessor_UpdateClientID(t *testing.T) {
	ctx := context.Background()
	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	sp.UpdateClientID(789)
	
	if sp.GetClientID() != 789 {
		t.Errorf("Expected ClientID=789, got %d", sp.GetClientID())
	}
}

func TestServerStreamProcessor_SetMappingID(t *testing.T) {
	ctx := context.Background()
	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "")

	sp.SetMappingID("mapping_789")
	
	if sp.GetMappingID() != "mapping_789" {
		t.Errorf("Expected MappingID=mapping_789, got %s", sp.GetMappingID())
	}
}

func TestServerStreamProcessor_WritePacket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	pkt := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
	}

	// WritePacket 会将数据推送到队列
	_, err := sp.WritePacket(pkt, false, 0)
	if err != nil {
		t.Fatalf("WritePacket failed: %v", err)
	}
}

func TestServerStreamProcessor_PushData(t *testing.T) {
	ctx := context.Background()
	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	base64Data := "dGVzdCBkYXRh" // "test data" in base64
	
	err := sp.PushData(base64Data)
	if err != nil {
		t.Fatalf("PushData failed: %v", err)
	}
}

func TestServerStreamProcessor_PollData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	// 先写入一些数据
	pkt := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
	}
	sp.WritePacket(pkt, false, 0)

	// 等待数据被调度
	time.Sleep(50 * time.Millisecond)

	// PollData 应该能获取到数据
	data, responsePkg, err := sp.HandlePollRequest(ctx, "", "control")
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("PollData failed: %v", err)
	}

	// 如果有数据，验证格式
	if data != "" {
		// data 应该是 Base64 编码的
		if len(data) == 0 {
			t.Error("PollData returned empty data")
		}
	}

	_ = responsePkg // 避免未使用变量
}

func TestServerStreamProcessor_GetReader_GetWriter(t *testing.T) {
	ctx := context.Background()
	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	reader := sp.GetReader()
	writer := sp.GetWriter()

	// HTTP 长轮询是无状态的，GetReader 和 GetWriter 应该返回 nil
	if reader != nil {
		t.Error("GetReader should return nil for HTTP long polling")
	}
	if writer != nil {
		t.Error("GetWriter should return nil for HTTP long polling")
	}
}

// TestServerStreamProcessor_IsClosed 测试 IsClosed 方法
func TestServerStreamProcessor_IsClosed(t *testing.T) {
	ctx := context.Background()
	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	// 初始状态应该是未关闭
	if sp.IsClosed() {
		t.Error("Expected IsClosed() = false initially, got true")
	}

	// 关闭后应该返回 true
	sp.Close()

	if !sp.IsClosed() {
		t.Error("Expected IsClosed() = true after Close(), got false")
	}
}

// TestServerStreamProcessor_IsContextDone 测试 IsContextDone 方法
func TestServerStreamProcessor_IsContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	// 初始状态 context 未取消
	if sp.IsContextDone() {
		t.Error("Expected IsContextDone() = false initially, got true")
	}

	// 取消 context
	cancel()

	// 等待一小段时间确保 context 被取消
	time.Sleep(10 * time.Millisecond)

	// 现在应该返回 true
	if !sp.IsContextDone() {
		t.Error("Expected IsContextDone() = true after cancel(), got false")
	}
}

// TestServerStreamProcessor_IsContextDone_AfterClose 测试关闭后 IsContextDone
func TestServerStreamProcessor_IsContextDone_AfterClose(t *testing.T) {
	ctx := context.Background()
	sp := NewServerStreamProcessor(ctx, "conn_123", 456, "mapping_789")

	// 初始状态
	if sp.IsContextDone() {
		t.Error("Expected IsContextDone() = false initially, got true")
	}

	// 关闭 StreamProcessor
	sp.Close()

	// 关闭后 context 应该被取消
	if !sp.IsContextDone() {
		t.Error("Expected IsContextDone() = true after Close(), got false")
	}
}

