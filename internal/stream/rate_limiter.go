package stream

import (
	"context"
	"io"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/errors"
	"tunnox-core/internal/utils"
)

// RateLimiter 实现限速传输的Reader和Writer
type RateLimiter struct {
	reader      io.Reader
	writer      io.Writer
	tokenBucket *TokenBucket
	utils.Dispose
}

// RateLimiterReader 实现io.Reader的限速读取
type RateLimiterReader struct {
	reader      io.Reader
	tokenBucket *TokenBucket
	utils.Dispose
}

// RateLimiterWriter 实现io.Writer的限速写入
type RateLimiterWriter struct {
	writer      io.Writer
	tokenBucket *TokenBucket
	utils.Dispose
}

// Read 实现io.Reader接口，限速读取
func (r *RateLimiterReader) Read(p []byte) (n int, err error) {
	if r.IsClosed() {
		return 0, io.EOF
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > constants.DefaultChunkSize {
		chunkSize = constants.DefaultChunkSize
	}

	// 等待限速器允许读取
	err = r.tokenBucket.WaitForTokens(chunkSize)
	if err != nil {
		return 0, errors.NewRateLimitError(0, "rate limiter wait failed", err)
	}

	// 只读取chunkSize大小的数据
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	return r.reader.Read(p)
}

// Write 实现io.Writer接口，限速写入
func (w *RateLimiterWriter) Write(p []byte) (n int, err error) {
	if w.IsClosed() {
		return 0, errors.ErrStreamClosed
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > constants.DefaultChunkSize {
		chunkSize = constants.DefaultChunkSize
	}

	// 等待限速器允许写入
	err = w.tokenBucket.WaitForTokens(chunkSize)
	if err != nil {
		return 0, errors.NewRateLimitError(0, "rate limiter wait failed", err)
	}

	// 只写入chunkSize大小的数据
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	return w.writer.Write(p)
}

// onClose 资源释放
func (r *RateLimiterReader) onClose() {
	// no-op
}

func (w *RateLimiterWriter) onClose() {
	// no-op
}

// NewRateLimiterReader 创建限速读取器
func NewRateLimiterReader(reader io.Reader, bytesPerSecond int64, ctx context.Context) (*RateLimiterReader, error) {
	if bytesPerSecond <= 0 {
		return nil, errors.ErrInvalidRate
	}

	tokenBucket, err := NewTokenBucket(bytesPerSecond, ctx)
	if err != nil {
		return nil, err
	}

	rateLimiter := &RateLimiterReader{
		reader:      reader,
		tokenBucket: tokenBucket,
	}
	rateLimiter.SetCtx(ctx, rateLimiter.onClose)
	return rateLimiter, nil
}

// NewRateLimiterWriter 创建限速写入器
func NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64, ctx context.Context) (*RateLimiterWriter, error) {
	if bytesPerSecond <= 0 {
		return nil, errors.ErrInvalidRate
	}

	tokenBucket, err := NewTokenBucket(bytesPerSecond, ctx)
	if err != nil {
		return nil, err
	}

	rateLimiter := &RateLimiterWriter{
		writer:      writer,
		tokenBucket: tokenBucket,
	}
	rateLimiter.SetCtx(ctx, rateLimiter.onClose)
	return rateLimiter, nil
}

// NewRateLimiter 创建限速器
func NewRateLimiter(bytesPerSecond int64, parentCtx context.Context) (*RateLimiter, error) {
	if bytesPerSecond <= 0 {
		return nil, errors.ErrInvalidRate
	}

	tokenBucket, err := NewTokenBucket(bytesPerSecond, parentCtx)
	if err != nil {
		return nil, err
	}

	rateLimiter := &RateLimiter{
		tokenBucket: tokenBucket,
	}
	rateLimiter.SetCtx(parentCtx, rateLimiter.onClose)
	return rateLimiter, nil
}

// SetReader 设置读取器
func (r *RateLimiter) SetReader(reader io.Reader) {
	r.reader = reader
}

// SetWriter 设置写入器
func (r *RateLimiter) SetWriter(writer io.Writer) {
	r.writer = writer
}

// Read 实现io.Reader接口
func (r *RateLimiter) Read(p []byte) (n int, err error) {
	if r.IsClosed() {
		return 0, io.EOF
	}

	if r.reader == nil {
		return 0, errors.ErrReaderNil
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > constants.DefaultChunkSize {
		chunkSize = constants.DefaultChunkSize
	}

	// 等待限速器允许读取
	err = r.tokenBucket.WaitForTokens(chunkSize)
	if err != nil {
		return 0, errors.NewRateLimitError(0, "rate limiter wait failed", err)
	}

	// 只读取chunkSize大小的数据
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	return r.reader.Read(p)
}

// Write 实现io.Writer接口
func (r *RateLimiter) Write(p []byte) (n int, err error) {
	if r.IsClosed() {
		return 0, errors.ErrStreamClosed
	}

	if r.writer == nil {
		return 0, errors.ErrWriterNil
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > constants.DefaultChunkSize {
		chunkSize = constants.DefaultChunkSize
	}

	// 等待限速器允许写入
	err = r.tokenBucket.WaitForTokens(chunkSize)
	if err != nil {
		return 0, errors.NewRateLimitError(0, "rate limiter wait failed", err)
	}

	// 只写入chunkSize大小的数据
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	return r.writer.Write(p)
}

// onClose 资源释放
func (r *RateLimiter) onClose() {
	// no-op
}

// SetRate 设置速率（仅对Reader和Writer有效）
func (r *RateLimiterReader) SetRate(bytesPerSecond int64) error {
	return r.tokenBucket.SetRate(bytesPerSecond)
}

func (w *RateLimiterWriter) SetRate(bytesPerSecond int64) error {
	return w.tokenBucket.SetRate(bytesPerSecond)
}
