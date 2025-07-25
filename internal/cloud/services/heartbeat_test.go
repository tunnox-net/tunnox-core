package services

import (
	"bytes"
	"context"
	"testing"
	"time"
	"tunnox-core/internal/packet"
	io2 "tunnox-core/internal/stream"
)

func TestHeartbeatOnly(t *testing.T) {
	// 设置5秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 只测试心跳包
	var buf bytes.Buffer

	// 创建写入 stream
	writeStream := io2.NewStreamProcessor(nil, &buf, ctx)
	defer writeStream.Close()

	// 创建读取 stream
	readStream := io2.NewStreamProcessor(&buf, nil, ctx)
	defer readStream.Close()

	heartbeatPacket := &packet.TransferPacket{
		PacketType:    packet.Heartbeat,
		CommandPacket: nil,
	}

	t.Log("Starting heartbeat test...")

	// 写入心跳包
	writtenBytes, err := writeStream.WritePacket(heartbeatPacket, false, 0)
	if err != nil {
		t.Fatalf("Failed to write heartbeat: %v", err)
	}
	if writtenBytes != 1 {
		t.Errorf("Expected 1 byte written, got %d", writtenBytes)
	}

	t.Logf("Wrote %d bytes", writtenBytes)

	// 读取心跳包
	readPacket, readBytes, err := readStream.ReadPacket()
	if err != nil {
		t.Fatalf("Failed to read heartbeat: %v", err)
	}
	if readBytes != 1 {
		t.Errorf("Expected 1 byte read, got %d", readBytes)
	}
	if !readPacket.PacketType.IsHeartbeat() {
		t.Error("Expected heartbeat packet type")
	}
	if readPacket.CommandPacket != nil {
		t.Error("Expected nil CommandPacket for heartbeat")
	}

	t.Logf("Read %d bytes successfully", readBytes)
	t.Log("Heartbeat test completed successfully")
}
