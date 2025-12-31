package transform

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/time/rate"
)

// StreamTransformer 流转换器接口
// 注意：压缩和加密已移至StreamProcessor，Transform只处理限速等商业特性
type StreamTransformer interface {
	// WrapReaderWithContext 包装 Reader（限速），支持 context 取消
	WrapReaderWithContext(ctx context.Context, r io.Reader) (io.Reader, error)

	// WrapWriterWithContext 包装 Writer（限速），支持 context 取消
	WrapWriterWithContext(ctx context.Context, w io.Writer) (io.WriteCloser, error)
}

// TransformConfig 转换配置
type TransformConfig struct {
	// 限速配置（字节/秒，0表示不限制）
	BandwidthLimit int64
}

// NoOpTransformer 无操作转换器（不限速）
type NoOpTransformer struct{}

func (t *NoOpTransformer) WrapReaderWithContext(ctx context.Context, r io.Reader) (io.Reader, error) {
	return r, nil
}

func (t *NoOpTransformer) WrapWriterWithContext(ctx context.Context, w io.Writer) (io.WriteCloser, error) {
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
}

// NewTransformer 创建转换器
func NewTransformer(config *TransformConfig) (StreamTransformer, error) {
	if config == nil || config.BandwidthLimit <= 0 {
		return &NoOpTransformer{}, nil
	}

	// 创建令牌桶限速器，允许2倍突发流量
	limiter := rate.NewLimiter(rate.Limit(config.BandwidthLimit), int(config.BandwidthLimit*2))

	return &RateLimitedTransformer{
		config:      *config,
		rateLimiter: limiter,
	}, nil
}

// WrapReaderWithContext 包装 Reader（添加限速），支持 context 取消
func (t *RateLimitedTransformer) WrapReaderWithContext(ctx context.Context, r io.Reader) (io.Reader, error) {
	return &rateLimitedReader{
		source:    r,
		limiter:   t.rateLimiter,
		parentCtx: ctx,
	}, nil
}

// WrapWriterWithContext 包装 Writer（添加限速），支持 context 取消
func (t *RateLimitedTransformer) WrapWriterWithContext(ctx context.Context, w io.Writer) (io.WriteCloser, error) {
	return &rateLimitedWriter{
		target:    w,
		limiter:   t.rateLimiter,
		parentCtx: ctx,
	}, nil
}

// rateLimitedReader 限速Reader
type rateLimitedReader struct {
	source    io.Reader
	limiter   *rate.Limiter
	parentCtx context.Context
}

func (r *rateLimitedReader) Read(p []byte) (n int, err error) {
	n, err = r.source.Read(p)
	if n > 0 {
		// 等待令牌，使用 parentCtx 派生的超时 context
		ctx, cancel := context.WithTimeout(r.parentCtx, 5*time.Second)
		if waitErr := r.limiter.WaitN(ctx, n); waitErr != nil {
			cancel()
			return n, fmt.Errorf("rate limit wait failed: %w", waitErr)
		}
		cancel()
	}
	return n, err
}

// rateLimitedWriter 限速Writer
type rateLimitedWriter struct {
	target    io.Writer
	limiter   *rate.Limiter
	parentCtx context.Context
}

func (w *rateLimitedWriter) Write(p []byte) (n int, err error) {
	// 等待令牌，使用 parentCtx 派生的超时 context
	ctx, cancel := context.WithTimeout(w.parentCtx, 5*time.Second)
	if waitErr := w.limiter.WaitN(ctx, len(p)); waitErr != nil {
		cancel()
		return 0, fmt.Errorf("rate limit wait failed: %w", waitErr)
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
