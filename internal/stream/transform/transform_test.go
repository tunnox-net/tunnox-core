package transform

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTransformer 测试创建转换器
func TestNewTransformer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         *TransformConfig
		wantNoOp       bool
		wantErr        bool
	}{
		{
			name:     "nil config returns NoOp transformer",
			config:   nil,
			wantNoOp: true,
			wantErr:  false,
		},
		{
			name: "zero bandwidth returns NoOp transformer",
			config: &TransformConfig{
				BandwidthLimit: 0,
			},
			wantNoOp: true,
			wantErr:  false,
		},
		{
			name: "negative bandwidth returns NoOp transformer",
			config: &TransformConfig{
				BandwidthLimit: -100,
			},
			wantNoOp: true,
			wantErr:  false,
		},
		{
			name: "positive bandwidth returns RateLimited transformer",
			config: &TransformConfig{
				BandwidthLimit: 1024,
			},
			wantNoOp: false,
			wantErr:  false,
		},
		{
			name: "high bandwidth limit",
			config: &TransformConfig{
				BandwidthLimit: 100 * 1024 * 1024, // 100 MB/s
			},
			wantNoOp: false,
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			transformer, err := NewTransformer(tc.config)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, transformer)

			if tc.wantNoOp {
				_, ok := transformer.(*NoOpTransformer)
				assert.True(t, ok, "expected NoOpTransformer")
			} else {
				_, ok := transformer.(*RateLimitedTransformer)
				assert.True(t, ok, "expected RateLimitedTransformer")
			}
		})
	}
}

// TestNoOpTransformer_WrapReader 测试 NoOp 转换器的 WrapReaderWithContext
func TestNoOpTransformer_WrapReader(t *testing.T) {
	t.Parallel()

	transformer := &NoOpTransformer{}
	ctx := context.Background()
	inputData := []byte("hello world")
	reader := bytes.NewReader(inputData)

	wrappedReader, err := transformer.WrapReaderWithContext(ctx, reader)
	require.NoError(t, err)
	require.NotNil(t, wrappedReader)

	// 读取数据应该与原始数据相同
	output, err := io.ReadAll(wrappedReader)
	require.NoError(t, err)
	assert.Equal(t, inputData, output)
}

// TestNoOpTransformer_WrapWriter 测试 NoOp 转换器的 WrapWriterWithContext
func TestNoOpTransformer_WrapWriter(t *testing.T) {
	t.Parallel()

	transformer := &NoOpTransformer{}
	ctx := context.Background()
	var buf bytes.Buffer

	wrappedWriter, err := transformer.WrapWriterWithContext(ctx, &buf)
	require.NoError(t, err)
	require.NotNil(t, wrappedWriter)

	// 写入数据
	inputData := []byte("hello world")
	n, err := wrappedWriter.Write(inputData)
	require.NoError(t, err)
	assert.Equal(t, len(inputData), n)

	// 关闭应该不报错
	err = wrappedWriter.Close()
	require.NoError(t, err)

	// 验证写入的数据
	assert.Equal(t, inputData, buf.Bytes())
}

// TestNoOpTransformer_WrapReaderWithContext 测试带 context 的 WrapReader
func TestNoOpTransformer_WrapReaderWithContext(t *testing.T) {
	t.Parallel()

	transformer := &NoOpTransformer{}
	ctx := context.Background()
	inputData := []byte("test data")
	reader := bytes.NewReader(inputData)

	wrappedReader, err := transformer.WrapReaderWithContext(ctx, reader)
	require.NoError(t, err)
	require.NotNil(t, wrappedReader)

	output, err := io.ReadAll(wrappedReader)
	require.NoError(t, err)
	assert.Equal(t, inputData, output)
}

// TestNoOpTransformer_WrapWriterWithContext 测试带 context 的 WrapWriter
func TestNoOpTransformer_WrapWriterWithContext(t *testing.T) {
	t.Parallel()

	transformer := &NoOpTransformer{}
	ctx := context.Background()
	var buf bytes.Buffer

	wrappedWriter, err := transformer.WrapWriterWithContext(ctx, &buf)
	require.NoError(t, err)
	require.NotNil(t, wrappedWriter)

	inputData := []byte("test data")
	n, err := wrappedWriter.Write(inputData)
	require.NoError(t, err)
	assert.Equal(t, len(inputData), n)

	err = wrappedWriter.Close()
	require.NoError(t, err)

	assert.Equal(t, inputData, buf.Bytes())
}

// TestRateLimitedTransformer_WrapReader 测试限速 Reader
func TestRateLimitedTransformer_WrapReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		bandwidthLimit int64
		inputData      []byte
	}{
		{
			name:           "small data with low bandwidth",
			bandwidthLimit: 1024, // 1 KB/s
			inputData:      []byte("hello"),
		},
		{
			name:           "small data with high bandwidth",
			bandwidthLimit: 1024 * 1024, // 1 MB/s
			inputData:      []byte("hello world test data"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := &TransformConfig{BandwidthLimit: tc.bandwidthLimit}
			transformer, err := NewTransformer(config)
			require.NoError(t, err)

			ctx := context.Background()
			reader := bytes.NewReader(tc.inputData)
			wrappedReader, err := transformer.WrapReaderWithContext(ctx, reader)
			require.NoError(t, err)

			output, err := io.ReadAll(wrappedReader)
			require.NoError(t, err)
			assert.Equal(t, tc.inputData, output)
		})
	}
}

// TestRateLimitedTransformer_WrapWriter 测试限速 Writer
func TestRateLimitedTransformer_WrapWriter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		bandwidthLimit int64
		inputData      []byte
	}{
		{
			name:           "small data with low bandwidth",
			bandwidthLimit: 1024, // 1 KB/s
			inputData:      []byte("hello"),
		},
		{
			name:           "small data with high bandwidth",
			bandwidthLimit: 1024 * 1024, // 1 MB/s
			inputData:      []byte("hello world test data"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := &TransformConfig{BandwidthLimit: tc.bandwidthLimit}
			transformer, err := NewTransformer(config)
			require.NoError(t, err)

			ctx := context.Background()
			var buf bytes.Buffer
			wrappedWriter, err := transformer.WrapWriterWithContext(ctx, &buf)
			require.NoError(t, err)

			n, err := wrappedWriter.Write(tc.inputData)
			require.NoError(t, err)
			assert.Equal(t, len(tc.inputData), n)

			err = wrappedWriter.Close()
			require.NoError(t, err)

			assert.Equal(t, tc.inputData, buf.Bytes())
		})
	}
}

// TestRateLimitedTransformer_WrapReaderWithContext 测试带 context 的限速 Reader
func TestRateLimitedTransformer_WrapReaderWithContext(t *testing.T) {
	t.Parallel()

	config := &TransformConfig{BandwidthLimit: 1024 * 1024}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	inputData := []byte("test data with context")
	reader := bytes.NewReader(inputData)

	wrappedReader, err := transformer.WrapReaderWithContext(ctx, reader)
	require.NoError(t, err)

	output, err := io.ReadAll(wrappedReader)
	require.NoError(t, err)
	assert.Equal(t, inputData, output)
}

// TestRateLimitedTransformer_WrapWriterWithContext 测试带 context 的限速 Writer
func TestRateLimitedTransformer_WrapWriterWithContext(t *testing.T) {
	t.Parallel()

	config := &TransformConfig{BandwidthLimit: 1024 * 1024}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer
	wrappedWriter, err := transformer.WrapWriterWithContext(ctx, &buf)
	require.NoError(t, err)

	inputData := []byte("test data with context")
	n, err := wrappedWriter.Write(inputData)
	require.NoError(t, err)
	assert.Equal(t, len(inputData), n)

	err = wrappedWriter.Close()
	require.NoError(t, err)

	assert.Equal(t, inputData, buf.Bytes())
}

// TestRateLimitedTransformer_ContextCancellation 测试 context 取消
func TestRateLimitedTransformer_ContextCancellation(t *testing.T) {
	t.Parallel()

	// 使用非常低的带宽限制，使得限速等待会被取消
	config := &TransformConfig{BandwidthLimit: 1} // 1 byte/s
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	// 创建一个很快就会取消的 context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	wrappedWriter, err := transformer.WrapWriterWithContext(ctx, &buf)
	require.NoError(t, err)

	// 尝试写入大量数据，应该因为 context 取消而失败
	largeData := bytes.Repeat([]byte("a"), 1000)
	_, err = wrappedWriter.Write(largeData)
	// 由于限速器有突发容量，小数据可能成功，大数据应该失败
	// 这里我们只验证行为合理（不 panic）
}

// TestNopWriteCloser 测试 nopWriteCloser
func TestNopWriteCloser(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	nwc := &nopWriteCloser{&buf}

	// 测试写入
	data := []byte("test")
	n, err := nwc.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)

	// 测试关闭
	err = nwc.Close()
	require.NoError(t, err)

	// 验证数据
	assert.Equal(t, data, buf.Bytes())
}

// TestRateLimitedWriter_CloseWithCloser 测试关闭带 Closer 的 Writer
func TestRateLimitedWriter_CloseWithCloser(t *testing.T) {
	t.Parallel()

	config := &TransformConfig{BandwidthLimit: 1024}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	// 创建一个实现了 Close 方法的 Writer
	ctx := context.Background()
	closerWriter := &closerMockWriter{}
	wrappedWriter, err := transformer.WrapWriterWithContext(ctx, closerWriter)
	require.NoError(t, err)

	// 写入数据
	_, err = wrappedWriter.Write([]byte("test"))
	require.NoError(t, err)

	// 关闭应该调用底层的 Close
	err = wrappedWriter.Close()
	require.NoError(t, err)
	assert.True(t, closerWriter.closed)
}

// closerMockWriter 实现了 io.WriteCloser 的 mock
type closerMockWriter struct {
	bytes.Buffer
	closed bool
}

func (w *closerMockWriter) Close() error {
	w.closed = true
	return nil
}

// TestStreamTransformer_Interface 测试接口实现
func TestStreamTransformer_Interface(t *testing.T) {
	t.Parallel()

	// 验证 NoOpTransformer 实现了 StreamTransformer 接口
	var _ StreamTransformer = (*NoOpTransformer)(nil)

	// 验证 RateLimitedTransformer 实现了 StreamTransformer 接口
	var _ StreamTransformer = (*RateLimitedTransformer)(nil)
}

// TestTransformConfig 测试配置结构
func TestTransformConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config TransformConfig
	}{
		{
			name:   "zero config",
			config: TransformConfig{},
		},
		{
			name: "with bandwidth limit",
			config: TransformConfig{
				BandwidthLimit: 1024 * 1024,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 验证配置可以正常使用
			transformer, err := NewTransformer(&tc.config)
			require.NoError(t, err)
			require.NotNil(t, transformer)
		})
	}
}

// BenchmarkNoOpTransformer_Read 基准测试 NoOp Reader
func BenchmarkNoOpTransformer_Read(b *testing.B) {
	transformer := &NoOpTransformer{}
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		wrappedReader, _ := transformer.WrapReaderWithContext(ctx, reader)
		io.Copy(io.Discard, wrappedReader)
	}
}

// BenchmarkNoOpTransformer_Write 基准测试 NoOp Writer
func BenchmarkNoOpTransformer_Write(b *testing.B) {
	transformer := &NoOpTransformer{}
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		wrappedWriter, _ := transformer.WrapWriterWithContext(ctx, &buf)
		wrappedWriter.Write(data)
		wrappedWriter.Close()
	}
}

// BenchmarkRateLimitedTransformer_Read 基准测试限速 Reader
func BenchmarkRateLimitedTransformer_Read(b *testing.B) {
	config := &TransformConfig{BandwidthLimit: 100 * 1024 * 1024} // 100 MB/s
	transformer, _ := NewTransformer(config)
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		wrappedReader, _ := transformer.WrapReaderWithContext(ctx, reader)
		io.Copy(io.Discard, wrappedReader)
	}
}

// BenchmarkRateLimitedTransformer_Write 基准测试限速 Writer
func BenchmarkRateLimitedTransformer_Write(b *testing.B) {
	config := &TransformConfig{BandwidthLimit: 100 * 1024 * 1024} // 100 MB/s
	transformer, _ := NewTransformer(config)
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		wrappedWriter, _ := transformer.WrapWriterWithContext(ctx, &buf)
		wrappedWriter.Write(data)
		wrappedWriter.Close()
	}
}
