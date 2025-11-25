package encryption

import (
	"bytes"
	"io"
	"testing"
)

// TestAESGCMBasic 测试 AES-GCM 基本加密/解密
func TestAESGCMBasic(t *testing.T) {
	// 生成密钥
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// 创建加密器
	encryptor, err := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// 测试数据
	originalData := []byte("Hello, this is a test message for AES-GCM encryption!")

	// 加密
	var encryptedBuf bytes.Buffer
	encryptWriter, err := encryptor.NewEncryptWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to create encrypt writer: %v", err)
	}

	n, err := encryptWriter.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}
	if n != len(originalData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(originalData), n)
	}

	if err := encryptWriter.Close(); err != nil {
		t.Fatalf("Failed to close encrypt writer: %v", err)
	}

	encryptedData := encryptedBuf.Bytes()
	t.Logf("Original size: %d, Encrypted size: %d", len(originalData), len(encryptedData))

	// 解密
	decryptReader, err := encryptor.NewDecryptReader(bytes.NewReader(encryptedData))
	if err != nil {
		t.Fatalf("Failed to create decrypt reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	_, err = io.Copy(&decryptedBuf, decryptReader)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// 验证
	if !bytes.Equal(decryptedBuf.Bytes(), originalData) {
		t.Errorf("Decrypted data does not match original.\nExpected: %s\nGot: %s",
			string(originalData), string(decryptedBuf.Bytes()))
	}
}

// TestChaCha20Poly1305Basic 测试 ChaCha20-Poly1305 基本加密/解密
func TestChaCha20Poly1305Basic(t *testing.T) {
	// 生成密钥
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// 创建加密器
	encryptor, err := NewEncryptor(&EncryptConfig{
		Method: MethodChaCha20Poly1305,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// 测试数据
	originalData := []byte("Hello, this is a test message for ChaCha20-Poly1305 encryption!")

	// 加密
	var encryptedBuf bytes.Buffer
	encryptWriter, err := encryptor.NewEncryptWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to create encrypt writer: %v", err)
	}

	n, err := encryptWriter.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}
	if n != len(originalData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(originalData), n)
	}

	if err := encryptWriter.Close(); err != nil {
		t.Fatalf("Failed to close encrypt writer: %v", err)
	}

	encryptedData := encryptedBuf.Bytes()
	t.Logf("Original size: %d, Encrypted size: %d", len(originalData), len(encryptedData))

	// 解密
	decryptReader, err := encryptor.NewDecryptReader(bytes.NewReader(encryptedData))
	if err != nil {
		t.Fatalf("Failed to create decrypt reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	_, err = io.Copy(&decryptedBuf, decryptReader)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// 验证
	if !bytes.Equal(decryptedBuf.Bytes(), originalData) {
		t.Errorf("Decrypted data does not match original.\nExpected: %s\nGot: %s",
			string(originalData), string(decryptedBuf.Bytes()))
	}
}

// TestLargeData 测试大数据加密/解密（多块）
func TestLargeData(t *testing.T) {
	// 生成密钥
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// 创建加密器
	encryptor, err := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// 生成 1MB 测试数据（会被分为多块）
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// 加密
	var encryptedBuf bytes.Buffer
	encryptWriter, err := encryptor.NewEncryptWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to create encrypt writer: %v", err)
	}

	// 分块写入
	chunkSize := 10000
	for i := 0; i < len(largeData); i += chunkSize {
		end := i + chunkSize
		if end > len(largeData) {
			end = len(largeData)
		}
		_, err := encryptWriter.Write(largeData[i:end])
		if err != nil {
			t.Fatalf("Failed to write chunk: %v", err)
		}
	}

	if err := encryptWriter.Close(); err != nil {
		t.Fatalf("Failed to close encrypt writer: %v", err)
	}

	encryptedData := encryptedBuf.Bytes()
	t.Logf("Original size: %d, Encrypted size: %d, Overhead: %.2f%%",
		len(largeData), len(encryptedData), float64(len(encryptedData)-len(largeData))/float64(len(largeData))*100)

	// 解密
	decryptReader, err := encryptor.NewDecryptReader(bytes.NewReader(encryptedData))
	if err != nil {
		t.Fatalf("Failed to create decrypt reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	_, err = io.Copy(&decryptedBuf, decryptReader)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// 验证
	if !bytes.Equal(decryptedBuf.Bytes(), largeData) {
		t.Errorf("Large data decryption failed")
	}
}

// TestMultipleWrites 测试多次写入
func TestMultipleWrites(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	encryptor, err := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// 多个数据块
	chunks := [][]byte{
		[]byte("First chunk of data"),
		[]byte("Second chunk of data"),
		[]byte("Third chunk of data"),
	}

	var encryptedBuf bytes.Buffer
	encryptWriter, err := encryptor.NewEncryptWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to create encrypt writer: %v", err)
	}

	for i, chunk := range chunks {
		_, err := encryptWriter.Write(chunk)
		if err != nil {
			t.Fatalf("Failed to write chunk %d: %v", i, err)
		}
	}

	if err := encryptWriter.Close(); err != nil {
		t.Fatalf("Failed to close encrypt writer: %v", err)
	}

	// 解密
	decryptReader, err := encryptor.NewDecryptReader(bytes.NewReader(encryptedBuf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to create decrypt reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	_, err = io.Copy(&decryptedBuf, decryptReader)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// 验证
	expectedData := bytes.Join(chunks, nil)
	if !bytes.Equal(decryptedBuf.Bytes(), expectedData) {
		t.Errorf("Multiple writes test failed")
	}
}

// TestMultipleReads 测试多次小批量读取
func TestMultipleReads(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	encryptor, err := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	originalData := []byte("This is test data for multiple small reads!")

	// 加密
	var encryptedBuf bytes.Buffer
	encryptWriter, err := encryptor.NewEncryptWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to create encrypt writer: %v", err)
	}

	_, err = encryptWriter.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	encryptWriter.Close()

	// 解密（小批量读取）
	decryptReader, err := encryptor.NewDecryptReader(bytes.NewReader(encryptedBuf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to create decrypt reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	buf := make([]byte, 10) // 每次读取 10 字节

	for {
		n, err := decryptReader.Read(buf)
		if n > 0 {
			decryptedBuf.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read: %v", err)
		}
	}

	// 验证
	if !bytes.Equal(decryptedBuf.Bytes(), originalData) {
		t.Errorf("Multiple reads test failed")
	}
}

// TestEmptyData 测试空数据
func TestEmptyData(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	encryptor, err := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// 加密空数据
	var encryptedBuf bytes.Buffer
	encryptWriter, err := encryptor.NewEncryptWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to create encrypt writer: %v", err)
	}

	if err := encryptWriter.Close(); err != nil {
		t.Fatalf("Failed to close encrypt writer: %v", err)
	}

	// 解密
	decryptReader, err := encryptor.NewDecryptReader(bytes.NewReader(encryptedBuf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to create decrypt reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	_, err = io.Copy(&decryptedBuf, decryptReader)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// 验证
	if decryptedBuf.Len() != 0 {
		t.Errorf("Expected empty data, got %d bytes", decryptedBuf.Len())
	}
}

// TestInvalidKey 测试无效密钥
func TestInvalidKey(t *testing.T) {
	// 无效密钥长度
	_, err := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    []byte("short"),
	})
	if err == nil {
		t.Error("Expected error for invalid key length")
	}
}

// TestCorruptedData 测试损坏的密文
func TestCorruptedData(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	encryptor, err := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	originalData := []byte("This data will be corrupted!")

	// 加密
	var encryptedBuf bytes.Buffer
	encryptWriter, err := encryptor.NewEncryptWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to create encrypt writer: %v", err)
	}

	_, err = encryptWriter.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	encryptWriter.Close()

	encryptedData := encryptedBuf.Bytes()

	// 损坏密文（修改最后一个字节）
	if len(encryptedData) > 0 {
		encryptedData[len(encryptedData)-1] ^= 0xFF
	}

	// 尝试解密应该失败
	decryptReader, err := encryptor.NewDecryptReader(bytes.NewReader(encryptedData))
	if err != nil {
		t.Fatalf("Failed to create decrypt reader: %v", err)
	}

	var decryptedBuf bytes.Buffer
	_, err = io.Copy(&decryptedBuf, decryptReader)
	if err == nil {
		t.Error("Expected error when decrypting corrupted data")
	}
}

// TestWriteAfterClose 测试关闭后写入
func TestWriteAfterClose(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	encryptor, err := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	var encryptedBuf bytes.Buffer
	encryptWriter, err := encryptor.NewEncryptWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("Failed to create encrypt writer: %v", err)
	}

	// 关闭
	if err := encryptWriter.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// 尝试写入应该失败
	_, err = encryptWriter.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error when writing after close")
	}
}

// TestGenerateKeyBase64 测试密钥生成
func TestGenerateKeyBase64(t *testing.T) {
	key1, err := GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	key2, err := GenerateKeyBase64()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// 两次生成的密钥应该不同
	if key1 == key2 {
		t.Error("Generated keys should be different")
	}

	// 密钥应该可以解码
	keyBytes, err := DecodeKeyBase64(key1)
	if err != nil {
		t.Fatalf("Failed to decode key: %v", err)
	}

	if len(keyBytes) != 32 {
		t.Errorf("Expected 32-byte key, got %d", len(keyBytes))
	}
}

// BenchmarkAESGCMEncrypt 加密性能基准测试
func BenchmarkAESGCMEncrypt(b *testing.B) {
	key, _ := GenerateKey()
	encryptor, _ := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})

	data := make([]byte, 64*1024) // 64KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		encryptWriter, _ := encryptor.NewEncryptWriter(&buf)
		encryptWriter.Write(data)
		encryptWriter.Close()
	}
}

// BenchmarkAESGCMDecrypt 解密性能基准测试
func BenchmarkAESGCMDecrypt(b *testing.B) {
	key, _ := GenerateKey()
	encryptor, _ := NewEncryptor(&EncryptConfig{
		Method: MethodAESGCM,
		Key:    key,
	})

	data := make([]byte, 64*1024) // 64KB

	// 先加密
	var encryptedBuf bytes.Buffer
	encryptWriter, _ := encryptor.NewEncryptWriter(&encryptedBuf)
	encryptWriter.Write(data)
	encryptWriter.Close()
	encryptedData := encryptedBuf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decryptReader, _ := encryptor.NewDecryptReader(bytes.NewReader(encryptedData))
		io.Copy(io.Discard, decryptReader)
	}
}

