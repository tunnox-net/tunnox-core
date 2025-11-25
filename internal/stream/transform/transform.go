package transform

import (
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

// StreamTransformer 流转换器接口
type StreamTransformer interface {
	// WrapReader 包装 Reader（解密 + 解压）
	WrapReader(r io.Reader) (io.Reader, error)

	// WrapWriter 包装 Writer（压缩 + 加密）
	WrapWriter(w io.Writer) (io.WriteCloser, error)
}

// TransformConfig 转换配置
type TransformConfig struct {
	EnableCompression bool
	CompressionLevel  int // 1-9, 默认 6
	EnableEncryption  bool
	EncryptionMethod  string // "aes-256-gcm", "chacha20-poly1305"
	EncryptionKey     string // Base64 编码的密钥
}

// NoOpTransformer 无操作转换器（默认，不压缩不加密）
type NoOpTransformer struct{}

func (t *NoOpTransformer) WrapReader(r io.Reader) (io.Reader, error) {
	return r, nil
}

func (t *NoOpTransformer) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	return &nopWriteCloser{w}, nil
}

// nopWriteCloser 包装 Writer 为 WriteCloser
type nopWriteCloser struct {
	io.Writer
}

func (w *nopWriteCloser) Close() error {
	return nil
}

// DefaultTransformer 默认转换器（支持压缩 + 加密）
type DefaultTransformer struct {
	config TransformConfig
	cipher cipher.AEAD
}

// NewTransformer 创建转换器
func NewTransformer(config *TransformConfig) (StreamTransformer, error) {
	if config == nil || (!config.EnableCompression && !config.EnableEncryption) {
		return &NoOpTransformer{}, nil
	}

	transformer := &DefaultTransformer{
		config: *config,
	}

	// ⚠️ 加密功能警告
	if config.EnableEncryption {
		// 注意：当前版本的加密功能未完成实现
		// 数据将以明文形式传输，仅压缩生效
		// TODO: 在生产环境使用前必须完成加密实现
		fmt.Println("⚠️  WARNING: Encryption is enabled but not yet implemented. Data will be transmitted in plaintext.")

		if err := transformer.initCipher(); err != nil {
			return nil, fmt.Errorf("failed to init cipher: %w", err)
		}
	}

	return transformer, nil
}

// initCipher 初始化加密器
func (t *DefaultTransformer) initCipher() error {
	// 解码密钥
	keyBytes, err := base64.StdEncoding.DecodeString(t.config.EncryptionKey)
	if err != nil {
		return fmt.Errorf("invalid encryption key: %w", err)
	}

	// 根据算法创建 AEAD
	switch t.config.EncryptionMethod {
	case "aes-256-gcm", "aes-gcm", "":
		block, err := aes.NewCipher(keyBytes)
		if err != nil {
			return fmt.Errorf("failed to create AES cipher: %w", err)
		}

		aead, err := cipher.NewGCM(block)
		if err != nil {
			return fmt.Errorf("failed to create GCM: %w", err)
		}

		t.cipher = aead

	case "chacha20-poly1305", "chacha20":
		aead, err := chacha20poly1305.New(keyBytes)
		if err != nil {
			return fmt.Errorf("failed to create ChaCha20-Poly1305: %w", err)
		}

		t.cipher = aead

	default:
		return fmt.Errorf("unsupported encryption method: %s", t.config.EncryptionMethod)
	}

	return nil
}

// WrapReader 包装 Reader（顺序：解密 → 解压）
func (t *DefaultTransformer) WrapReader(r io.Reader) (io.Reader, error) {
	var reader io.Reader = r

	// 1. 解密（如果启用）
	if t.config.EnableEncryption {
		reader = newDecryptReader(reader, t.cipher)
	}

	// 2. 解压（如果启用）
	if t.config.EnableCompression {
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		reader = gzipReader
	}

	return reader, nil
}

// WrapWriter 包装 Writer（顺序：压缩 → 加密）
func (t *DefaultTransformer) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	var writer io.Writer = w
	var closers []io.Closer

	// 1. 加密（如果启用）
	if t.config.EnableEncryption {
		encryptWriter := newEncryptWriter(writer, t.cipher)
		writer = encryptWriter
		closers = append(closers, encryptWriter)
	}

	// 2. 压缩（如果启用）
	if t.config.EnableCompression {
		level := t.config.CompressionLevel
		if level == 0 {
			level = gzip.DefaultCompression
		}

		gzipWriter, err := gzip.NewWriterLevel(writer, level)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip writer: %w", err)
		}

		writer = gzipWriter
		closers = append([]io.Closer{gzipWriter}, closers...) // gzip 先关闭
	}

	return &multiCloser{
		Writer:  writer,
		closers: closers,
	}, nil
}

// multiCloser 多层 Closer
type multiCloser struct {
	io.Writer
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var firstErr error
	for _, closer := range m.closers {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ============================================================================
// ⚠️ 警告：加密功能当前未实现
// ============================================================================
// decryptReader 和 encryptWriter 当前只是占位符实现
// 实际的加密/解密逻辑需要在生产环境使用前完成
//
// TODO: 实现分块加密/解密（AEAD stream）
// 建议的实现方案：
// 1. 将数据分块（每块 64KB）
// 2. 为每块生成唯一的 nonce（使用计数器）
// 3. 使用 AEAD.Seal/Open 加密/解密每块
// 4. 格式: [块长度(4字节)][nonce(12/24字节)][密文+tag]
// ============================================================================

// decryptReader 解密 Reader (⚠️ 当前未实现，直接透传)
type decryptReader struct {
	reader io.Reader
	cipher cipher.AEAD
	buffer []byte
}

func newDecryptReader(r io.Reader, aead cipher.AEAD) *decryptReader {
	return &decryptReader{
		reader: r,
		cipher: aead,
		buffer: make([]byte, 32*1024),
	}
}

func (d *decryptReader) Read(p []byte) (int, error) {
	// ⚠️ 警告：当前未实现加密，数据明文传输
	// TODO: 实现分块解密逻辑
	return d.reader.Read(p)
}

// encryptWriter 加密 Writer (⚠️ 当前未实现，直接透传)
type encryptWriter struct {
	writer io.Writer
	cipher cipher.AEAD
	buffer []byte
}

func newEncryptWriter(w io.Writer, aead cipher.AEAD) *encryptWriter {
	return &encryptWriter{
		writer: w,
		cipher: aead,
		buffer: make([]byte, 32*1024),
	}
}

func (e *encryptWriter) Write(p []byte) (int, error) {
	// ⚠️ 警告：当前未实现加密，数据明文传输
	// TODO: 实现分块加密逻辑
	return e.writer.Write(p)
}

func (e *encryptWriter) Close() error {
	if closer, ok := e.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// GenerateEncryptionKey 生成随机加密密钥
func GenerateEncryptionKey(method string) (string, error) {
	var keyLen int
	switch method {
	case "aes-256-gcm", "aes-gcm", "":
		keyLen = 32 // AES-256
	case "chacha20-poly1305", "chacha20":
		keyLen = 32 // ChaCha20
	default:
		return "", fmt.Errorf("unsupported encryption method: %s", method)
	}

	key := make([]byte, keyLen)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(key), nil
}
