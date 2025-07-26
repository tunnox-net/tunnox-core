package encryption

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"io"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/errors"
)

// EncryptionKey 加密密钥接口
type EncryptionKey interface {
	// GetKey 获取加密密钥
	GetKey() []byte
	// GetKeyID 获取密钥ID，用于密钥管理
	GetKeyID() string
}

// StaticKey 静态密钥实现
type StaticKey struct {
	key   []byte
	keyID string
}

// NewStaticKey 创建静态密钥
func NewStaticKey(key []byte, keyID string) *StaticKey {
	return &StaticKey{
		key:   key,
		keyID: keyID,
	}
}

func (sk *StaticKey) GetKey() []byte {
	return sk.key
}

func (sk *StaticKey) GetKeyID() string {
	return sk.keyID
}

// EncryptionReader 加密读取器
type EncryptionReader struct {
	reader io.Reader
	key    EncryptionKey
	ctx    context.Context
	dispose.Dispose
}

// NewEncryptionReader 创建加密读取器
func NewEncryptionReader(reader io.Reader, key EncryptionKey, parentCtx context.Context) *EncryptionReader {
	er := &EncryptionReader{
		reader: reader,
		key:    key,
	}
	er.SetCtx(parentCtx, er.onClose)
	return er
}

func (er *EncryptionReader) onClose() error {
	// 加密读取器不需要特殊清理
	return nil
}

// Read 读取并解密数据
func (er *EncryptionReader) Read(p []byte) (n int, err error) {
	select {
	case <-er.Ctx().Done():
		return 0, er.Ctx().Err()
	default:
	}

	// 读取加密数据长度（4字节）
	lengthBytes := make([]byte, 4)
	_, err = io.ReadFull(er.reader, lengthBytes)
	if err != nil {
		return 0, err
	}

	encryptedLength := binary.BigEndian.Uint32(lengthBytes)
	if encryptedLength == 0 {
		return 0, io.EOF
	}

	// 读取加密数据
	encryptedData := make([]byte, encryptedLength)
	_, err = io.ReadFull(er.reader, encryptedData)
	if err != nil {
		return 0, err
	}

	// 解密数据
	decryptedData, err := er.decrypt(encryptedData)
	if err != nil {
		return 0, err
	}

	// 复制到输出缓冲区
	n = copy(p, decryptedData)
	return n, nil
}

// ReadAll 读取所有数据并解密
func (er *EncryptionReader) ReadAll() ([]byte, error) {
	var result bytes.Buffer
	buffer := make([]byte, 4096)

	for {
		n, err := er.Read(buffer)
		if n > 0 {
			result.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return result.Bytes(), nil
}

// decrypt 解密数据
func (er *EncryptionReader) decrypt(encryptedData []byte) ([]byte, error) {
	if len(encryptedData) < aes.BlockSize+12 { // 至少需要IV(12字节)和GCM标签(16字节)
		return nil, errors.NewEncryptionError("decrypt", "encrypted data too short", nil)
	}

	// 提取IV（前12字节）
	iv := encryptedData[:12]
	ciphertext := encryptedData[12:]

	// 创建AES cipher
	block, err := aes.NewCipher(er.key.GetKey())
	if err != nil {
		return nil, errors.NewEncryptionError("decrypt", "failed to create cipher", err)
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.NewEncryptionError("decrypt", "failed to create GCM", err)
	}

	// 解密
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, errors.NewEncryptionError("decrypt", "failed to decrypt data", err)
	}

	return plaintext, nil
}

// EncryptionWriter 加密写入器
type EncryptionWriter struct {
	writer io.Writer
	key    EncryptionKey
	ctx    context.Context
	dispose.Dispose
}

// NewEncryptionWriter 创建加密写入器
func NewEncryptionWriter(writer io.Writer, key EncryptionKey, parentCtx context.Context) *EncryptionWriter {
	ew := &EncryptionWriter{
		writer: writer,
		key:    key,
	}
	ew.SetCtx(parentCtx, ew.onClose)
	return ew
}

func (ew *EncryptionWriter) onClose() error {
	// 加密写入器不需要特殊清理
	return nil
}

// Write 加密并写入数据
func (ew *EncryptionWriter) Write(p []byte) (n int, err error) {
	select {
	case <-ew.Ctx().Done():
		return 0, ew.Ctx().Err()
	default:
	}

	if len(p) == 0 {
		return 0, nil
	}

	// 加密数据
	encryptedData, err := ew.encrypt(p)
	if err != nil {
		return 0, err
	}

	// 写入加密数据长度
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(encryptedData)))
	_, err = ew.writer.Write(lengthBytes)
	if err != nil {
		return 0, err
	}

	// 写入加密数据
	_, err = ew.writer.Write(encryptedData)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// WriteAll 加密并写入所有数据
func (ew *EncryptionWriter) WriteAll(data []byte) error {
	_, err := ew.Write(data)
	return err
}

// encrypt 加密数据
func (ew *EncryptionWriter) encrypt(plaintext []byte) ([]byte, error) {
	// 创建AES cipher
	block, err := aes.NewCipher(ew.key.GetKey())
	if err != nil {
		return nil, errors.NewEncryptionError("encrypt", "failed to create cipher", err)
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.NewEncryptionError("encrypt", "failed to create GCM", err)
	}

	// 生成随机IV（12字节）
	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, errors.NewEncryptionError("encrypt", "failed to generate IV", err)
	}

	// 加密
	ciphertext := gcm.Seal(nil, iv, plaintext, nil)

	// 返回IV + 密文
	result := make([]byte, len(iv)+len(ciphertext))
	copy(result, iv)
	copy(result[len(iv):], ciphertext)

	return result, nil
}

// EncryptionManager 加密管理器
type EncryptionManager struct {
	key EncryptionKey
	dispose.Dispose
}

// NewEncryptionManager 创建加密管理器
func NewEncryptionManager(key EncryptionKey, parentCtx context.Context) *EncryptionManager {
	em := &EncryptionManager{
		key: key,
	}
	em.SetCtx(parentCtx, em.onClose)
	return em
}

func (em *EncryptionManager) onClose() error {
	// 加密管理器不需要特殊清理
	return nil
}

// EncryptData 加密数据
func (em *EncryptionManager) EncryptData(data []byte) ([]byte, error) {
	select {
	case <-em.Ctx().Done():
		return nil, em.Ctx().Err()
	default:
	}

	// 创建AES cipher
	block, err := aes.NewCipher(em.key.GetKey())
	if err != nil {
		return nil, errors.NewEncryptionError("encrypt_data", "failed to create cipher", err)
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.NewEncryptionError("encrypt_data", "failed to create GCM", err)
	}

	// 生成随机IV（12字节）
	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, errors.NewEncryptionError("encrypt_data", "failed to generate IV", err)
	}

	// 加密
	ciphertext := gcm.Seal(nil, iv, data, nil)

	// 返回IV + 密文
	result := make([]byte, len(iv)+len(ciphertext))
	copy(result, iv)
	copy(result[len(iv):], ciphertext)

	return result, nil
}

// DecryptData 解密数据
func (em *EncryptionManager) DecryptData(encryptedData []byte) ([]byte, error) {
	select {
	case <-em.Ctx().Done():
		return nil, em.Ctx().Err()
	default:
	}

	if len(encryptedData) < aes.BlockSize+12 { // 至少需要IV(12字节)和GCM标签(16字节)
		return nil, errors.NewEncryptionError("decrypt_data", "encrypted data too short", nil)
	}

	// 提取IV（前12字节）
	iv := encryptedData[:12]
	ciphertext := encryptedData[12:]

	// 创建AES cipher
	block, err := aes.NewCipher(em.key.GetKey())
	if err != nil {
		return nil, errors.NewEncryptionError("decrypt_data", "failed to create cipher", err)
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.NewEncryptionError("decrypt_data", "failed to create GCM", err)
	}

	// 解密
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, errors.NewEncryptionError("decrypt_data", "failed to decrypt data", err)
	}

	return plaintext, nil
}

// GetKey 获取加密密钥
func (em *EncryptionManager) GetKey() EncryptionKey {
	return em.key
}

// SetKey 设置加密密钥
func (em *EncryptionManager) SetKey(key EncryptionKey) {
	em.key = key
}
