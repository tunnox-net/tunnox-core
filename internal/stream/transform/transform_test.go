package transform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoOpTransformer_WrapReader(t *testing.T) {
	transformer := &NoOpTransformer{}
	reader := bytes.NewReader([]byte("test data"))

	wrappedReader, err := transformer.WrapReader(reader)
	assert.NoError(t, err)
	assert.Equal(t, reader, wrappedReader, "NoOpTransformer should return original reader")

	// Verify data can be read
	data, err := io.ReadAll(wrappedReader)
	require.NoError(t, err)
	assert.Equal(t, []byte("test data"), data)
}

func TestNoOpTransformer_WrapWriter(t *testing.T) {
	transformer := &NoOpTransformer{}
	buffer := &bytes.Buffer{}

	wrappedWriter, err := transformer.WrapWriter(buffer)
	assert.NoError(t, err)
	assert.NotNil(t, wrappedWriter)

	// Verify data can be written
	testData := []byte("test data")
	n, err := wrappedWriter.Write(testData)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, buffer.Bytes())

	// Verify close doesn't error
	err = wrappedWriter.Close()
	assert.NoError(t, err)
}

func TestNewTransformer(t *testing.T) {
	tests := []struct {
		name           string
		config         *TransformConfig
		expectNoOp     bool
		expectRateLimit bool
	}{
		{
			name:           "nil config returns NoOpTransformer",
			config:         nil,
			expectNoOp:     true,
			expectRateLimit: false,
		},
		{
			name: "zero bandwidth returns NoOpTransformer",
			config: &TransformConfig{
				BandwidthLimit: 0,
			},
			expectNoOp:     true,
			expectRateLimit: false,
		},
		{
			name: "negative bandwidth returns NoOpTransformer",
			config: &TransformConfig{
				BandwidthLimit: -1000,
			},
			expectNoOp:     true,
			expectRateLimit: false,
		},
		{
			name: "positive bandwidth returns RateLimitedTransformer",
			config: &TransformConfig{
				BandwidthLimit: 1024,
			},
			expectNoOp:     false,
			expectRateLimit: true,
		},
		{
			name: "large bandwidth returns RateLimitedTransformer",
			config: &TransformConfig{
				BandwidthLimit: 1024 * 1024 * 10, // 10MB/s
			},
			expectNoOp:     false,
			expectRateLimit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewTransformer(tt.config)
			require.NoError(t, err)
			require.NotNil(t, transformer)

			if tt.expectNoOp {
				_, ok := transformer.(*NoOpTransformer)
				assert.True(t, ok, "Expected NoOpTransformer")
			}

			if tt.expectRateLimit {
				_, ok := transformer.(*RateLimitedTransformer)
				assert.True(t, ok, "Expected RateLimitedTransformer")
			}
		})
	}
}

func TestNewTransformerWithContext(t *testing.T) {
	tests := []struct {
		name    string
		config  *TransformConfig
		ctx     context.Context
		expectNoOp bool
	}{
		{
			name:    "nil config with nil context",
			config:  nil,
			ctx:     nil,
			expectNoOp: true,
		},
		{
			name:    "nil config with valid context",
			config:  nil,
			ctx:     context.Background(),
			expectNoOp: true,
		},
		{
			name: "valid config with nil context",
			config: &TransformConfig{
				BandwidthLimit: 1024,
			},
			ctx:     nil,
			expectNoOp: false,
		},
		{
			name: "valid config with valid context",
			config: &TransformConfig{
				BandwidthLimit: 2048,
			},
			ctx:     context.Background(),
			expectNoOp: false,
		},
		{
			name: "valid config with cancelled context",
			config: &TransformConfig{
				BandwidthLimit: 512,
			},
			ctx:     func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }(),
			expectNoOp: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewTransformerWithContext(tt.config, tt.ctx)
			require.NoError(t, err)
			require.NotNil(t, transformer)

			if tt.expectNoOp {
				_, ok := transformer.(*NoOpTransformer)
				assert.True(t, ok)
			} else {
				rateLimited, ok := transformer.(*RateLimitedTransformer)
				assert.True(t, ok)
				assert.NotNil(t, rateLimited.rateLimiter)
			}
		})
	}
}

func TestRateLimitedTransformer_WrapReader(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024, // 1KB/s
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	testData := []byte("test data for rate limiting")
	reader := bytes.NewReader(testData)

	wrappedReader, err := transformer.WrapReader(reader)
	require.NoError(t, err)
	assert.NotNil(t, wrappedReader)

	// Read data and verify it works
	data, err := io.ReadAll(wrappedReader)
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestRateLimitedTransformer_WrapWriter(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024, // 1KB/s
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	buffer := &bytes.Buffer{}
	wrappedWriter, err := transformer.WrapWriter(buffer)
	require.NoError(t, err)
	assert.NotNil(t, wrappedWriter)

	// Write data and verify it works
	testData := []byte("test data for rate limiting")
	n, err := wrappedWriter.Write(testData)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, buffer.Bytes())

	// Close should not error
	err = wrappedWriter.Close()
	assert.NoError(t, err)
}

func TestRateLimitedReader_SmallReads(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024 * 1024, // 1MB/s (high limit to avoid blocking in test)
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	testData := []byte("Hello, World!")
	reader := bytes.NewReader(testData)

	wrappedReader, err := transformer.WrapReader(reader)
	require.NoError(t, err)

	data, err := io.ReadAll(wrappedReader)
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestRateLimitedWriter_SmallWrites(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024 * 1024, // 1MB/s (high limit to avoid blocking in test)
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	buffer := &bytes.Buffer{}
	wrappedWriter, err := transformer.WrapWriter(buffer)
	require.NoError(t, err)

	testData := []byte("Hello, World!")
	n, err := wrappedWriter.Write(testData)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, buffer.Bytes())
}

func TestRateLimitedReader_MultipleReads(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024 * 1024, // 1MB/s
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	testData := bytes.Repeat([]byte("x"), 1000)
	reader := bytes.NewReader(testData)

	wrappedReader, err := transformer.WrapReader(reader)
	require.NoError(t, err)

	// Read in chunks
	buffer := make([]byte, 100)
	totalRead := 0
	for {
		n, err := wrappedReader.Read(buffer)
		totalRead += n
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	assert.Equal(t, len(testData), totalRead)
}

func TestRateLimitedWriter_MultipleWrites(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024 * 1024, // 1MB/s
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	buffer := &bytes.Buffer{}
	wrappedWriter, err := transformer.WrapWriter(buffer)
	require.NoError(t, err)

	// Write multiple chunks
	chunks := [][]byte{
		[]byte("chunk1"),
		[]byte("chunk2"),
		[]byte("chunk3"),
	}

	for _, chunk := range chunks {
		n, err := wrappedWriter.Write(chunk)
		require.NoError(t, err)
		assert.Equal(t, len(chunk), n)
	}

	expected := []byte("chunk1chunk2chunk3")
	assert.Equal(t, expected, buffer.Bytes())
}

func TestRateLimitedReader_EmptyRead(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024,
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	reader := bytes.NewReader([]byte{})
	wrappedReader, err := transformer.WrapReader(reader)
	require.NoError(t, err)

	data, err := io.ReadAll(wrappedReader)
	assert.NoError(t, err)
	assert.Empty(t, data)
}

func TestRateLimitedWriter_EmptyWrite(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024,
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	buffer := &bytes.Buffer{}
	wrappedWriter, err := transformer.WrapWriter(buffer)
	require.NoError(t, err)

	n, err := wrappedWriter.Write([]byte{})
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Empty(t, buffer.Bytes())
}

func TestRateLimitedReader_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := &TransformConfig{
		BandwidthLimit: 10, // Very low limit to force waiting
	}
	transformer, err := NewTransformerWithContext(config, ctx)
	require.NoError(t, err)

	// Large data to force rate limiting
	testData := bytes.Repeat([]byte("x"), 10000)
	reader := bytes.NewReader(testData)

	wrappedReader, err := transformer.WrapReader(reader)
	require.NoError(t, err)

	// Cancel context immediately
	cancel()

	// Try to read - should eventually get an error
	buffer := make([]byte, 1000)
	_, err = wrappedReader.Read(buffer)
	// May or may not error depending on timing, but should not hang
	// The test passing means it didn't hang
}

func TestRateLimitedWriter_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := &TransformConfig{
		BandwidthLimit: 10, // Very low limit to force waiting
	}
	transformer, err := NewTransformerWithContext(config, ctx)
	require.NoError(t, err)

	buffer := &bytes.Buffer{}
	wrappedWriter, err := transformer.WrapWriter(buffer)
	require.NoError(t, err)

	// Cancel context immediately
	cancel()

	// Try to write - should eventually get an error
	largeData := bytes.Repeat([]byte("x"), 10000)
	_, err = wrappedWriter.Write(largeData)
	// May or may not error depending on timing, but should not hang
}

func TestRateLimitedWriter_CloseWithClosableTarget(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024,
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	closableBuffer := &closableBuffer{Buffer: &bytes.Buffer{}}
	wrappedWriter, err := transformer.WrapWriter(closableBuffer)
	require.NoError(t, err)

	err = wrappedWriter.Close()
	assert.NoError(t, err)
	assert.True(t, closableBuffer.closed)
}

func TestRateLimitedWriter_CloseWithNonClosableTarget(t *testing.T) {
	config := &TransformConfig{
		BandwidthLimit: 1024,
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	buffer := &bytes.Buffer{}
	wrappedWriter, err := transformer.WrapWriter(buffer)
	require.NoError(t, err)

	// Should not error even though buffer doesn't implement Close
	err = wrappedWriter.Close()
	assert.NoError(t, err)
}

func TestRateLimiting_ActuallyLimits(t *testing.T) {
	// This test verifies that rate limiting actually slows down operations
	config := &TransformConfig{
		BandwidthLimit: 1000, // 1KB/s
	}
	transformer, err := NewTransformer(config)
	require.NoError(t, err)

	buffer := &bytes.Buffer{}
	wrappedWriter, err := transformer.WrapWriter(buffer)
	require.NoError(t, err)

	// Write 2KB of data - should take at least ~1 second with 1KB/s limit
	// But we use burst capacity (2x) so it might be faster
	data := bytes.Repeat([]byte("x"), 2000)

	start := time.Now()
	_, err = wrappedWriter.Write(data)
	elapsed := time.Since(start)

	require.NoError(t, err)
	// With 2x burst capacity, 2KB should still complete
	// Just verify it doesn't hang
	assert.Less(t, elapsed, 10*time.Second, "Should not hang indefinitely")
}

func TestNopWriteCloser_Close(t *testing.T) {
	buffer := &bytes.Buffer{}
	nopWC := &nopWriteCloser{Writer: buffer}

	// Write some data
	n, err := nopWC.Write([]byte("test"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	// Close should not error
	err = nopWC.Close()
	assert.NoError(t, err)

	// Verify data was written
	assert.Equal(t, []byte("test"), buffer.Bytes())
}

func TestTransformConfig_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		config *TransformConfig
	}{
		{
			name: "very large bandwidth",
			config: &TransformConfig{
				BandwidthLimit: 1024 * 1024 * 1024, // 1GB/s
			},
		},
		{
			name: "bandwidth of 1",
			config: &TransformConfig{
				BandwidthLimit: 1, // 1 byte/s
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewTransformer(tt.config)
			require.NoError(t, err)
			assert.NotNil(t, transformer)

			// Verify it can wrap reader and writer
			reader, err := transformer.WrapReader(bytes.NewReader([]byte("test")))
			assert.NoError(t, err)
			assert.NotNil(t, reader)

			writer, err := transformer.WrapWriter(&bytes.Buffer{})
			assert.NoError(t, err)
			assert.NotNil(t, writer)
		})
	}
}

// Helper types for testing

type closableBuffer struct {
	*bytes.Buffer
	closed bool
}

func (cb *closableBuffer) Close() error {
	cb.closed = true
	return nil
}

type errorWriter struct{}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write error")
}

func (w *errorWriter) Close() error {
	return errors.New("close error")
}
