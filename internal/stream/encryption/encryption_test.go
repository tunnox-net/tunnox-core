package encryption

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// TestEncryptionManager 测试加密管理器
func TestEncryptionManager(t *testing.T) {
	ctx := context.Background()

	// 生成测试密钥
	key := make([]byte, 32) // AES-256
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// 创建静态密钥
	staticKey := stream.NewStaticKey(key, "test-key-1")

	// 创建加密管理器
	encMgr := stream.NewEncryptionManager(staticKey, ctx)
	defer encMgr.Close()

	// 测试数据
	testData := []byte("Hello, World! This is a test message for encryption.")

	// 测试加密
	encryptedData, err := encMgr.EncryptData(testData)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// 验证加密后的数据长度
	if len(encryptedData) <= len(testData) {
		t.Errorf("Encrypted data should be longer than original data")
	}

	// 验证加密后的数据不等于原始数据
	if bytes.Equal(encryptedData, testData) {
		t.Errorf("Encrypted data should not equal original data")
	}

	// 测试解密
	decryptedData, err := encMgr.DecryptData(encryptedData)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	// 验证解密后的数据等于原始数据
	if !bytes.Equal(decryptedData, testData) {
		t.Errorf("Decrypted data does not match original data")
		t.Errorf("Original: %s", string(testData))
		t.Errorf("Decrypted: %s", string(decryptedData))
	}

	// 测试空数据
	emptyEncrypted, err := encMgr.EncryptData([]byte{})
	if err != nil {
		t.Fatalf("Empty data encryption failed: %v", err)
	}

	emptyDecrypted, err := encMgr.DecryptData(emptyEncrypted)
	if err != nil {
		t.Fatalf("Empty data decryption failed: %v", err)
	}

	if len(emptyDecrypted) != 0 {
		t.Errorf("Empty data decryption should return empty data")
	}
}

// TestEncryptionReader 测试加密读取器
func TestEncryptionReader(t *testing.T) {
	ctx := context.Background()

	// 生成测试密钥
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	staticKey := stream.NewStaticKey(key, "test-key-2")

	// 测试数据
	testData := []byte("This is a test message for encryption reader.")

	// 创建加密管理器来加密数据
	encMgr := stream.NewEncryptionManager(staticKey, ctx)
	encryptedData, err := encMgr.EncryptData(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt test data: %v", err)
	}

	// 创建包含长度信息的完整数据
	var buffer bytes.Buffer
	lengthBytes := make([]byte, 4)
	lengthBytes[0] = byte(len(encryptedData) >> 24)
	lengthBytes[1] = byte(len(encryptedData) >> 16)
	lengthBytes[2] = byte(len(encryptedData) >> 8)
	lengthBytes[3] = byte(len(encryptedData))
	buffer.Write(lengthBytes)
	buffer.Write(encryptedData)

	// 创建加密读取器
	reader := stream.NewEncryptionReader(&buffer, staticKey, ctx)
	defer reader.Close()

	// 读取并解密数据
	decryptedData, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read and decrypt data: %v", err)
	}

	// 验证解密后的数据
	if !bytes.Equal(decryptedData, testData) {
		t.Errorf("Decrypted data does not match original data")
		t.Errorf("Original: %s", string(testData))
		t.Errorf("Decrypted: %s", string(decryptedData))
	}
}

// TestEncryptionWriter 测试加密写入器
func TestEncryptionWriter(t *testing.T) {
	ctx := context.Background()

	// 生成测试密钥
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	staticKey := stream.NewStaticKey(key, "test-key-3")

	// 测试数据
	testData := []byte("This is a test message for encryption writer.")

	// 创建缓冲区
	var buffer bytes.Buffer

	// 创建加密写入器
	writer := stream.NewEncryptionWriter(&buffer, staticKey, ctx)
	defer writer.Close()

	// 写入并加密数据
	err = writer.WriteAll(testData)
	if err != nil {
		t.Fatalf("Failed to write and encrypt data: %v", err)
	}

	// 获取加密后的数据
	encryptedBytes := buffer.Bytes()

	// 验证加密后的数据长度
	if len(encryptedBytes) <= len(testData)+4 { // +4 for length prefix
		t.Errorf("Encrypted data should be longer than original data + length prefix")
	}

	// 创建加密管理器来解密数据
	encMgr := stream.NewEncryptionManager(staticKey, ctx)

	// 解析长度前缀
	if len(encryptedBytes) < 4 {
		t.Fatalf("Encrypted data too short")
	}

	length := int(encryptedBytes[0])<<24 | int(encryptedBytes[1])<<16 | int(encryptedBytes[2])<<8 | int(encryptedBytes[3])
	encryptedData := encryptedBytes[4 : 4+length]

	// 解密数据
	decryptedData, err := encMgr.DecryptData(encryptedData)
	if err != nil {
		t.Fatalf("Failed to decrypt data: %v", err)
	}

	// 验证解密后的数据
	if !bytes.Equal(decryptedData, testData) {
		t.Errorf("Decrypted data does not match original data")
		t.Errorf("Original: %s", string(testData))
		t.Errorf("Decrypted: %s", string(decryptedData))
	}
}

// TestStreamProcessorWithEncryption 测试带加密的流处理器
func TestStreamProcessorWithEncryption(t *testing.T) {
	ctx := context.Background()

	// 生成测试密钥
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	staticKey := stream.NewStaticKey(key, "test-key-4")

	// 创建写入缓冲区
	var writeBuffer bytes.Buffer

	// 创建带加密的写入流处理器
	writeProcessor := stream.NewStreamProcessorWithEncryption(&writeBuffer, &writeBuffer, staticKey, ctx)
	defer writeProcessor.Close()

	// 创建测试数据包
	testPacket := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.TcpMap,
			CommandId:   "test-command-1",
			Token:       "test-token",
			SenderId:    "sender-1",
			ReceiverId:  "receiver-1",
			CommandBody: `{"port": 8080, "host": "localhost"}`,
		},
	}

	// 写入数据包（启用压缩和加密）
	writtenBytes, err := writeProcessor.WritePacket(testPacket, true, 0)
	if err != nil {
		t.Fatalf("Failed to write packet: %v", err)
	}

	t.Logf("Written bytes: %d", writtenBytes)

	// 创建读取缓冲区，复制写入的数据
	var readBuffer bytes.Buffer
	readBuffer.Write(writeBuffer.Bytes())

	// 创建带加密的读取流处理器
	readProcessor := stream.NewStreamProcessorWithEncryption(&readBuffer, &readBuffer, staticKey, ctx)
	defer readProcessor.Close()

	// 读取数据包
	readPacket, readBytes, err := readProcessor.ReadPacket()
	if err != nil {
		t.Fatalf("Failed to read packet: %v", err)
	}

	t.Logf("Read bytes: %d", readBytes)

	// 验证读取的数据包
	if readPacket == nil {
		t.Fatalf("Read packet is nil")
	}

	if readPacket.PacketType != packet.JsonCommand|packet.Compressed|packet.Encrypted {
		t.Errorf("Expected packet type with compression and encryption flags")
	}

	if readPacket.CommandPacket == nil {
		t.Fatalf("Command packet is nil")
	}

	if readPacket.CommandPacket.CommandType != packet.TcpMap {
		t.Errorf("Expected command type TcpMap, got %v", readPacket.CommandPacket.CommandType)
	}

	if readPacket.CommandPacket.CommandId != "test-command-1" {
		t.Errorf("Expected command ID 'test-command-1', got '%s'", readPacket.CommandPacket.CommandId)
	}

	// 验证命令体
	var commandBody map[string]interface{}
	err = json.Unmarshal([]byte(readPacket.CommandPacket.CommandBody), &commandBody)
	if err != nil {
		t.Fatalf("Failed to unmarshal command body: %v", err)
	}

	if commandBody["port"] != float64(8080) {
		t.Errorf("Expected port 8080, got %v", commandBody["port"])
	}

	if commandBody["host"] != "localhost" {
		t.Errorf("Expected host 'localhost', got '%v'", commandBody["host"])
	}
}

// TestStreamProcessorEncryptionToggle 测试流处理器加密开关
func TestStreamProcessorEncryptionToggle(t *testing.T) {
	ctx := context.Background()

	// 生成测试密钥
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	staticKey := stream.NewStaticKey(key, "test-key-5")

	// 创建内存缓冲区
	var buffer bytes.Buffer

	// 创建不带加密的流处理器
	processor := stream.NewStreamProcessor(&buffer, &buffer, ctx)
	defer processor.Close()

	// 验证初始状态
	if processor.IsEncryptionEnabled() {
		t.Errorf("Encryption should be disabled initially")
	}

	// 启用加密
	processor.EnableEncryption(staticKey)

	if !processor.IsEncryptionEnabled() {
		t.Errorf("Encryption should be enabled after EnableEncryption")
	}

	if processor.GetEncryptionKey() != staticKey {
		t.Errorf("Encryption key should match the provided key")
	}

	// 禁用加密
	processor.DisableEncryption()

	if processor.IsEncryptionEnabled() {
		t.Errorf("Encryption should be disabled after DisableEncryption")
	}

	if processor.GetEncryptionKey() != nil {
		t.Errorf("Encryption key should be nil after DisableEncryption")
	}
}

// TestStreamFactoryWithEncryption 测试带加密的流工厂
func TestStreamFactoryWithEncryption(t *testing.T) {
	ctx := context.Background()

	// 生成测试密钥
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	staticKey := stream.NewStaticKey(key, "test-key-6")

	// 创建带加密配置的流工厂
	config := stream.StreamFactoryConfig{
		DefaultCompression: true,
		DefaultRateLimit:   1024,
		BufferSize:         4096,
		EnableMemoryPool:   true,
		EncryptionKey:      staticKey,
	}

	factory := stream.NewConfigurableStreamFactory(ctx, config)

	// 创建内存缓冲区
	var buffer bytes.Buffer

	// 通过工厂创建流处理器
	processor := factory.NewStreamProcessor(&buffer, &buffer)
	defer processor.Close()

	// 验证加密已启用（需要类型断言）
	if streamProcessor, ok := processor.(*stream.StreamProcessor); ok {
		if !streamProcessor.IsEncryptionEnabled() {
			t.Errorf("Stream processor should have encryption enabled")
		}
	} else {
		t.Errorf("Failed to cast to StreamProcessor")
	}

	// 测试加密管理器创建
	encMgr := factory.NewEncryptionManager()
	if encMgr == nil {
		t.Errorf("Encryption manager should not be nil")
	}

	if encMgr.GetKey() != staticKey {
		t.Errorf("Encryption manager key should match the provided key")
	}

	// 测试加密读取器创建
	encReader := factory.NewEncryptionReader(&buffer)
	if encReader == nil {
		t.Errorf("Encryption reader should not be nil")
	}

	// 测试加密写入器创建
	encWriter := factory.NewEncryptionWriter(&buffer)
	if encWriter == nil {
		t.Errorf("Encryption writer should not be nil")
	}
}

// TestEncryptionErrorHandling 测试加密错误处理
func TestEncryptionErrorHandling(t *testing.T) {
	ctx := context.Background()

	// 测试无效密钥长度
	invalidKey := []byte("short-key") // 太短的密钥
	staticKey := stream.NewStaticKey(invalidKey, "invalid-key")

	encMgr := stream.NewEncryptionManager(staticKey, ctx)
	defer encMgr.Close()

	testData := []byte("test data")

	// 应该返回错误
	_, err := encMgr.EncryptData(testData)
	if err == nil {
		t.Errorf("Should return error for invalid key length")
	}

	// 测试解密无效数据
	invalidData := []byte("invalid encrypted data")
	_, err = encMgr.DecryptData(invalidData)
	if err == nil {
		t.Errorf("Should return error for invalid encrypted data")
	}

	// 测试空密钥
	nilKey := stream.NewStaticKey(nil, "nil-key")
	encMgr2 := stream.NewEncryptionManager(nilKey, ctx)
	defer encMgr2.Close()

	_, err = encMgr2.EncryptData(testData)
	if err == nil {
		t.Errorf("Should return error for nil key")
	}
}

// TestEncryptionPerformance 测试加密性能
func TestEncryptionPerformance(t *testing.T) {
	ctx := context.Background()

	// 生成测试密钥
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	staticKey := stream.NewStaticKey(key, "perf-key")
	encMgr := stream.NewEncryptionManager(staticKey, ctx)
	defer encMgr.Close()

	// 生成测试数据
	testData := make([]byte, 1024*1024) // 1MB
	_, err = rand.Read(testData)
	if err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}

	// 测试加密性能
	start := time.Now()
	encryptedData, err := encMgr.EncryptData(testData)
	encryptTime := time.Since(start)

	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	t.Logf("Encryption time for 1MB: %v", encryptTime)

	// 测试解密性能
	start = time.Now()
	decryptedData, err := encMgr.DecryptData(encryptedData)
	decryptTime := time.Since(start)

	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	t.Logf("Decryption time for 1MB: %v", decryptTime)

	// 验证数据完整性
	if !bytes.Equal(decryptedData, testData) {
		t.Errorf("Data integrity check failed")
	}

	// 性能基准：加密和解密都应该在合理时间内完成
	if encryptTime > 100*time.Millisecond {
		t.Logf("Warning: Encryption took longer than expected: %v", encryptTime)
	}

	if decryptTime > 100*time.Millisecond {
		t.Logf("Warning: Decryption took longer than expected: %v", decryptTime)
	}
}

// TestEncryptionConcurrency 测试加密并发性
func TestEncryptionConcurrency(t *testing.T) {
	ctx := context.Background()

	// 生成测试密钥
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	staticKey := stream.NewStaticKey(key, "concurrent-key")
	encMgr := stream.NewEncryptionManager(staticKey, ctx)
	defer encMgr.Close()

	// 并发测试数据
	testData := []byte("concurrent test data")
	numGoroutines := 10

	// 使用通道来收集结果
	results := make(chan error, numGoroutines)

	// 启动并发加密测试
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// 加密
			encrypted, err := encMgr.EncryptData(testData)
			if err != nil {
				results <- err
				return
			}

			// 解密
			decrypted, err := encMgr.DecryptData(encrypted)
			if err != nil {
				results <- err
				return
			}

			// 验证
			if !bytes.Equal(decrypted, testData) {
				results <- err
				return
			}

			results <- nil
		}(i)
	}

	// 收集结果
	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			t.Errorf("Concurrent encryption test failed: %v", err)
		}
	}
}

// TestEncryptionKeyManagement 测试密钥管理
func TestEncryptionKeyManagement(t *testing.T) {
	ctx := context.Background()

	// 生成两个不同的密钥
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	_, err := rand.Read(key1)
	if err != nil {
		t.Fatalf("Failed to generate key1: %v", err)
	}
	_, err = rand.Read(key2)
	if err != nil {
		t.Fatalf("Failed to generate key2: %v", err)
	}

	staticKey1 := stream.NewStaticKey(key1, "key-1")
	staticKey2 := stream.NewStaticKey(key2, "key-2")

	// 创建加密管理器
	encMgr := stream.NewEncryptionManager(staticKey1, ctx)
	defer encMgr.Close()

	// 测试数据
	testData := []byte("key management test data")

	// 使用第一个密钥加密
	encrypted1, err := encMgr.EncryptData(testData)
	if err != nil {
		t.Fatalf("Encryption with key1 failed: %v", err)
	}

	// 使用第一个密钥解密（应该成功）
	decrypted1, err := encMgr.DecryptData(encrypted1)
	if err != nil {
		t.Fatalf("Decryption with key1 failed: %v", err)
	}

	if !bytes.Equal(decrypted1, testData) {
		t.Errorf("Decryption with key1 failed data integrity check")
	}

	// 切换到第二个密钥
	encMgr.SetKey(staticKey2)

	// 使用第二个密钥加密
	encrypted2, err := encMgr.EncryptData(testData)
	if err != nil {
		t.Fatalf("Encryption with key2 failed: %v", err)
	}

	// 使用第二个密钥解密（应该成功）
	decrypted2, err := encMgr.DecryptData(encrypted2)
	if err != nil {
		t.Fatalf("Decryption with key2 failed: %v", err)
	}

	if !bytes.Equal(decrypted2, testData) {
		t.Errorf("Decryption with key2 failed data integrity check")
	}

	// 验证密钥已切换
	if encMgr.GetKey() != staticKey2 {
		t.Errorf("Key should be switched to key2")
	}

	// 使用第二个密钥解密第一个密钥加密的数据（应该失败）
	_, err = encMgr.DecryptData(encrypted1)
	if err == nil {
		t.Errorf("Should fail to decrypt data encrypted with different key")
	}
}
