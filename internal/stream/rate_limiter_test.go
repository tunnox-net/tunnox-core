package stream

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewRateLimiter 测试创建限速器
func TestNewRateLimiter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		bytesPerSecond int64
		wantErr        bool
	}{
		{
			name:           "valid rate",
			bytesPerSecond: 1024,
			wantErr:        false,
		},
		{
			name:           "high rate",
			bytesPerSecond: 100 * 1024 * 1024,
			wantErr:        false,
		},
		{
			name:           "zero rate",
			bytesPerSecond: 0,
			wantErr:        true,
		},
		{
			name:           "negative rate",
			bytesPerSecond: -100,
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			rl, err := NewRateLimiter(tc.bytesPerSecond, ctx)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, rl)
			} else {
				require.NoError(t, err)
				require.NotNil(t, rl)
				rl.Close()
			}
		})
	}
}

// TestNewRateLimiterReader 测试创建限速读取器
func TestNewRateLimiterReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		bytesPerSecond int64
		wantErr        bool
	}{
		{
			name:           "valid rate",
			bytesPerSecond: 1024,
			wantErr:        false,
		},
		{
			name:           "zero rate",
			bytesPerSecond: 0,
			wantErr:        true,
		},
		{
			name:           "negative rate",
			bytesPerSecond: -100,
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			reader := bytes.NewBufferString("test data")
			rl, err := NewRateLimiterReader(reader, tc.bytesPerSecond, ctx)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, rl)
			} else {
				require.NoError(t, err)
				require.NotNil(t, rl)
				rl.Close()
			}
		})
	}
}

// TestNewRateLimiterWriter 测试创建限速写入器
func TestNewRateLimiterWriter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		bytesPerSecond int64
		wantErr        bool
	}{
		{
			name:           "valid rate",
			bytesPerSecond: 1024,
			wantErr:        false,
		},
		{
			name:           "zero rate",
			bytesPerSecond: 0,
			wantErr:        true,
		},
		{
			name:           "negative rate",
			bytesPerSecond: -100,
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			writer := &bytes.Buffer{}
			rl, err := NewRateLimiterWriter(writer, tc.bytesPerSecond, ctx)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, rl)
			} else {
				require.NoError(t, err)
				require.NotNil(t, rl)
				rl.Close()
			}
		})
	}
}

// TestRateLimiterReader_Read 测试限速读取
func TestRateLimiterReader_Read(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	inputData := []byte("hello world test data")
	reader := bytes.NewReader(inputData)

	// 使用高速率以避免测试超时
	rl, err := NewRateLimiterReader(reader, 1024*1024, ctx)
	require.NoError(t, err)
	defer rl.Close()

	// 读取数据
	output, err := io.ReadAll(rl)
	require.NoError(t, err)
	assert.Equal(t, inputData, output)
}

// TestRateLimiterWriter_Write 测试限速写入
func TestRateLimiterWriter_Write(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	inputData := []byte("hello world test data")
	writer := &bytes.Buffer{}

	// 使用高速率以避免测试超时
	rl, err := NewRateLimiterWriter(writer, 1024*1024, ctx)
	require.NoError(t, err)
	defer rl.Close()

	// 写入数据
	n, err := rl.Write(inputData)
	require.NoError(t, err)
	assert.Equal(t, len(inputData), n)
	assert.Equal(t, inputData, writer.Bytes())
}

// TestRateLimiter_Read 测试 RateLimiter Read
func TestRateLimiter_Read(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	inputData := []byte("hello world")
	reader := bytes.NewReader(inputData)

	rl, err := NewRateLimiter(1024*1024, ctx)
	require.NoError(t, err)
	defer rl.Close()

	rl.SetReader(reader)

	buf := make([]byte, 100)
	n, err := rl.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(inputData), n)
	assert.Equal(t, inputData, buf[:n])
}

// TestRateLimiter_Write 测试 RateLimiter Write
func TestRateLimiter_Write(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	inputData := []byte("hello world")
	writer := &bytes.Buffer{}

	rl, err := NewRateLimiter(1024*1024, ctx)
	require.NoError(t, err)
	defer rl.Close()

	rl.SetWriter(writer)

	n, err := rl.Write(inputData)
	require.NoError(t, err)
	assert.Equal(t, len(inputData), n)
	assert.Equal(t, inputData, writer.Bytes())
}

// TestRateLimiter_Read_NilReader 测试 nil reader
func TestRateLimiter_Read_NilReader(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rl, err := NewRateLimiter(1024, ctx)
	require.NoError(t, err)
	defer rl.Close()

	// 不设置 reader
	buf := make([]byte, 10)
	_, err = rl.Read(buf)
	assert.Error(t, err)
}

// TestRateLimiter_Write_NilWriter 测试 nil writer
func TestRateLimiter_Write_NilWriter(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rl, err := NewRateLimiter(1024, ctx)
	require.NoError(t, err)
	defer rl.Close()

	// 不设置 writer
	_, err = rl.Write([]byte("test"))
	assert.Error(t, err)
}

// TestRateLimiterReader_Read_Closed 测试关闭后读取
func TestRateLimiterReader_Read_Closed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reader := bytes.NewBufferString("test")
	rl, err := NewRateLimiterReader(reader, 1024, ctx)
	require.NoError(t, err)

	rl.Close()

	buf := make([]byte, 10)
	_, err = rl.Read(buf)
	assert.Error(t, err)
}

// TestRateLimiterWriter_Write_Closed 测试关闭后写入
func TestRateLimiterWriter_Write_Closed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	writer := &bytes.Buffer{}
	rl, err := NewRateLimiterWriter(writer, 1024, ctx)
	require.NoError(t, err)

	rl.Close()

	_, err = rl.Write([]byte("test"))
	assert.Error(t, err)
}

// TestRateLimiter_Read_Closed 测试关闭后读取
func TestRateLimiter_Read_Closed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rl, err := NewRateLimiter(1024, ctx)
	require.NoError(t, err)
	rl.SetReader(bytes.NewBufferString("test"))

	rl.Close()

	buf := make([]byte, 10)
	_, err = rl.Read(buf)
	assert.Error(t, err)
}

// TestRateLimiter_Write_Closed 测试关闭后写入
func TestRateLimiter_Write_Closed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rl, err := NewRateLimiter(1024, ctx)
	require.NoError(t, err)
	rl.SetWriter(&bytes.Buffer{})

	rl.Close()

	_, err = rl.Write([]byte("test"))
	assert.Error(t, err)
}

// TestRateLimiterReader_SetRate 测试设置速率
func TestRateLimiterReader_SetRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		newRate int64
		wantErr bool
	}{
		{
			name:    "valid new rate",
			newRate: 2048,
			wantErr: false,
		},
		{
			name:    "zero rate",
			newRate: 0,
			wantErr: true,
		},
		{
			name:    "negative rate",
			newRate: -100,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			reader := bytes.NewBufferString("test")
			rl, err := NewRateLimiterReader(reader, 1024, ctx)
			require.NoError(t, err)
			defer rl.Close()

			err = rl.SetRate(tc.newRate)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRateLimiterWriter_SetRate 测试设置速率
func TestRateLimiterWriter_SetRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		newRate int64
		wantErr bool
	}{
		{
			name:    "valid new rate",
			newRate: 2048,
			wantErr: false,
		},
		{
			name:    "zero rate",
			newRate: 0,
			wantErr: true,
		},
		{
			name:    "negative rate",
			newRate: -100,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			writer := &bytes.Buffer{}
			rl, err := NewRateLimiterWriter(writer, 1024, ctx)
			require.NoError(t, err)
			defer rl.Close()

			err = rl.SetRate(tc.newRate)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRateLimiter_SetReader 测试设置 Reader
func TestRateLimiter_SetReader(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rl, err := NewRateLimiter(1024, ctx)
	require.NoError(t, err)
	defer rl.Close()

	reader := bytes.NewBufferString("test")
	rl.SetReader(reader)

	// 读取验证
	buf := make([]byte, 10)
	n, err := rl.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
}

// TestRateLimiter_SetWriter 测试设置 Writer
func TestRateLimiter_SetWriter(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rl, err := NewRateLimiter(1024, ctx)
	require.NoError(t, err)
	defer rl.Close()

	writer := &bytes.Buffer{}
	rl.SetWriter(writer)

	// 写入验证
	n, err := rl.Write([]byte("test"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "test", writer.String())
}

// BenchmarkRateLimiterReader_Read 基准测试限速读取
func BenchmarkRateLimiterReader_Read(b *testing.B) {
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		rl, _ := NewRateLimiterReader(reader, 100*1024*1024, ctx)
		io.Copy(io.Discard, rl)
		rl.Close()
	}
}

// BenchmarkRateLimiterWriter_Write 基准测试限速写入
func BenchmarkRateLimiterWriter_Write(b *testing.B) {
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer := &bytes.Buffer{}
		rl, _ := NewRateLimiterWriter(writer, 100*1024*1024, ctx)
		rl.Write(data)
		rl.Close()
	}
}
