package transform

import (
	"compress/gzip"
	"fmt"
	"io"
	"tunnox-core/internal/stream/encryption"
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
	config    TransformConfig
	encryptor encryption.Encryptor
}

// NewTransformer 创建转换器
func NewTransformer(config *TransformConfig) (StreamTransformer, error) {
	if config == nil || (!config.EnableCompression && !config.EnableEncryption) {
		return &NoOpTransformer{}, nil
	}

	transformer := &DefaultTransformer{
		config: *config,
	}

	// 初始化加密器
	if config.EnableEncryption {
		if err := transformer.initEncryptor(); err != nil {
			return nil, fmt.Errorf("failed to init encryptor: %w", err)
		}
	}

	return transformer, nil
}

// initEncryptor 初始化加密器
func (t *DefaultTransformer) initEncryptor() error {
	// 解码密钥
	keyBytes, err := encryption.DecodeKeyBase64(t.config.EncryptionKey)
	if err != nil {
		return fmt.Errorf("invalid encryption key: %w", err)
	}

	// 创建加密器
	encryptConfig := &encryption.EncryptConfig{
		Method: encryption.EncryptionMethod(t.config.EncryptionMethod),
		Key:    keyBytes,
	}

	encryptor, err := encryption.NewEncryptor(encryptConfig)
	if err != nil {
		return fmt.Errorf("failed to create encryptor: %w", err)
	}

	t.encryptor = encryptor
	return nil
}

// WrapReader 包装 Reader（顺序：解密 → 解压）
func (t *DefaultTransformer) WrapReader(r io.Reader) (io.Reader, error) {
	var reader io.Reader = r

	// 1. 解密（如果启用）
	if t.config.EnableEncryption {
		decryptReader, err := t.encryptor.NewDecryptReader(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to create decrypt reader: %w", err)
		}
		reader = decryptReader
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
		encryptWriter, err := t.encryptor.NewEncryptWriter(writer)
		if err != nil {
			return nil, fmt.Errorf("failed to create encrypt writer: %w", err)
		}
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

// GenerateEncryptionKey 生成随机加密密钥
func GenerateEncryptionKey(method string) (string, error) {
	// 所有支持的方法都使用 32 字节密钥
	return encryption.GenerateKeyBase64()
}
