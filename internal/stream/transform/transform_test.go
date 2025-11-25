package transform

import (
	"bytes"
	"io"
	"testing"
	"tunnox-core/internal/stream/encryption"
)

// TestNoOpTransformer 测试无操作转换器
func TestNoOpTransformer(t *testing.T) {
	transformer, err := NewTransformer(nil)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	originalData := []byte("Test data for no-op transformer")

	// Write
	var buf bytes.Buffer
	writer, err := transformer.WrapWriter(&buf)
	if err != nil {
		t.Fatalf("Failed to wrap writer: %v", err)
	}

	_, err = writer.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	writer.Close()

	// Read
	reader, err := transformer.WrapReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to wrap reader: %v", err)
	}

	var result bytes.Buffer
	_, err = io.Copy(&result, reader)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	// 验证
	if !bytes.Equal(result.Bytes(), originalData) {
		t.Errorf("NoOp transformer should not modify data")
	}
}

// TestCompressionOnly 测试仅压缩
func TestCompressionOnly(t *testing.T) {
	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  false,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	// 使用重复数据以获得更好的压缩率
	originalData := bytes.Repeat([]byte("This is test data for compression. "), 100)

	// 压缩
	var compressedBuf bytes.Buffer
	writer, err := transformer.WrapWriter(&compressedBuf)
	if err != nil {
		t.Fatalf("Failed to wrap writer: %v", err)
	}

	_, err = writer.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	writer.Close()

	compressedData := compressedBuf.Bytes()
	t.Logf("Original size: %d, Compressed size: %d, Ratio: %.2f%%",
		len(originalData), len(compressedData), float64(len(compressedData))/float64(len(originalData))*100)

	// 验证压缩有效
	if len(compressedData) >= len(originalData) {
		t.Logf("Warning: Compressed data is not smaller (may be expected for small data)")
	}

	// 解压
	reader, err := transformer.WrapReader(bytes.NewReader(compressedData))
	if err != nil {
		t.Fatalf("Failed to wrap reader: %v", err)
	}

	var decompressedBuf bytes.Buffer
	_, err = io.Copy(&decompressedBuf, reader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	// 验证数据完整性
	if !bytes.Equal(decompressedBuf.Bytes(), originalData) {
		t.Errorf("Decompressed data does not match original")
	}
}

// TestEncryptionOnly 测试仅加密
func TestEncryptionOnly(t *testing.T) {
	// 生成密钥
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: false,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	originalData := []byte("This is sensitive data that needs encryption!")

	// 加密
	var encryptedBuf bytes.Buffer
	writer, err := transformer.WrapWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to wrap writer: %v", err)
	}

	_, err = writer.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	writer.Close()

	encryptedData := encryptedBuf.Bytes()
	t.Logf("Original size: %d, Encrypted size: %d", len(originalData), len(encryptedData))

	// 验证数据已加密（不等于原始数据）
	if bytes.Equal(encryptedData, originalData) {
		t.Error("Data should be encrypted")
	}

	// 解密
	reader, err := transformer.WrapReader(bytes.NewReader(encryptedData))
	if err != nil {
		t.Fatalf("Failed to wrap reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	_, err = io.Copy(&decryptedBuf, reader)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// 验证数据完整性
	if !bytes.Equal(decryptedBuf.Bytes(), originalData) {
		t.Errorf("Decrypted data does not match original")
	}
}

// TestCompressionAndEncryption 测试压缩+加密
func TestCompressionAndEncryption(t *testing.T) {
	// 生成密钥
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

	// 使用重复数据以测试压缩+加密
	originalData := bytes.Repeat([]byte("Compressed and encrypted data. "), 100)

	// 压缩+加密
	var transformedBuf bytes.Buffer
	writer, err := transformer.WrapWriter(&transformedBuf)
	if err != nil {
		t.Fatalf("Failed to wrap writer: %v", err)
	}

	_, err = writer.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	writer.Close()

	transformedData := transformedBuf.Bytes()
	t.Logf("Original size: %d, Transformed size: %d, Ratio: %.2f%%",
		len(originalData), len(transformedData), float64(len(transformedData))/float64(len(originalData))*100)

	// 解密+解压
	reader, err := transformer.WrapReader(bytes.NewReader(transformedData))
	if err != nil {
		t.Fatalf("Failed to wrap reader: %v", err)
	}

	var restoredBuf bytes.Buffer
	_, err = io.Copy(&restoredBuf, reader)
	if err != nil {
		t.Fatalf("Failed to restore: %v", err)
	}

	// 验证数据完整性
	if !bytes.Equal(restoredBuf.Bytes(), originalData) {
		t.Errorf("Restored data does not match original")
	}
}

// TestChaCha20Encryption 测试 ChaCha20-Poly1305 加密
func TestChaCha20Encryption(t *testing.T) {
	key, err := encryption.GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	config := &TransformConfig{
		EnableCompression: false,
		EnableEncryption:  true,
		EncryptionMethod:  "chacha20-poly1305",
		EncryptionKey:     key,
	}

	transformer, err := NewTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	originalData := []byte("Testing ChaCha20-Poly1305 encryption!")

	// 加密
	var encryptedBuf bytes.Buffer
	writer, err := transformer.WrapWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to wrap writer: %v", err)
	}

	writer.Write(originalData)
	writer.Close()

	// 解密
	reader, err := transformer.WrapReader(bytes.NewReader(encryptedBuf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to wrap reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	io.Copy(&decryptedBuf, reader)

	// 验证
	if !bytes.Equal(decryptedBuf.Bytes(), originalData) {
		t.Errorf("ChaCha20 decryption failed")
	}
}

// TestLargeDataTransform 测试大数据转换
func TestLargeDataTransform(t *testing.T) {
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

	// 生成 1MB 测试数据
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// 转换
	var transformedBuf bytes.Buffer
	writer, err := transformer.WrapWriter(&transformedBuf)
	if err != nil {
		t.Fatalf("Failed to wrap writer: %v", err)
	}

	// 分块写入
	chunkSize := 10000
	for i := 0; i < len(largeData); i += chunkSize {
		end := i + chunkSize
		if end > len(largeData) {
			end = len(largeData)
		}
		_, err := writer.Write(largeData[i:end])
		if err != nil {
			t.Fatalf("Failed to write chunk: %v", err)
		}
	}
	writer.Close()

	transformedData := transformedBuf.Bytes()
	t.Logf("Original size: %d, Transformed size: %d, Ratio: %.2f%%",
		len(largeData), len(transformedData), float64(len(transformedData))/float64(len(largeData))*100)

	// 还原
	reader, err := transformer.WrapReader(bytes.NewReader(transformedData))
	if err != nil {
		t.Fatalf("Failed to wrap reader: %v", err)
	}

	var restoredBuf bytes.Buffer
	_, err = io.Copy(&restoredBuf, reader)
	if err != nil {
		t.Fatalf("Failed to restore: %v", err)
	}

	// 验证
	if !bytes.Equal(restoredBuf.Bytes(), largeData) {
		t.Errorf("Large data transformation failed")
	}
}

// TestMultipleChunksTransform 测试多块数据转换
func TestMultipleChunksTransform(t *testing.T) {
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

	// 多个数据块
	chunks := [][]byte{
		[]byte("First chunk of test data. "),
		[]byte("Second chunk of test data. "),
		[]byte("Third chunk of test data. "),
	}

	// 转换
	var transformedBuf bytes.Buffer
	writer, err := transformer.WrapWriter(&transformedBuf)
	if err != nil {
		t.Fatalf("Failed to wrap writer: %v", err)
	}

	for _, chunk := range chunks {
		_, err := writer.Write(chunk)
		if err != nil {
			t.Fatalf("Failed to write chunk: %v", err)
		}
	}
	writer.Close()

	// 还原
	reader, err := transformer.WrapReader(bytes.NewReader(transformedBuf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to wrap reader: %v", err)
	}

	var restoredBuf bytes.Buffer
	io.Copy(&restoredBuf, reader)

	// 验证
	expectedData := bytes.Join(chunks, nil)
	if !bytes.Equal(restoredBuf.Bytes(), expectedData) {
		t.Errorf("Multiple chunks transformation failed")
	}
}

// TestInvalidKey 测试无效密钥
func TestInvalidKey(t *testing.T) {
	config := &TransformConfig{
		EnableCompression: false,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     "invalid-key",
	}

	_, err := NewTransformer(config)
	if err == nil {
		t.Error("Expected error for invalid encryption key")
	}
}

// TestGenerateEncryptionKey 测试密钥生成
func TestGenerateEncryptionKey(t *testing.T) {
	key1, err := GenerateEncryptionKey("aes-256-gcm")
	if err != nil {
		t.Fatalf("Failed to generate AES key: %v", err)
	}

	key2, err := GenerateEncryptionKey("chacha20-poly1305")
	if err != nil {
		t.Fatalf("Failed to generate ChaCha20 key: %v", err)
	}

	// 两次生成的密钥应该不同
	if key1 == key2 {
		t.Error("Generated keys should be different")
	}

	// 验证密钥可以被使用
	config := &TransformConfig{
		EnableCompression: false,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key1,
	}

	_, err = NewTransformer(config)
	if err != nil {
		t.Errorf("Generated key should be valid: %v", err)
	}
}

// TestEmptyData 测试空数据
func TestEmptyData(t *testing.T) {
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

	// 转换空数据
	var transformedBuf bytes.Buffer
	writer, err := transformer.WrapWriter(&transformedBuf)
	if err != nil {
		t.Fatalf("Failed to wrap writer: %v", err)
	}
	writer.Close()

	// 还原
	reader, err := transformer.WrapReader(bytes.NewReader(transformedBuf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to wrap reader: %v", err)
	}

	var restoredBuf bytes.Buffer
	_, err = io.Copy(&restoredBuf, reader)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to restore: %v", err)
	}

	// 验证
	if restoredBuf.Len() != 0 {
		t.Errorf("Expected empty data, got %d bytes", restoredBuf.Len())
	}
}

// BenchmarkCompressionOnly 压缩性能基准测试
func BenchmarkCompressionOnly(b *testing.B) {
	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  false,
	}

	transformer, _ := NewTransformer(config)
	data := bytes.Repeat([]byte("benchmark data "), 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		writer, _ := transformer.WrapWriter(&buf)
		writer.Write(data)
		writer.Close()
	}
}

// BenchmarkEncryptionOnly 加密性能基准测试
func BenchmarkEncryptionOnly(b *testing.B) {
	key, _ := encryption.GenerateKeyBase64()
	config := &TransformConfig{
		EnableCompression: false,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, _ := NewTransformer(config)
	data := make([]byte, 64*1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		writer, _ := transformer.WrapWriter(&buf)
		writer.Write(data)
		writer.Close()
	}
}

// BenchmarkCompressionAndEncryption 压缩+加密性能基准测试
func BenchmarkCompressionAndEncryption(b *testing.B) {
	key, _ := encryption.GenerateKeyBase64()
	config := &TransformConfig{
		EnableCompression: true,
		CompressionLevel:  6,
		EnableEncryption:  true,
		EncryptionMethod:  "aes-256-gcm",
		EncryptionKey:     key,
	}

	transformer, _ := NewTransformer(config)
	data := bytes.Repeat([]byte("benchmark data "), 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		writer, _ := transformer.WrapWriter(&buf)
		writer.Write(data)
		writer.Close()
	}
}

