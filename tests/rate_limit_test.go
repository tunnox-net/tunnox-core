package tests

import (
	"bytes"
	"context"
	"testing"
	"time"
	io2 "tunnox-core/internal/io"
	"tunnox-core/internal/packet"
)

func TestRateLimitSimple(t *testing.T) {
	// 设置10秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer

	writeStream := io2.NewPackageStream(nil, &buf, ctx)
	defer writeStream.Close()

	readStream := io2.NewPackageStream(&buf, nil, ctx)
	defer readStream.Close()

	// 创建简单的 CommandPacket
	commandPacket := &packet.CommandPacket{
		CommandType: packet.TcpMap,
		Token:       "test-token",
		SenderId:    "sender",
		ReceiverId:  "receiver",
		CommandBody: "test-body",
	}

	testPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: commandPacket,
	}

	t.Log("Starting rate limit test...")

	// 写入包（启用限速：1KB/s）
	start := time.Now()
	writtenBytes, err := writeStream.WritePacket(testPacket, false, 1*1024)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to write packet: %v", err)
	}
	if writtenBytes <= 0 {
		t.Errorf("Expected positive bytes written, got %d", writtenBytes)
	}

	t.Logf("Wrote %d bytes in %v", writtenBytes, duration)

	// 读取包
	readPacket, readBytes, err := readStream.ReadPacket()
	if err != nil {
		t.Fatalf("Failed to read packet: %v", err)
	}
	if readBytes <= 0 {
		t.Errorf("Expected positive bytes read, got %d", readBytes)
	}

	// 验证数据内容
	if readPacket.CommandPacket == nil {
		t.Fatal("Expected non-nil CommandPacket")
	}
	if readPacket.CommandPacket.CommandType != testPacket.CommandPacket.CommandType {
		t.Errorf("CommandType mismatch")
	}

	t.Logf("Read %d bytes successfully", readBytes)
	t.Log("Rate limit test completed successfully")
}
