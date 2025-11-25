package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
)

// EncryptionMethod 加密方法
type EncryptionMethod string

const (
	MethodAESGCM           EncryptionMethod = "aes-256-gcm"
	MethodChaCha20Poly1305 EncryptionMethod = "chacha20-poly1305"
)

// 分块加密配置
const (
	// ChunkSize 每块明文大小 (64KB)
	ChunkSize = 64 * 1024

	// MaxChunkSize 最大块大小限制（明文 + overhead + nonce）
	// 用于防止 DoS 攻击（恶意构造巨大 length）
	// 64KB + 16 bytes (AEAD overhead) + 24 bytes (max nonce) = 64KB + 40 bytes
	MaxChunkSize = ChunkSize + 40

	// MaxCiphertextSize 最大密文大小（仅密文+tag，不含nonce）
	// 64KB + 16 bytes (AEAD overhead)
	MaxCiphertextSize = ChunkSize + 16

	// NonceSize nonce 大小
	NonceSizeAESGCM   = 12
	NonceSizeChaCha20 = 24
)

// EncryptConfig 加密配置
type EncryptConfig struct {
	Method EncryptionMethod
	Key    []byte // 原始密钥（32字节）
}

// Encryptor 加密器接口
type Encryptor interface {
	NewEncryptWriter(w io.Writer) (io.WriteCloser, error)
	NewDecryptReader(r io.Reader) (io.Reader, error)
	NonceSize() int
}

// NewEncryptor 创建加密器
func NewEncryptor(config *EncryptConfig) (Encryptor, error) {
	switch config.Method {
	case MethodAESGCM, "aes-gcm", "":
		return newAESGCMEncryptor(config.Key)
	case MethodChaCha20Poly1305, "chacha20":
		return newChaCha20Encryptor(config.Key)
	default:
		return nil, fmt.Errorf("unsupported encryption method: %s", config.Method)
	}
}

// ============================================================================
// AES-GCM 加密器
// ============================================================================

type aesGCMEncryptor struct {
	aead cipher.AEAD
}

func newAESGCMEncryptor(key []byte) (*aesGCMEncryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256-GCM requires 32-byte key, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &aesGCMEncryptor{aead: aead}, nil
}

func (e *aesGCMEncryptor) NewEncryptWriter(w io.Writer) (io.WriteCloser, error) {
	return newEncryptWriter(w, e.aead, NonceSizeAESGCM), nil
}

func (e *aesGCMEncryptor) NewDecryptReader(r io.Reader) (io.Reader, error) {
	return newDecryptReader(r, e.aead, NonceSizeAESGCM), nil
}

func (e *aesGCMEncryptor) NonceSize() int {
	return NonceSizeAESGCM
}

// ============================================================================
// ChaCha20-Poly1305 加密器
// ============================================================================

type chaCha20Encryptor struct {
	aead cipher.AEAD
}

func newChaCha20Encryptor(key []byte) (*chaCha20Encryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("ChaCha20-Poly1305 requires 32-byte key, got %d", len(key))
	}

	aead, err := chacha20poly1305.NewX(key) // XChaCha20-Poly1305 (24-byte nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to create ChaCha20-Poly1305: %w", err)
	}

	return &chaCha20Encryptor{aead: aead}, nil
}

func (e *chaCha20Encryptor) NewEncryptWriter(w io.Writer) (io.WriteCloser, error) {
	return newEncryptWriter(w, e.aead, NonceSizeChaCha20), nil
}

func (e *chaCha20Encryptor) NewDecryptReader(r io.Reader) (io.Reader, error) {
	return newDecryptReader(r, e.aead, NonceSizeChaCha20), nil
}

func (e *chaCha20Encryptor) NonceSize() int {
	return NonceSizeChaCha20
}

// ============================================================================
// 加密 Writer (分块加密)
// ============================================================================

// encryptWriter 加密写入器
// 格式: [块长度(4字节)][nonce(12/24字节)][密文+tag]
type encryptWriter struct {
	writer    io.Writer
	aead      cipher.AEAD
	nonceSize int
	buffer    []byte
	mu        sync.Mutex
	closed    bool
}

func newEncryptWriter(w io.Writer, aead cipher.AEAD, nonceSize int) *encryptWriter {
	return &encryptWriter{
		writer:    w,
		aead:      aead,
		nonceSize: nonceSize,
		buffer:    make([]byte, 0, ChunkSize),
	}
}

func (e *encryptWriter) Write(p []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return 0, io.ErrClosedPipe
	}

	totalWritten := 0

	for len(p) > 0 {
		// 计算本次可写入缓冲区的数据量
		available := ChunkSize - len(e.buffer)
		toWrite := len(p)
		if toWrite > available {
			toWrite = available
		}

		// 追加到缓冲区
		e.buffer = append(e.buffer, p[:toWrite]...)
		p = p[toWrite:]
		totalWritten += toWrite

		// 如果缓冲区满了，加密并写入
		if len(e.buffer) >= ChunkSize {
			if err := e.flush(); err != nil {
				return totalWritten, err
			}
		}
	}

	return totalWritten, nil
}

func (e *encryptWriter) flush() error {
	if len(e.buffer) == 0 {
		return nil
	}

	// 生成随机 nonce
	nonce := make([]byte, e.nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// 加密数据 (AEAD.Seal 会自动追加 tag)
	ciphertext := e.aead.Seal(nil, nonce, e.buffer, nil)

	// 写入格式: [块长度(4字节)][nonce][密文+tag]
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(ciphertext)))

	// 写入 [块长度]
	if _, err := e.writer.Write(header); err != nil {
		return fmt.Errorf("failed to write chunk length: %w", err)
	}

	// 写入 [nonce]
	if _, err := e.writer.Write(nonce); err != nil {
		return fmt.Errorf("failed to write nonce: %w", err)
	}

	// 写入 [密文+tag]
	if _, err := e.writer.Write(ciphertext); err != nil {
		return fmt.Errorf("failed to write ciphertext: %w", err)
	}

	// 清空缓冲区
	e.buffer = e.buffer[:0]

	return nil
}

func (e *encryptWriter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}

	// 1. Flush 剩余数据（最后一次 Seal）
	var flushErr error
	if len(e.buffer) > 0 {
		flushErr = e.flush()
	}

	// 2. 标记为已关闭（即使 flush 失败也要标记，避免重复 Close）
	e.closed = true

	// 3. 关闭底层 writer (如果支持)
	var closeErr error
	if closer, ok := e.writer.(io.Closer); ok {
		closeErr = closer.Close()
	}

	// 4. 返回第一个错误
	if flushErr != nil {
		return fmt.Errorf("flush error during close: %w", flushErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close underlying writer error: %w", closeErr)
	}

	return nil
}

// ============================================================================
// 解密 Reader (分块解密)
// ============================================================================

// decryptReader 解密读取器
type decryptReader struct {
	reader    io.Reader
	aead      cipher.AEAD
	nonceSize int
	buffer    []byte // 当前块的明文缓冲区
	offset    int    // 当前块的读取偏移
	eof       bool
	mu        sync.Mutex
}

func newDecryptReader(r io.Reader, aead cipher.AEAD, nonceSize int) *decryptReader {
	return &decryptReader{
		reader:    r,
		aead:      aead,
		nonceSize: nonceSize,
		buffer:    nil,
		offset:    0,
		eof:       false,
	}
}

func (d *decryptReader) Read(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.eof {
		return 0, io.EOF
	}

	totalRead := 0

	for len(p) > 0 {
		// 如果当前块已读完，读取下一块
		if d.buffer == nil || d.offset >= len(d.buffer) {
			if err := d.readNextChunk(); err != nil {
				if err == io.EOF {
					d.eof = true
					if totalRead > 0 {
						return totalRead, nil
					}
					return 0, io.EOF
				}
				return totalRead, err
			}
		}

		// 从当前块读取数据
		n := copy(p, d.buffer[d.offset:])
		d.offset += n
		p = p[n:]
		totalRead += n
	}

	return totalRead, nil
}

func (d *decryptReader) readNextChunk() error {
	// 读取 [块长度(4字节)]
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(d.reader, lengthBuf); err != nil {
		return err
	}
	chunkLen := binary.BigEndian.Uint32(lengthBuf)

	// 严格的长度校验，防止 DoS 攻击
	// chunkLen 是密文长度（不含nonce），最大应为 ChunkSize + AEAD overhead (16 bytes)
	if chunkLen == 0 {
		return fmt.Errorf("invalid chunk length: 0 (empty chunk)")
	}
	if chunkLen > MaxCiphertextSize {
		return fmt.Errorf("chunk length %d exceeds maximum allowed %d (potential DoS attack)", 
			chunkLen, MaxCiphertextSize)
	}

	// 读取 [nonce]
	nonce := make([]byte, d.nonceSize)
	if _, err := io.ReadFull(d.reader, nonce); err != nil {
		return fmt.Errorf("failed to read nonce: %w", err)
	}

	// 读取 [密文+tag]
	ciphertext := make([]byte, chunkLen)
	if _, err := io.ReadFull(d.reader, ciphertext); err != nil {
		return fmt.Errorf("failed to read ciphertext: %w", err)
	}

	// 解密
	plaintext, err := d.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	// 更新缓冲区
	d.buffer = plaintext
	d.offset = 0

	return nil
}

// ============================================================================
// 工具函数
// ============================================================================

// GenerateKey 生成随机密钥
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32) // 256-bit key
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// GenerateKeyBase64 生成 Base64 编码的密钥
func GenerateKeyBase64() (string, error) {
	key, err := GenerateKey()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// DecodeKeyBase64 解码 Base64 密钥
func DecodeKeyBase64(keyStr string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(keyStr)
}
