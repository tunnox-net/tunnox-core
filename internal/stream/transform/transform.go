package transform

import (
	"context"
	"io"
	"time"

	coreErrors "tunnox-core/internal/core/errors"
	"golang.org/x/time/rate"
)

// StreamTransformer 流转换器接口
// 注意：压缩和加密已移至StreamProcessor，Transform只处理限速等商业特性
type StreamTransformer interface {
	// WrapReader 包装 Reader（限速）
	WrapReader(r io.Reader) (io.Reader, error)

	// WrapWriter 包装 Writer（限速）
	WrapWriter(w io.Writer) (io.WriteCloser, error)
}

// TransformConfig 转换配置
type TransformConfig struct {
	// 限速配置（字节/秒，0表示不限制）
	BandwidthLimit int64
}

// NoOpTransformer 无操作转换器（不限速）
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

// RateLimitedTransformer 限速转换器
type RateLimitedTransformer struct {
	config      TransformConfig
	rateLimiter *rate.Limiter
	ctx         context.Context // 保存 context 用于超时控制
}

// NewTransformer 创建转换器
func NewTransformer(config *TransformConfig) (StreamTransformer, error) {
	return NewTransformerWithContext(config, nil)
}

// NewTransformerWithContext 创建转换器（带 context）
func NewTransformerWithContext(config *TransformConfig, ctx context.Context) (StreamTransformer, error) {
	if config == nil || config.BandwidthLimit <= 0 {
		return &NoOpTransformer{}, nil
	}

	// 创建令牌桶限速器，允许2倍突发流量
	limiter := rate.NewLimiter(rate.Limit(config.BandwidthLimit), int(config.BandwidthLimit*2))

	return &RateLimitedTransformer{
		config:      *config,
		rateLimiter: limiter,
		ctx:         ctx,
	}, nil
}

// WrapReader 包装 Reader（添加限速）
func (t *RateLimitedTransformer) WrapReader(r io.Reader) (io.Reader, error) {
	return &rateLimitedReader{
		source:  r,
		limiter: t.rateLimiter,
		ctx:     t.ctx,
	}, nil
}

// WrapWriter 包装 Writer（添加限速）
func (t *RateLimitedTransformer) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	return &rateLimitedWriter{
		target:  w,
		limiter: t.rateLimiter,
		ctx:     t.ctx,
	}, nil
}

// rateLimitedReader 限速Reader
type rateLimitedReader struct {
	source  io.Reader
	limiter *rate.Limiter
	ctx     context.Context // 保存 context 用于超时控制
}

func (r *rateLimitedReader) Read(p []byte) (n int, err error) {
	n, err = r.source.Read(p)
	if n > 0 {
		// 等待令牌（5秒超时）
		// 如果提供了 context，使用它作为父 context；否则使用 Background（仅用于超时控制）
		baseCtx := r.ctx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
		if waitErr := r.limiter.WaitN(ctx, n); waitErr != nil {
			cancel()
			return n, coreErrors.Wrap(waitErr, coreErrors.ErrorTypeTemporary, "rate limit wait failed")
		}
		cancel()
	}
	return n, err
}

// rateLimitedWriter 限速Writer
type rateLimitedWriter struct {
	target  io.Writer
	limiter *rate.Limiter
	ctx     context.Context // 保存 context 用于超时控制
}

func (w *rateLimitedWriter) Write(p []byte) (n int, err error) {
	// 等待令牌（5秒超时）
	// 如果提供了 context，使用它作为父 context；否则使用 Background（仅用于超时控制）
	baseCtx := w.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	if waitErr := w.limiter.WaitN(ctx, len(p)); waitErr != nil {
		cancel()
		return 0, coreErrors.Wrap(waitErr, coreErrors.ErrorTypeTemporary, "rate limit wait failed")
	}
	cancel()

	return w.target.Write(p)
}

func (w *rateLimitedWriter) Close() error {
	// 如果target支持Close，则关闭
	if closer, ok := w.target.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
