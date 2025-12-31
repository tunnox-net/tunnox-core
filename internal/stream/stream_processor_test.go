package stream

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewStreamProcessor 测试创建流处理器
func TestNewStreamProcessor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reader io.Reader
		writer io.Writer
	}{
		{
			name:   "with both reader and writer",
			reader: &bytes.Buffer{},
			writer: &bytes.Buffer{},
		},
		{
			name:   "with nil reader",
			reader: nil,
			writer: &bytes.Buffer{},
		},
		{
			name:   "with nil writer",
			reader: &bytes.Buffer{},
			writer: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			sp := NewStreamProcessor(tc.reader, tc.writer, ctx)
			require.NotNil(t, sp)

			// 验证 reader 和 writer 正确设置
			assert.Equal(t, tc.reader, sp.GetReader())
			assert.Equal(t, tc.writer, sp.GetWriter())

			sp.Close()
		})
	}
}

// TestStreamProcessor_GetReader 测试获取 Reader
func TestStreamProcessor_GetReader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reader := bytes.NewBufferString("test data")
	writer := &bytes.Buffer{}

	sp := NewStreamProcessor(reader, writer, ctx)
	defer sp.Close()

	gotReader := sp.GetReader()
	assert.Equal(t, reader, gotReader)
}

// TestStreamProcessor_GetWriter 测试获取 Writer
func TestStreamProcessor_GetWriter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reader := &bytes.Buffer{}
	writer := &bytes.Buffer{}

	sp := NewStreamProcessor(reader, writer, ctx)
	defer sp.Close()

	gotWriter := sp.GetWriter()
	assert.Equal(t, writer, gotWriter)
}

// TestStreamProcessor_ReadExact 测试精确读取
func TestStreamProcessor_ReadExact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputData string
		readLen   int
		wantData  string
		wantErr   bool
	}{
		{
			name:      "read exact bytes",
			inputData: "hello world",
			readLen:   5,
			wantData:  "hello",
			wantErr:   false,
		},
		{
			name:      "read all bytes",
			inputData: "test",
			readLen:   4,
			wantData:  "test",
			wantErr:   false,
		},
		{
			name:      "read more than available",
			inputData: "short",
			readLen:   10,
			wantData:  "",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			reader := bytes.NewBufferString(tc.inputData)
			sp := NewStreamProcessor(reader, &bytes.Buffer{}, ctx)
			defer sp.Close()

			data, err := sp.ReadExact(tc.readLen)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantData, string(data))
			}
		})
	}
}

// TestStreamProcessor_ReadAvailable 测试读取可用数据
func TestStreamProcessor_ReadAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputData string
		maxLen    int
		wantErr   bool
	}{
		{
			name:      "read with default max",
			inputData: "hello",
			maxLen:    0, // 使用默认值
			wantErr:   false,
		},
		{
			name:      "read with custom max",
			inputData: "hello world",
			maxLen:    5,
			wantErr:   false,
		},
		{
			name:      "read empty",
			inputData: "",
			maxLen:    10,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			reader := bytes.NewBufferString(tc.inputData)
			sp := NewStreamProcessor(reader, &bytes.Buffer{}, ctx)
			defer sp.Close()

			data, err := sp.ReadAvailable(tc.maxLen)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, data)
			}
		})
	}
}

// TestStreamProcessor_WriteExact 测试精确写入
func TestStreamProcessor_WriteExact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputData string
		wantErr   bool
	}{
		{
			name:      "write bytes",
			inputData: "hello world",
			wantErr:   false,
		},
		{
			name:      "write empty bytes",
			inputData: "",
			wantErr:   false,
		},
		{
			name:      "write large bytes",
			inputData: string(bytes.Repeat([]byte("a"), 10000)),
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			writer := &bytes.Buffer{}
			sp := NewStreamProcessor(&bytes.Buffer{}, writer, ctx)
			defer sp.Close()

			err := sp.WriteExact([]byte(tc.inputData))

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.inputData, writer.String())
			}
		})
	}
}

// TestStreamProcessor_WritePacket_Heartbeat 测试写入心跳包
func TestStreamProcessor_WritePacket_Heartbeat(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	writer := &bytes.Buffer{}
	sp := NewStreamProcessor(&bytes.Buffer{}, writer, ctx)
	defer sp.Close()

	pkt := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
	}

	n, err := sp.WritePacket(pkt, false, 0)
	require.NoError(t, err)
	assert.Greater(t, n, 0)
}

// TestStreamProcessor_WritePacket_Nil 测试写入空包
func TestStreamProcessor_WritePacket_Nil(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	writer := &bytes.Buffer{}
	sp := NewStreamProcessor(&bytes.Buffer{}, writer, ctx)
	defer sp.Close()

	_, err := sp.WritePacket(nil, false, 0)
	assert.Error(t, err)
}

// TestStreamProcessor_WritePacket_JsonCommand 测试写入 JsonCommand 包
func TestStreamProcessor_WritePacket_JsonCommand(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	writer := &bytes.Buffer{}
	sp := NewStreamProcessor(&bytes.Buffer{}, writer, ctx)
	defer sp.Close()

	cmdPkt := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		Token:       "test-token",
		SenderId:    "sender-001",
		ReceiverId:  "receiver-001",
		CommandBody: "test body",
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	n, err := sp.WritePacket(pkt, false, 0)
	require.NoError(t, err)
	assert.Greater(t, n, 0)
}

// TestStreamProcessor_WritePacket_WithCompression 测试带压缩写入
func TestStreamProcessor_WritePacket_WithCompression(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	writer := &bytes.Buffer{}
	sp := NewStreamProcessor(&bytes.Buffer{}, writer, ctx)
	defer sp.Close()

	cmdPkt := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		Token:       "test-token",
		SenderId:    "sender-001",
		ReceiverId:  "receiver-001",
		CommandBody: string(bytes.Repeat([]byte("test body "), 100)), // 大量重复数据更容易压缩
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	n, err := sp.WritePacket(pkt, true, 0)
	require.NoError(t, err)
	assert.Greater(t, n, 0)
}

// TestStreamProcessor_ReadWritePacket_RoundTrip 测试读写往返
func TestStreamProcessor_ReadWritePacket_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	buf := &bytes.Buffer{}
	sp := NewStreamProcessor(buf, buf, ctx)
	defer sp.Close()

	// 创建测试包
	cmdPkt := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		Token:       "round-trip-token",
		SenderId:    "sender-001",
		ReceiverId:  "receiver-001",
		CommandBody: "round trip test",
	}

	originalPkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	// 写入包
	_, err := sp.WritePacket(originalPkt, false, 0)
	require.NoError(t, err)

	// 读取包
	readPkt, _, err := sp.ReadPacket()
	require.NoError(t, err)
	require.NotNil(t, readPkt)
	require.NotNil(t, readPkt.CommandPacket)

	// 验证内容
	assert.Equal(t, originalPkt.CommandPacket.CommandType, readPkt.CommandPacket.CommandType)
	assert.Equal(t, originalPkt.CommandPacket.Token, readPkt.CommandPacket.Token)
	assert.Equal(t, originalPkt.CommandPacket.SenderId, readPkt.CommandPacket.SenderId)
	assert.Equal(t, originalPkt.CommandPacket.ReceiverId, readPkt.CommandPacket.ReceiverId)
}

// TestStreamProcessor_Close 测试关闭
func TestStreamProcessor_Close(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sp := NewStreamProcessor(&bytes.Buffer{}, &bytes.Buffer{}, ctx)

	// Close 不应该 panic
	assert.NotPanics(t, func() {
		sp.Close()
	})

	// 重复关闭也不应该 panic
	assert.NotPanics(t, func() {
		sp.Close()
	})
}

// TestStreamProcessor_CloseWithResult 测试关闭并返回结果
func TestStreamProcessor_CloseWithResult(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sp := NewStreamProcessor(&bytes.Buffer{}, &bytes.Buffer{}, ctx)

	result := sp.CloseWithResult()
	assert.NotNil(t, result)
}

// TestStreamProcessor_ContextCancellation 测试 context 取消
func TestStreamProcessor_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	sp := NewStreamProcessor(&bytes.Buffer{}, &bytes.Buffer{}, ctx)

	// 取消 context
	cancel()

	// 等待一点时间让取消生效
	time.Sleep(50 * time.Millisecond)

	// 尝试读取应该失败
	_, err := sp.ReadExact(10)
	assert.Error(t, err)
}

// TestStreamProcessor_AcquireReadLock_Closed 测试关闭后获取读锁
func TestStreamProcessor_AcquireReadLock_Closed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sp := NewStreamProcessor(&bytes.Buffer{}, &bytes.Buffer{}, ctx)
	sp.Close()

	// 关闭后读取应该返回 EOF
	_, err := sp.ReadExact(10)
	assert.Error(t, err)
}

// TestStreamProcessor_AcquireWriteLock_Closed 测试关闭后获取写锁
func TestStreamProcessor_AcquireWriteLock_Closed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sp := NewStreamProcessor(&bytes.Buffer{}, &bytes.Buffer{}, ctx)
	sp.Close()

	// 关闭后写入应该返回错误
	err := sp.WriteExact([]byte("test"))
	assert.Error(t, err)
}

// TestStreamProcessor_NilReader 测试 nil reader
func TestStreamProcessor_NilReader(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sp := NewStreamProcessor(nil, &bytes.Buffer{}, ctx)
	defer sp.Close()

	// 使用 nil reader 应该返回错误
	_, err := sp.ReadExact(10)
	assert.Error(t, err)
}

// TestStreamProcessor_NilWriter 测试 nil writer
func TestStreamProcessor_NilWriter(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sp := NewStreamProcessor(&bytes.Buffer{}, nil, ctx)
	defer sp.Close()

	// 使用 nil writer 应该返回错误
	err := sp.WriteExact([]byte("test"))
	assert.Error(t, err)
}

// BenchmarkStreamProcessor_WriteExact 基准测试写入
func BenchmarkStreamProcessor_WriteExact(b *testing.B) {
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer := &bytes.Buffer{}
		sp := NewStreamProcessor(&bytes.Buffer{}, writer, ctx)
		sp.WriteExact(data)
		sp.Close()
	}
}

// BenchmarkStreamProcessor_ReadExact 基准测试读取
func BenchmarkStreamProcessor_ReadExact(b *testing.B) {
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		sp := NewStreamProcessor(reader, &bytes.Buffer{}, ctx)
		sp.ReadExact(1024)
		sp.Close()
	}
}
