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
	if chunkSize > 1024 {
		chunkSize = 1024 // 限制单次读取的最大块大小
	}

	// 等待限速器允许读取
	err = r.limiter.WaitN(r.Ctx(), chunkSize)
	if err != nil {
		return 0, err
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
		return 0, io.ErrClosedPipe
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > 1024 {
		chunkSize = 1024 // 限制单次写入的最大块大小
	}

	// 等待限速器允许写入
	err = w.limiter.WaitN(w.Ctx(), chunkSize)
	if err != nil {
		return 0, err
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

// NewRateLimiterReader 创建限速读取器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiterReader(reader io.Reader, bytesPerSecond int64, parentCtx context.Context) *RateLimiterReader {
	// 设置burst为速率的一半，但至少为1024字节
	burst := int(bytesPerSecond / 2)
	if burst < 1024 {
		burst = 1024
	}
	limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
	r := &RateLimiterReader{
		reader:  reader,
		limiter: limiter,
	}
	r.SetCtx(parentCtx, r.onClose)
	return r
}

// NewRateLimiterWriter 创建限速写入器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiterWriter(writer io.Writer, bytesPerSecond int64, parentCtx context.Context) *RateLimiterWriter {
	// 设置burst为速率的一半，但至少为1024字节
	burst := int(bytesPerSecond / 2)
	if burst < 1024 {
		burst = 1024
	}
	limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
	w := &RateLimiterWriter{
		writer:  writer,
		limiter: limiter,
	}
	w.SetCtx(parentCtx, w.onClose)
	return w
}

// NewRateLimiter 创建同时支持读写限速的限速器
// bytesPerSecond: 每秒允许的字节数
func NewRateLimiter(bytesPerSecond int64, parentCtx context.Context) *RateLimiter {
	// 设置burst为速率的一半，但至少为1024字节
	burst := int(bytesPerSecond / 2)
	if burst < 1024 {
		burst = 1024
	}
	limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
	r := &RateLimiter{
		limiter: limiter,
	}
	r.SetCtx(parentCtx, r.onClose)
	return r
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
		return 0, io.ErrClosedPipe
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > 1024 {
		chunkSize = 1024 // 限制单次读取的最大块大小
	}

	// 等待限速器允许读取
	err = r.limiter.WaitN(r.Ctx(), chunkSize)
	if err != nil {
		return 0, err
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
		return 0, io.ErrClosedPipe
	}
	if r.writer == nil {
		return 0, io.ErrClosedPipe
	}

	// 分块处理，避免超出burst限制
	chunkSize := len(p)
	if chunkSize > 1024 {
		chunkSize = 1024 // 限制单次写入的最大块大小
	}

	// 等待限速器允许写入
	err = r.limiter.WaitN(r.Ctx(), chunkSize)
	if err != nil {
		return 0, err
	}

	// 只写入chunkSize大小的数据
	if len(p) > chunkSize {
		p = p[:chunkSize]
	}

	return r.writer.Write(p)
}

// onClose 资源释放
func (r *RateLimiter) onClose() {
	// 限速器本身不需要特殊清理
}

// SetRate 动态调整限速速率
func (r *RateLimiter) SetRate(bytesPerSecond int64) {
	burst := int(bytesPerSecond / 2)
	if burst < 1024 {
		burst = 1024
	}
	r.limiter.SetLimit(rate.Limit(bytesPerSecond))
	r.limiter.SetBurst(burst)
}

// SetRate 动态调整读取器限速速率
func (r *RateLimiterReader) SetRate(bytesPerSecond int64) {
	burst := int(bytesPerSecond / 2)
	if burst < 1024 {
		burst = 1024
	}
	r.limiter.SetLimit(rate.Limit(bytesPerSecond))
	r.limiter.SetBurst(burst)
}

// SetRate 动态调整写入器限速速率
func (w *RateLimiterWriter) SetRate(bytesPerSecond int64) {
	burst := int(bytesPerSecond / 2)
	if burst < 1024 {
		burst = 1024
	}
	w.limiter.SetLimit(rate.Limit(bytesPerSecond))
	w.limiter.SetBurst(burst)
}
