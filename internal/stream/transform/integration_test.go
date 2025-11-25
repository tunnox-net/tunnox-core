package transform

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
	"tunnox-core/internal/stream/encryption"
)

// TestEndToEndTransparentTransmission 测试端到端透明传输
// 模拟: ClientA -> ServerA -> ServerB -> ClientB 的场景
func TestEndToEndTransparentTransmission(t *testing.T) {
	// 生成加密密钥
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil{
		t.Fatalf("Failed to create transformer: %v", err)
	}

	// 准备测试数据
	originalData := bytes.Repeat([]byte("This is test data for end-to-end transparent transmission. "), 100)

	// 模拟 ClientA 发送数据 -> ServerA (压缩+加密)
	var serverABuf bytes.Buffer
	clientAWriter, err := transformer.WrapWriter(&serverABuf)
	if err != nil {
		t.Fatalf("Failed to create ClientA writer: %v", err)
	}

	_, err = clientAWriter.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write from ClientA: %v", err)
	}
	clientAWriter.Close()

	transmittedData := serverABuf.Bytes()
	t.Logf("Original size: %d, Transmitted size: %d, Ratio: %.2f%%",
		len(originalData), len(transmittedData), float64(len(transmittedData))/float64(len(originalData))*100)

	// 模拟 ServerB 接收数据 -> ClientB (解密+解压)
	serverBReader, err := transformer.WrapReader(bytes.NewReader(transmittedData))
	if err != nil {
		t.Fatalf("Failed to create ServerB reader: %v", err)
	}

	var clientBBuf bytes.Buffer
	_, err = io.Copy(&clientBBuf, serverBReader)
	if err != nil {
		t.Fatalf("Failed to read at ClientB: %v", err)
	}

	// 验证数据完整性
	if !bytes.Equal(clientBBuf.Bytes(), originalData) {
		t.Errorf("End-to-end transmission failed: data mismatch")
	}

	t.Logf("✅ End-to-end transmission successful: %d bytes transmitted and verified", len(originalData))
}

// TestBidirectionalTransmission 测试双向透明传输
func TestBidirectionalTransmission(t *testing.T) {
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	// A -> B 方向
	dataAtoB := []byte("Request from A to B")
	var bufAtoB bytes.Buffer
	writerA, _ := transformer.WrapWriter(&bufAtoB)
	writerA.Write(dataAtoB)
	writerA.Close()

	readerB, _ := transformer.WrapReader(bytes.NewReader(bufAtoB.Bytes()))
	var resultAtoB bytes.Buffer
	io.Copy(&resultAtoB, readerB)

	if !bytes.Equal(resultAtoB.Bytes(), dataAtoB) {
		t.Errorf("A->B transmission failed")
	}

	// B -> A 方向
	dataBtoA := []byte("Response from B to A")
	var bufBtoA bytes.Buffer
	writerB, _ := transformer.WrapWriter(&bufBtoA)
	writerB.Write(dataBtoA)
	writerB.Close()

	readerA, _ := transformer.WrapReader(bytes.NewReader(bufBtoA.Bytes()))
	var resultBtoA bytes.Buffer
	io.Copy(&resultBtoA, readerA)

	if !bytes.Equal(resultBtoA.Bytes(), dataBtoA) {
		t.Errorf("B->A transmission failed")
	}

	t.Logf("✅ Bidirectional transmission successful")
}

// TestStreamingTransmission 测试流式传输
func TestStreamingTransmission(t *testing.T) {
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	// 使用管道模拟流式传输
	pr, pw := io.Pipe()

	// 发送端 goroutine
	go func() {
		defer pw.Close()
		
		writer, err := transformer.WrapWriter(pw)
		if err != nil {
			t.Errorf("Failed to create writer: %v", err)
			return
		}
		defer writer.Close()

		// 分多次发送数据
		for i := 0; i < 10; i++ {
			data := []byte(fmt.Sprintf("Chunk %d: streaming test data\n", i))
			_, err := writer.Write(data)
			if err != nil {
				t.Errorf("Failed to write chunk %d: %v", i, err)
				return
			}
			time.Sleep(10 * time.Millisecond) // 模拟网络延迟
		}
	}()

	// 接收端
	reader, err := transformer.WrapReader(pr)
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	var receivedBuf bytes.Buffer
	_, err = io.Copy(&receivedBuf, reader)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	// 验证接收到的数据
	receivedLines := bytes.Split(bytes.TrimSpace(receivedBuf.Bytes()), []byte("\n"))
	if len(receivedLines) != 10 {
		t.Errorf("Expected 10 lines, got %d", len(receivedLines))
	}

	t.Logf("✅ Streaming transmission successful: received %d chunks", len(receivedLines))
}

// TestTCPTransmission 测试 TCP 透明传输
func TestTCPTransmission(t *testing.T) {
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	// 启动 TCP 服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()
	t.Logf("Server listening on %s", serverAddr)

	// 服务器端 goroutine
	serverDone := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("Failed to accept connection: %v", err)
			return
		}
		defer conn.Close()

		// 解密+解压
		reader, err := transformer.WrapReader(conn)
		if err != nil {
			t.Errorf("Failed to create server reader: %v", err)
			return
		}

		var buf bytes.Buffer
		io.Copy(&buf, reader)
		serverDone <- buf.Bytes()
	}()

	// 客户端
	time.Sleep(100 * time.Millisecond) // 等待服务器启动

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// 压缩+加密
	writer, err := transformer.WrapWriter(conn)
	if err != nil {
		t.Fatalf("Failed to create client writer: %v", err)
	}

	originalData := []byte("TCP transmission test data with compression and encryption!")
	_, err = writer.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	writer.Close()
	conn.Close()

	// 等待服务器接收完成
	select {
	case receivedData := <-serverDone:
		if !bytes.Equal(receivedData, originalData) {
			t.Errorf("TCP transmission failed: data mismatch")
		}
		t.Logf("✅ TCP transmission successful: %d bytes", len(receivedData))
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for server to receive data")
	}
}

// TestConcurrentTransmissions 测试并发传输
func TestConcurrentTransmissions(t *testing.T) {
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	// 并发传输 10 个连接
	concurrency := 10
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer func() { done <- true }()

			originalData := []byte(fmt.Sprintf("Concurrent transmission %d: test data", id))

			// 加密+压缩
			var buf bytes.Buffer
			writer, err := transformer.WrapWriter(&buf)
			if err != nil {
				t.Errorf("Connection %d: failed to create writer: %v", id, err)
				return
			}

			writer.Write(originalData)
			writer.Close()

			// 解密+解压
			reader, err := transformer.WrapReader(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Errorf("Connection %d: failed to create reader: %v", id, err)
				return
			}

			var result bytes.Buffer
			io.Copy(&result, reader)

			if !bytes.Equal(result.Bytes(), originalData) {
				t.Errorf("Connection %d: data mismatch", id)
			}
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < concurrency; i++ {
		select {
		case <-done:
			// OK
		case <-time.After(10 * time.Second):
			t.Fatalf("Timeout waiting for concurrent transmission %d", i)
		}
	}

	t.Logf("✅ Concurrent transmissions successful: %d connections", concurrency)
}

// TestDifferentDataSizes 测试不同大小的数据
func TestDifferentDataSizes(t *testing.T) {
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	testSizes := []int{
		1,           // 1 byte
		100,         // 100 bytes
		1024,        // 1KB
		10 * 1024,   // 10KB
		100 * 1024,  // 100KB
		1024 * 1024, // 1MB
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			originalData := make([]byte, size)
			for i := range originalData {
				originalData[i] = byte(i % 256)
			}

			// 转换
			var buf bytes.Buffer
			writer, err := transformer.WrapWriter(&buf)
			if err != nil {
				t.Fatalf("Failed to create writer: %v", err)
			}

			writer.Write(originalData)
			writer.Close()

			// 还原
			reader, err := transformer.WrapReader(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("Failed to create reader: %v", err)
			}

			var result bytes.Buffer
			io.Copy(&result, reader)

			// 验证
			if !bytes.Equal(result.Bytes(), originalData) {
				t.Errorf("Data mismatch for size %d", size)
			}

			t.Logf("Size %d: Original=%d, Transmitted=%d, Ratio=%.2f%%",
				size, len(originalData), buf.Len(), float64(buf.Len())/float64(len(originalData))*100)
		})
	}
}

// TestErrorRecovery 测试错误恢复
func TestErrorRecovery(t *testing.T) {
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	// 正常传输
	originalData := []byte("Test data before error")
	var buf bytes.Buffer
	writer, _ := transformer.WrapWriter(&buf)
	writer.Write(originalData)
	writer.Close()

	// 损坏部分数据
	corruptedData := buf.Bytes()
	if len(corruptedData) > 20 {
		corruptedData[len(corruptedData)-10] ^= 0xFF
	}

	// 尝试解密应该失败
	reader, err := transformer.WrapReader(bytes.NewReader(corruptedData))
	
	// 可能在创建 reader 时就失败（解密第一块时）
	if err != nil {
		t.Logf("✅ Error detection successful: corrupted data rejected at reader creation: %v", err)
		return
	}

	// 或者在读取时失败
	var result bytes.Buffer
	_, err = io.Copy(&result, reader)
	if err == nil {
		t.Error("Expected error when reading corrupted data")
		return
	}

	t.Logf("✅ Error detection successful: corrupted data rejected during read: %v", err)
}

