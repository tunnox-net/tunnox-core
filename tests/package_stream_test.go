package tests

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"
	"tunnox-core/internal/packet"
	io2 "tunnox-core/internal/stream"
)

// ==================== 基本数据包读写测试 ====================

func TestPackageStream_BasicPacketReadWrite(t *testing.T) {
	// 设置10秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer

	// 创建写入和读取流
	writeStream := io2.NewStreamProcessor(nil, &buf, ctx)
	defer writeStream.Close()

	readStream := io2.NewStreamProcessor(&buf, nil, ctx)
	defer readStream.Close()

	// 创建测试数据包
	commandPacket := &packet.CommandPacket{
		CommandType: packet.TcpMap,
		Token:       "test-token-123",
		SenderId:    "sender-001",
		ReceiverId:  "receiver-001",
		CommandBody: "Hello, this is a test command body with some data!",
	}

	testPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: commandPacket,
	}

	t.Log("Starting basic packet read/write test...")

	// 写入数据包（不压缩，不限速）
	writtenBytes, err := writeStream.WritePacket(testPacket, false, 0)
	if err != nil {
		t.Fatalf("Failed to write packet: %v", err)
	}
	if writtenBytes <= 0 {
		t.Errorf("Expected positive bytes written, got %d", writtenBytes)
	}

	t.Logf("Wrote %d bytes", writtenBytes)

	// 读取数据包
	readPacket, readBytes, err := readStream.ReadPacket()
	if err != nil {
		t.Fatalf("Failed to read packet: %v", err)
	}
	if readBytes <= 0 {
		t.Errorf("Expected positive bytes read, got %d", readBytes)
	}

	// 验证数据包内容
	if readPacket.CommandPacket == nil {
		t.Fatal("Expected non-nil CommandPacket")
	}
	if readPacket.CommandPacket.CommandType != testPacket.CommandPacket.CommandType {
		t.Errorf("CommandType mismatch: expected %v, got %v", testPacket.CommandPacket.CommandType, readPacket.CommandPacket.CommandType)
	}
	if readPacket.CommandPacket.Token != testPacket.CommandPacket.Token {
		t.Errorf("Token mismatch: expected %s, got %s", testPacket.CommandPacket.Token, readPacket.CommandPacket.Token)
	}
	if readPacket.CommandPacket.CommandBody != testPacket.CommandPacket.CommandBody {
		t.Errorf("CommandBody mismatch: expected %s, got %s", testPacket.CommandPacket.CommandBody, readPacket.CommandPacket.CommandBody)
	}

	t.Logf("Read %d bytes successfully", readBytes)
	t.Log("Basic packet test completed successfully")
}

// ==================== 压缩数据包读写测试 ====================

func TestPackageStream_CompressedPacketReadWrite(t *testing.T) {
	// 设置10秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer

	// 创建写入和读取流
	writeStream := io2.NewStreamProcessor(nil, &buf, ctx)
	defer writeStream.Close()

	readStream := io2.NewStreamProcessor(&buf, nil, ctx)
	defer readStream.Close()

	// 创建包含大量重复数据的命令包（便于压缩）
	largeBody := ""
	for i := 0; i < 1000; i++ {
		largeBody += "This is repeated data for compression testing. "
	}

	commandPacket := &packet.CommandPacket{
		CommandType: packet.TcpMap,
		Token:       "compression-test-token",
		SenderId:    "sender-002",
		ReceiverId:  "receiver-002",
		CommandBody: largeBody,
	}

	testPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: commandPacket,
	}

	t.Log("Starting compressed packet read/write test...")

	// 写入数据包（启用压缩）
	writtenBytes, err := writeStream.WritePacket(testPacket, true, 0)
	if err != nil {
		t.Fatalf("Failed to write compressed packet: %v", err)
	}
	if writtenBytes <= 0 {
		t.Errorf("Expected positive bytes written, got %d", writtenBytes)
	}

	t.Logf("Wrote %d bytes (compressed)", writtenBytes)

	// 读取数据包
	readPacket, readBytes, err := readStream.ReadPacket()
	if err != nil {
		t.Fatalf("Failed to read compressed packet: %v", err)
	}
	if readBytes <= 0 {
		t.Errorf("Expected positive bytes read, got %d", readBytes)
	}

	// 验证数据包内容
	if readPacket.CommandPacket == nil {
		t.Fatal("Expected non-nil CommandPacket")
	}
	if readPacket.CommandPacket.CommandBody != testPacket.CommandPacket.CommandBody {
		t.Errorf("CommandBody mismatch after compression/decompression")
	}

	// 验证压缩效果
	originalSize := len(largeBody)
	compressedSize := writtenBytes - 5 // 减去包类型(1)和大小字段(4)
	compressionRatio := float64(compressedSize) / float64(originalSize)

	t.Logf("Original size: %d bytes", originalSize)
	t.Logf("Compressed size: %d bytes", compressedSize)
	t.Logf("Compression ratio: %.2f%%", compressionRatio*100)

	if compressionRatio >= 1.0 {
		t.Logf("Warning: No compression achieved (ratio: %.2f)", compressionRatio)
	} else {
		t.Logf("Compression achieved: %.1f%% reduction", (1-compressionRatio)*100)
	}

	t.Logf("Read %d bytes successfully", readBytes)
	t.Log("Compressed packet test completed successfully")
}

// ==================== 限速数据包读写测试 ====================

func TestPackageStream_RateLimitedPacketReadWrite(t *testing.T) {
	// 设置15秒超时（限速需要更多时间）
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var buf bytes.Buffer

	// 创建写入和读取流
	writeStream := io2.NewStreamProcessor(nil, &buf, ctx)
	defer writeStream.Close()

	readStream := io2.NewStreamProcessor(&buf, nil, ctx)
	defer readStream.Close()

	// 创建测试数据包
	commandPacket := &packet.CommandPacket{
		CommandType: packet.TcpMap,
		Token:       "rate-limit-test-token",
		SenderId:    "sender-003",
		ReceiverId:  "receiver-003",
		CommandBody: "Rate limited packet test data",
	}

	testPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: commandPacket,
	}

	t.Log("Starting rate limited packet read/write test...")

	// 记录开始时间
	startTime := time.Now()

	// 写入数据包（启用限速：1KB/s）
	writtenBytes, err := writeStream.WritePacket(testPacket, false, 1024)
	if err != nil {
		t.Fatalf("Failed to write rate limited packet: %v", err)
	}
	if writtenBytes <= 0 {
		t.Errorf("Expected positive bytes written, got %d", writtenBytes)
	}

	// 计算实际写入时间
	writeDuration := time.Since(startTime)
	expectedDuration := time.Duration(writtenBytes) * time.Second / 1024

	t.Logf("Wrote %d bytes in %v", writtenBytes, writeDuration)
	t.Logf("Expected duration: %v", expectedDuration)

	// 验证限速效果（实际时间应该接近或大于预期时间）
	if writeDuration < expectedDuration*8/10 { // 允许10%的误差
		t.Logf("Warning: Rate limiting may not be working properly")
	}

	// 读取数据包
	readPacket, readBytes, err := readStream.ReadPacket()
	if err != nil {
		t.Fatalf("Failed to read rate limited packet: %v", err)
	}
	if readBytes <= 0 {
		t.Errorf("Expected positive bytes read, got %d", readBytes)
	}

	// 验证数据包内容
	if readPacket.CommandPacket == nil {
		t.Fatal("Expected non-nil CommandPacket")
	}
	if readPacket.CommandPacket.CommandBody != testPacket.CommandPacket.CommandBody {
		t.Errorf("CommandBody mismatch after rate limiting")
	}

	t.Logf("Read %d bytes successfully", readBytes)
	t.Log("Rate limited packet test completed successfully")
}

// ==================== 压缩+限速组合测试 ====================

func TestPackageStream_CompressedAndRateLimitedPacketReadWrite(t *testing.T) {
	// 设置20秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var buf bytes.Buffer

	// 创建写入和读取流
	writeStream := io2.NewStreamProcessor(nil, &buf, ctx)
	defer writeStream.Close()

	readStream := io2.NewStreamProcessor(&buf, nil, ctx)
	defer readStream.Close()

	// 创建包含大量重复数据的命令包
	largeBody := ""
	for i := 0; i < 2000; i++ {
		largeBody += "This is repeated data for compression and rate limiting testing. "
	}

	commandPacket := &packet.CommandPacket{
		CommandType: packet.TcpMap,
		Token:       "compression-rate-limit-test-token",
		SenderId:    "sender-004",
		ReceiverId:  "receiver-004",
		CommandBody: largeBody,
	}

	testPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: commandPacket,
	}

	t.Log("Starting compressed and rate limited packet read/write test...")

	// 记录开始时间
	startTime := time.Now()

	// 写入数据包（启用压缩和限速：2KB/s）
	writtenBytes, err := writeStream.WritePacket(testPacket, true, 2048)
	if err != nil {
		t.Fatalf("Failed to write compressed and rate limited packet: %v", err)
	}
	if writtenBytes <= 0 {
		t.Errorf("Expected positive bytes written, got %d", writtenBytes)
	}

	// 计算实际写入时间
	writeDuration := time.Since(startTime)
	expectedDuration := time.Duration(writtenBytes) * time.Second / 2048

	t.Logf("Wrote %d bytes in %v", writtenBytes, writeDuration)
	t.Logf("Expected duration: %v", expectedDuration)

	// 验证限速效果
	if writeDuration < expectedDuration*8/10 {
		t.Logf("Warning: Rate limiting may not be working properly")
	}

	// 读取数据包
	readPacket, readBytes, err := readStream.ReadPacket()
	if err != nil {
		t.Fatalf("Failed to read compressed and rate limited packet: %v", err)
	}
	if readBytes <= 0 {
		t.Errorf("Expected positive bytes read, got %d", readBytes)
	}

	// 验证数据包内容
	if readPacket.CommandPacket == nil {
		t.Fatal("Expected non-nil CommandPacket")
	}
	if readPacket.CommandPacket.CommandBody != testPacket.CommandPacket.CommandBody {
		t.Errorf("CommandBody mismatch after compression and rate limiting")
	}

	// 验证压缩效果
	originalSize := len(largeBody)
	compressedSize := writtenBytes - 5 // 减去包类型(1)和大小字段(4)
	compressionRatio := float64(compressedSize) / float64(originalSize)

	t.Logf("Original size: %d bytes", originalSize)
	t.Logf("Compressed size: %d bytes", compressedSize)
	t.Logf("Compression ratio: %.2f%%", compressionRatio*100)

	t.Logf("Read %d bytes successfully", readBytes)
	t.Log("Compressed and rate limited packet test completed successfully")
}

// ==================== 大数据包测试 ====================

func TestPackageStream_LargePacketReadWrite(t *testing.T) {
	// 设置30秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer

	// 创建写入和读取流
	writeStream := io2.NewStreamProcessor(nil, &buf, ctx)
	defer writeStream.Close()

	readStream := io2.NewStreamProcessor(&buf, nil, ctx)
	defer readStream.Close()

	// 创建大数据包（100KB数据，避免JSON序列化问题）
	largeData := make([]byte, 100*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	// base64编码
	largeDataB64 := base64.StdEncoding.EncodeToString(largeData)

	commandPacket := &packet.CommandPacket{
		CommandType: packet.TcpMap,
		Token:       "large-packet-test-token",
		SenderId:    "sender-005",
		ReceiverId:  "receiver-005",
		CommandBody: largeDataB64,
	}

	testPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: commandPacket,
	}

	t.Log("Starting large packet read/write test...")

	// 写入大数据包（启用压缩）
	writtenBytes, err := writeStream.WritePacket(testPacket, true, 0)
	if err != nil {
		t.Fatalf("Failed to write large packet: %v", err)
	}
	if writtenBytes <= 0 {
		t.Errorf("Expected positive bytes written, got %d", writtenBytes)
	}

	t.Logf("Wrote %d bytes (large packet)", writtenBytes)

	// 读取大数据包
	readPacket, readBytes, err := readStream.ReadPacket()
	if err != nil {
		t.Fatalf("Failed to read large packet: %v", err)
	}
	if readBytes <= 0 {
		t.Errorf("Expected positive bytes read, got %d", readBytes)
	}

	// 验证数据包内容
	if readPacket.CommandPacket == nil {
		t.Fatal("Expected non-nil CommandPacket")
	}
	// base64解码后比对原始数据
	decoded, err := base64.StdEncoding.DecodeString(readPacket.CommandPacket.CommandBody)
	if err != nil {
		t.Fatalf("Failed to decode base64 CommandBody: %v", err)
	}
	if !bytes.Equal(decoded, largeData) {
		t.Errorf("Large packet CommandBody mismatch after base64 decode")
	}

	// 验证压缩效果
	originalSize := len(largeData)
	compressedSize := writtenBytes - 5
	compressionRatio := float64(compressedSize) / float64(originalSize)

	t.Logf("Original size: %d bytes", originalSize)
	t.Logf("Compressed size: %d bytes", compressedSize)
	t.Logf("Compression ratio: %.2f%%", compressionRatio*100)

	t.Logf("Read %d bytes successfully", readBytes)
	t.Log("Large packet test completed successfully")
}

// ==================== 多数据包连续读写测试 ====================

func TestPackageStream_MultiplePacketsReadWrite(t *testing.T) {
	// 设置15秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var buf bytes.Buffer

	// 创建写入和读取流
	writeStream := io2.NewStreamProcessor(nil, &buf, ctx)
	defer writeStream.Close()

	readStream := io2.NewStreamProcessor(&buf, nil, ctx)
	defer readStream.Close()

	// 创建多个测试数据包
	testPackets := []*packet.TransferPacket{
		{
			PacketType:    packet.Heartbeat,
			CommandPacket: nil,
		},
		{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.TcpMap,
				Token:       "multi-packet-1",
				SenderId:    "sender-006",
				ReceiverId:  "receiver-006",
				CommandBody: "First packet data",
			},
		},
		{
			PacketType: packet.JsonCommand | packet.Compressed,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.TcpMap,
				Token:       "multi-packet-2",
				SenderId:    "sender-006",
				ReceiverId:  "receiver-006",
				CommandBody: "Second packet data with compression",
			},
		},
		{
			PacketType:    packet.Heartbeat,
			CommandPacket: nil,
		},
	}

	t.Log("Starting multiple packets read/write test...")

	// 写入多个数据包
	totalWritten := 0
	for i, pkt := range testPackets {
		useCompression := pkt.PacketType.IsCompressed()
		writtenBytes, err := writeStream.WritePacket(pkt, useCompression, 0)
		if err != nil {
			t.Fatalf("Failed to write packet %d: %v", i+1, err)
		}
		totalWritten += writtenBytes
		t.Logf("Wrote packet %d: %d bytes", i+1, writtenBytes)
	}

	t.Logf("Total written: %d bytes", totalWritten)

	// 读取多个数据包
	for i := 0; i < len(testPackets); i++ {
		readPacket, readBytes, err := readStream.ReadPacket()
		if err != nil {
			t.Fatalf("Failed to read packet %d: %v", i+1, err)
		}

		expectedPacket := testPackets[i]

		// 验证包类型
		if readPacket.PacketType != expectedPacket.PacketType {
			t.Errorf("Packet %d type mismatch: expected %v, got %v", i+1, expectedPacket.PacketType, readPacket.PacketType)
		}

		// 验证心跳包
		if expectedPacket.PacketType.IsHeartbeat() {
			if readPacket.CommandPacket != nil {
				t.Errorf("Packet %d: Expected nil CommandPacket for heartbeat", i+1)
			}
		} else {
			// 验证命令包
			if readPacket.CommandPacket == nil {
				t.Errorf("Packet %d: Expected non-nil CommandPacket", i+1)
			} else if readPacket.CommandPacket.CommandBody != expectedPacket.CommandPacket.CommandBody {
				t.Errorf("Packet %d CommandBody mismatch", i+1)
			}
		}

		t.Logf("Read packet %d: %d bytes", i+1, readBytes)
	}

	t.Log("Multiple packets test completed successfully")
}

// ==================== 错误情况测试 ====================

func TestPackageStream_ErrorConditions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf bytes.Buffer

	// 测试写入nil数据包
	writeStream := io2.NewStreamProcessor(nil, &buf, ctx)
	defer writeStream.Close()

	_, err := writeStream.WritePacket(nil, false, 0)
	if err == nil {
		t.Error("Expected error when writing nil packet")
	}

	// 测试读取空缓冲区
	readStream := io2.NewStreamProcessor(&buf, nil, ctx)
	defer readStream.Close()

	_, _, err = readStream.ReadPacket()
	if err == nil {
		t.Error("Expected error when reading from empty buffer")
	}

	t.Log("Error conditions test completed")
}
