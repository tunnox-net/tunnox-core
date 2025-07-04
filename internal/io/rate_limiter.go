package io

import (
	"context"
	"io"
	"tunnox-core/internal/utils"

	"golang.org/x/time/rate"
)

// RateLimiter 实现限速传输的Reader和Writer
type RateLimiter struct {
	reader  io.Reader
	writer  io.Writer
	limiter *rate.Limiter
	utils.Dispose
}

// RateLimiterReader 实现io.Reader的限速读取
type RateLimiterReader struct {
	reader  io.Reader
	limiter *rate.Limiter
	utils.Dispose
}

// RateLimiterWriter 实现io.Writer的限速写入
type RateLimiterWriter struct {
	writer  io.Writer
	limiter *rate.Limiter
	utils.Dispose
}

// Read 实现io.Reader接口，限速读取
func (r *RateLimiterReader) Read(p []byte) (n int, err error) {
	if r.IsClosed() {
		return 0, io.EOF
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > utils.DefaultChunkSize {
		chunkSize = utils.DefaultChunkSize
	}

	// 等待限速器允许读取
	err = r.limiter.WaitN(r.Ctx(), chunkSize)
	if err != nil {
		return 0, utils.NewRateLimitError(0, "rate limiter wait failed", err)
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
		return 0, utils.ErrStreamClosed
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > utils.DefaultChunkSize {
		chunkSize = utils.DefaultChunkSize
	}

	// 等待限速器允许写入
	err = w.limiter.WaitN(w.Ctx(), chunkSize)
	if err != nil {
		return 0, utils.NewRateLimitError(0, "rate limiter wait failed", err)
	}

	// 只写入chunkSize大小的数据
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	return w.writer.Write(p)
}

// onClose 资源释放
func (r *RateLimiterReader) onClose() {
	// 读取器通常不需要特殊清理，但可以在这里添加日志或其他清理逻辑
}

func (w *RateLimiterWriter) onClose() {
	// 写入器通常不需要特殊清理，但可以在这里添加日志或其他清理逻辑
}

// calculateBurst 计算突发大小
func calculateBurst(bytesPerSecond int64) int {
	if bytesPerSecond <= 0 {
		return utils.MinBurstSize
	}

	burst := int(bytesPerSecond / int64(utils.DefaultBurstRatio))
	if burst < utils.MinBurstSize {
		burst = utils.MinBurstSize
	}
	return burst
}

// NewRateLimiterReader 创建限速读取器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiterReader(reader io.Reader, bytesPerSecond int64, parentCtx context.Context) (*RateLimiterReader, error) {
	if bytesPerSecond <= 0 {
		return nil, utils.NewRateLimitError(bytesPerSecond, "invalid rate limit", utils.ErrInvalidRate)
	}
	burst := calculateBurst(bytesPerSecond)
	limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
	r := &RateLimiterReader{
		reader:  reader,
		limiter: limiter,
	}
	r.SetCtx(parentCtx, r.onClose)
	return r, nil
}

// NewRateLimiterWriter 创建限速写入器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64, parentCtx context.Context) (*RateLimiterWriter, error) {
	if bytesPerSecond <= 0 {
		return nil, utils.NewRateLimitError(bytesPerSecond, "invalid rate limit", utils.ErrInvalidRate)
	}
	burst := calculateBurst(bytesPerSecond)
	limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
	w := &RateLimiterWriter{
		writer:  writer,
		limiter: limiter,
	}
	w.SetCtx(parentCtx, w.onClose)
	return w, nil
}

// NewRateLimiter 创建同时支持读写限速的限速器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiter(bytesPerSecond int64, parentCtx context.Context) (*RateLimiter, error) {
	if bytesPerSecond <= 0 {
		return nil, utils.NewRateLimitError(bytesPerSecond, "invalid rate limit", utils.ErrInvalidRate)
	}
	burst := calculateBurst(bytesPerSecond)
	limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
	r := &RateLimiter{
		limiter: limiter,
	}
	r.SetCtx(parentCtx, r.onClose)
	return r, nil
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
		return 0, utils.ErrReaderNil
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > utils.DefaultChunkSize {
		chunkSize = utils.DefaultChunkSize
	}

	// 等待限速器允许读取
	err = r.limiter.WaitN(r.Ctx(), chunkSize)
	if err != nil {
		return 0, utils.NewRateLimitError(0, "rate limiter wait failed", err)
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
		return 0, utils.ErrStreamClosed
	}
	if r.writer == nil {
		return 0, utils.ErrWriterNil
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > utils.DefaultChunkSize {
		chunkSize = utils.DefaultChunkSize
	}

	// 等待限速器允许写入
	err = r.limiter.WaitN(r.Ctx(), chunkSize)
	if err != nil {
		return 0, utils.NewRateLimitError(0, "rate limiter wait failed", err)
	}

	// 只写入chunkSize大小的数据
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	return r.writer.Write(p)
}

// onClose 资源释放
func (r *RateLimiter) onClose() {
	// 限速器通常不需要特殊清理，但可以在这里添加日志或其他清理逻辑
}

// SetRate 设置新的速率限制
func (r *RateLimiter) SetRate(bytesPerSecond int64) error {
	if bytesPerSecond <= 0 {
		return utils.NewRateLimitError(bytesPerSecond, "invalid rate limit", utils.ErrInvalidRate)
	}
	burst := calculateBurst(bytesPerSecond)
	r.limiter.SetLimit(rate.Limit(bytesPerSecond))
	r.limiter.SetBurst(burst)
	return nil
}

// SetRate 设置新的速率限制
func (r *RateLimiterReader) SetRate(bytesPerSecond int64) error {
	if bytesPerSecond <= 0 {
		return utils.NewRateLimitError(bytesPerSecond, "invalid rate limit", utils.ErrInvalidRate)
	}
	burst := calculateBurst(bytesPerSecond)
	r.limiter.SetLimit(rate.Limit(bytesPerSecond))
	r.limiter.SetBurst(burst)
	return nil
}

// SetRate 设置新的速率限制
func (w *RateLimiterWriter) SetRate(bytesPerSecond int64) error {
	if bytesPerSecond <= 0 {
		return utils.NewRateLimitError(bytesPerSecond, "invalid rate limit", utils.ErrInvalidRate)
	}
	burst := calculateBurst(bytesPerSecond)
	w.limiter.SetLimit(rate.Limit(bytesPerSecond))
	w.limiter.SetBurst(burst)
	return nil
}
