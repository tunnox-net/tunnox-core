package tests

import (
	"bytes"
	"context"
	"testing"
	io2 "tunnox-core/internal/io"
	"tunnox-core/internal/packet"
)

func TestPackageStreamSimple(t *testing.T) {
	// 简单的读写测试
	var buf bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建写入 stream
	writeStream := io2.NewPackageStream(nil, &buf, ctx)
	defer writeStream.Close()

	// 创建读取 stream
	readStream := io2.NewPackageStream(&buf, nil, ctx)
	defer readStream.Close()

	// 测试心跳包
	t.Run("Heartbeat", func(t *testing.T) {
		heartbeatPacket := &packet.TransferPacket{
			PacketType:    packet.Heartbeat,
			CommandPacket: nil,
		}

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
	})
}

func TestPackageStreamJsonCommand(t *testing.T) {
	// JsonCommand 包测试
	var buf bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeStream := io2.NewPackageStream(nil, &buf, ctx)
	defer writeStream.Close()

	t.Run("JsonCommand_NoCompression", func(t *testing.T) {
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

		// 写入包
		writtenBytes, err := writeStream.WritePacket(testPacket, false, 0)
		if err != nil {
			t.Fatalf("Failed to write packet: %v", err)
		}
		if writtenBytes <= 0 {
			t.Errorf("Expected positive bytes written, got %d", writtenBytes)
		}

		t.Logf("Wrote %d bytes", writtenBytes)

		// 读取包（直接用 buf）
		readStream := io2.NewPackageStream(bytes.NewBuffer(buf.Bytes()), nil, ctx)
		defer readStream.Close()

		readPacket, readBytes, err := readStream.ReadPacket()
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}
		if readBytes <= 0 {
			t.Errorf("Expected positive bytes read, got %d", readBytes)
		}

		// 验证包类型
		if !readPacket.PacketType.IsJsonCommand() {
			t.Error("Expected JsonCommand packet type")
		}
		if readPacket.CommandPacket == nil {
			t.Fatal("Expected non-nil CommandPacket")
		}

		// 验证数据内容
		if readPacket.CommandPacket.CommandType != testPacket.CommandPacket.CommandType {
			t.Errorf("CommandType mismatch: expected %v, got %v",
				testPacket.CommandPacket.CommandType, readPacket.CommandPacket.CommandType)
		}
		if readPacket.CommandPacket.Token != testPacket.CommandPacket.Token {
			t.Errorf("Token mismatch: expected %s, got %s",
				testPacket.CommandPacket.Token, readPacket.CommandPacket.Token)
		}

		t.Logf("Read %d bytes successfully", readBytes)
	})

	t.Run("JsonCommand_Compression", func(t *testing.T) {
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

		// 写入包
		writtenBytes, err := writeStream.WritePacket(testPacket, true, 0)
		if err != nil {
			t.Fatalf("Failed to write packet: %v", err)
		}
		if writtenBytes <= 0 {
			t.Errorf("Expected positive bytes written, got %d", writtenBytes)
		}

		t.Logf("Wrote %d bytes", writtenBytes)

		// 读取包（关键：用新 buffer）
		readStream := io2.NewPackageStream(bytes.NewBuffer(buf.Bytes()), nil, ctx)
		defer readStream.Close()

		readPacket, readBytes, err := readStream.ReadPacket()
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}
		if readBytes <= 0 {
			t.Errorf("Expected positive bytes read, got %d", readBytes)
		}

		// 验证包类型
		if !readPacket.PacketType.IsJsonCommand() {
			t.Error("Expected JsonCommand packet type")
		}
		if readPacket.CommandPacket == nil {
			t.Fatal("Expected non-nil CommandPacket")
		}

		// 验证数据内容
		if readPacket.CommandPacket.CommandType != testPacket.CommandPacket.CommandType {
			t.Errorf("CommandType mismatch: expected %v, got %v",
				testPacket.CommandPacket.CommandType, readPacket.CommandPacket.CommandType)
		}
		if readPacket.CommandPacket.Token != testPacket.CommandPacket.Token {
			t.Errorf("Token mismatch: expected %s, got %s",
				testPacket.CommandPacket.Token, readPacket.CommandPacket.Token)
		}

		t.Logf("Read %d bytes successfully", readBytes)
	})
}
