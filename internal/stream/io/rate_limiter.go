package io

import (
	"context"
	"io"
	"sync"
	"time"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/errors"
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
	rate      int64
	rateMutex sync.Mutex
	lastTime  time.Time
	tokens    int
}

// RateLimiterWriter 实现io.Writer的限速写入
type RateLimiterWriter struct {
	writer  io.Writer
	limiter *rate.Limiter
	utils.Dispose
	rate      int64
	rateMutex sync.Mutex
	lastTime  time.Time
	tokens    int
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
	err = r.waitForTokens(chunkSize)
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
	err = w.waitForTokens(chunkSize)
	if err != nil {
		return 0, errors.NewRateLimitError(0, "rate limiter wait failed", err)
	}

	// 只写入chunkSize大小的数据
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	return w.writer.Write(p)
}

// waitForTokens 等待足够的令牌
func (r *RateLimiterReader) waitForTokens(tokensNeeded int) error {
	r.rateMutex.Lock()
	defer r.rateMutex.Unlock()

	now := time.Now()
	if r.lastTime.IsZero() {
		r.lastTime = now
		r.tokens = constants.MinBurstSize
	}

	// 计算从上次到现在应该产生的令牌数
	elapsed := now.Sub(r.lastTime)
	tokensToAdd := int(float64(r.rate) * elapsed.Seconds())
	r.tokens += tokensToAdd

	// 限制令牌数量不超过burst大小
	burstSize := int(float64(r.rate) / float64(constants.DefaultBurstRatio))
	if burstSize < constants.MinBurstSize {
		burstSize = constants.MinBurstSize
	}
	if r.tokens > burstSize {
		r.tokens = burstSize
	}

	// 如果令牌不足，需要等待
	if r.tokens < tokensNeeded {
		tokensNeeded -= r.tokens
		r.tokens = 0

		// 计算需要等待的时间
		waitTime := time.Duration(float64(time.Second) * float64(tokensNeeded) / float64(r.rate))
		if waitTime > 0 {
			select {
			case <-time.After(waitTime):
			case <-r.Ctx().Done():
				return errors.ErrContextCancelled
			}
		}
	} else {
		r.tokens -= tokensNeeded
	}

	r.lastTime = time.Now()
	return nil
}

// waitForTokens 等待足够的令牌
func (w *RateLimiterWriter) waitForTokens(tokensNeeded int) error {
	w.rateMutex.Lock()
	defer w.rateMutex.Unlock()

	now := time.Now()
	if w.lastTime.IsZero() {
		w.lastTime = now
		w.tokens = constants.MinBurstSize
	}

	// 计算从上次到现在应该产生的令牌数
	elapsed := now.Sub(w.lastTime)
	tokensToAdd := int(float64(w.rate) * elapsed.Seconds())
	w.tokens += tokensToAdd

	// 限制令牌数量不超过burst大小
	burstSize := int(float64(w.rate) / float64(constants.DefaultBurstRatio))
	if burstSize < constants.MinBurstSize {
		burstSize = constants.MinBurstSize
	}
	if w.tokens > burstSize {
		w.tokens = burstSize
	}

	// 如果令牌不足，需要等待
	if w.tokens < tokensNeeded {
		tokensNeeded -= w.tokens
		w.tokens = 0

		// 计算需要等待的时间
		waitTime := time.Duration(float64(time.Second) * float64(tokensNeeded) / float64(w.rate))
		if waitTime > 0 {
			select {
			case <-time.After(waitTime):
			case <-w.Ctx().Done():
				return errors.ErrContextCancelled
			}
		}
	} else {
		w.tokens -= tokensNeeded
	}

	w.lastTime = time.Now()
	return nil
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
		return constants.MinBurstSize
	}

	burst := int(bytesPerSecond / int64(constants.DefaultBurstRatio))
	if burst < constants.MinBurstSize {
		burst = constants.MinBurstSize
	}
	return burst
}

// NewRateLimiterReader 创建限速读取器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiterReader(reader io.Reader, bytesPerSecond int64, ctx context.Context) (*RateLimiterReader, error) {
	if bytesPerSecond <= 0 {
		return nil, errors.NewRateLimitError(bytesPerSecond, "invalid rate limit", errors.ErrInvalidRate)
	}

	limiter := &RateLimiterReader{
		reader: reader,
		rate:   bytesPerSecond,
	}
	limiter.SetCtx(ctx, limiter.onClose)
	return limiter, nil
}

// NewRateLimiterWriter 创建限速写入器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64, ctx context.Context) (*RateLimiterWriter, error) {
	if bytesPerSecond <= 0 {
		return nil, errors.NewRateLimitError(bytesPerSecond, "invalid rate limit", errors.ErrInvalidRate)
	}

	limiter := &RateLimiterWriter{
		writer: writer,
		rate:   bytesPerSecond,
	}
	limiter.SetCtx(ctx, limiter.onClose)
	return limiter, nil
}

// NewRateLimiter 创建同时支持读写限速的限速器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiter(bytesPerSecond int64, parentCtx context.Context) (*RateLimiter, error) {
	if bytesPerSecond <= 0 {
		return nil, errors.NewRateLimitError(bytesPerSecond, "invalid rate limit", errors.ErrInvalidRate)
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
		return 0, errors.ErrReaderNil
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > constants.DefaultChunkSize {
		chunkSize = constants.DefaultChunkSize
	}

	// 等待限速器允许读取
	err = r.limiter.WaitN(r.Ctx(), chunkSize)
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
	err = r.limiter.WaitN(r.Ctx(), chunkSize)
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
	// 限速器通常不需要特殊清理，但可以在这里添加日志或其他清理逻辑
}

// SetRate 设置新的速率限制
func (r *RateLimiterReader) SetRate(bytesPerSecond int64) error {
	if bytesPerSecond <= 0 {
		return errors.NewRateLimitError(bytesPerSecond, "invalid rate limit", errors.ErrInvalidRate)
	}

	r.rateMutex.Lock()
	defer r.rateMutex.Unlock()

	r.rate = bytesPerSecond
	r.lastTime = time.Now()
	r.tokens = constants.MinBurstSize

	return nil
}

// SetRate 设置新的速率限制
func (w *RateLimiterWriter) SetRate(bytesPerSecond int64) error {
	if bytesPerSecond <= 0 {
		return errors.NewRateLimitError(bytesPerSecond, "invalid rate limit", errors.ErrInvalidRate)
	}

	w.rateMutex.Lock()
	defer w.rateMutex.Unlock()

	w.rate = bytesPerSecond
	w.lastTime = time.Now()
	w.tokens = constants.MinBurstSize

	return nil
}
