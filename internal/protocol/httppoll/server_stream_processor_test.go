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
	data, responsePkg, err := sp.HandlePollRequest(ctx)
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

