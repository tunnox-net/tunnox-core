package processor

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockReader 模拟 Reader
type mockReader struct {
	data   []byte
	offset int
}

func newMockReader(data []byte) *mockReader {
	return &mockReader{data: data}
}

func (r *mockReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

// mockWriter 模拟 Writer
type mockWriter struct {
	data bytes.Buffer
}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	return w.data.Write(p)
}

func (w *mockWriter) Bytes() []byte {
	return w.data.Bytes()
}

// TestDefaultStreamProcessor_Creation 测试创建默认流处理器
func TestDefaultStreamProcessor_Creation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		reader        io.Reader
		writer        io.Writer
		wantNonNilSP  bool
	}{
		{
			name:         "with both reader and writer",
			reader:       &bytes.Buffer{},
			writer:       &bytes.Buffer{},
			wantNonNilSP: true,
		},
		{
			name:         "with nil reader",
			reader:       nil,
			writer:       &bytes.Buffer{},
			wantNonNilSP: true,
		},
		{
			name:         "with nil writer",
			reader:       &bytes.Buffer{},
			writer:       nil,
			wantNonNilSP: true,
		},
		{
			name:         "with both nil",
			reader:       nil,
			writer:       nil,
			wantNonNilSP: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sp := NewDefaultStreamProcessor(tc.reader, tc.writer, nil, nil, nil)
			if tc.wantNonNilSP {
				require.NotNil(t, sp)
			}
		})
	}
}

// TestDefaultStreamProcessor_GetReader 测试获取 Reader
func TestDefaultStreamProcessor_GetReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		reader     io.Reader
		wantReader io.Reader
	}{
		{
			name:       "with valid reader",
			reader:     &bytes.Buffer{},
			wantReader: &bytes.Buffer{},
		},
		{
			name:       "with nil reader",
			reader:     nil,
			wantReader: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sp := NewDefaultStreamProcessor(tc.reader, &bytes.Buffer{}, nil, nil, nil)
			gotReader := sp.GetReader()

			if tc.reader == nil {
				assert.Nil(t, gotReader)
			} else {
				assert.NotNil(t, gotReader)
			}
		})
	}
}

// TestDefaultStreamProcessor_GetWriter 测试获取 Writer
func TestDefaultStreamProcessor_GetWriter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		writer     io.Writer
		wantWriter io.Writer
	}{
		{
			name:       "with valid writer",
			writer:     &bytes.Buffer{},
			wantWriter: &bytes.Buffer{},
		},
		{
			name:       "with nil writer",
			writer:     nil,
			wantWriter: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sp := NewDefaultStreamProcessor(&bytes.Buffer{}, tc.writer, nil, nil, nil)
			gotWriter := sp.GetWriter()

			if tc.writer == nil {
				assert.Nil(t, gotWriter)
			} else {
				assert.NotNil(t, gotWriter)
			}
		})
	}
}

// TestDefaultStreamProcessor_ReadExact 测试精确读取
func TestDefaultStreamProcessor_ReadExact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputData []byte
		readLen   int
		wantData  []byte
		wantErr   bool
	}{
		{
			name:      "read exact bytes",
			inputData: []byte("hello world"),
			readLen:   5,
			wantData:  []byte("hello"),
			wantErr:   false,
		},
		{
			name:      "read all bytes",
			inputData: []byte("test"),
			readLen:   4,
			wantData:  []byte("test"),
			wantErr:   false,
		},
		{
			name:      "read more than available",
			inputData: []byte("short"),
			readLen:   10,
			wantData:  nil,
			wantErr:   true,
		},
		{
			name:      "read zero bytes",
			inputData: []byte("data"),
			readLen:   0,
			wantData:  []byte{},
			wantErr:   false,
		},
		{
			name:      "read from empty source",
			inputData: []byte{},
			readLen:   5,
			wantData:  nil,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reader := newMockReader(tc.inputData)
			sp := NewDefaultStreamProcessor(reader, &bytes.Buffer{}, nil, nil, nil)

			gotData, err := sp.ReadExact(tc.readLen)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantData, gotData)
			}
		})
	}
}

// TestDefaultStreamProcessor_WriteExact 测试精确写入
func TestDefaultStreamProcessor_WriteExact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputData []byte
		wantData  []byte
		wantErr   bool
	}{
		{
			name:      "write bytes",
			inputData: []byte("hello world"),
			wantData:  []byte("hello world"),
			wantErr:   false,
		},
		{
			name:      "write empty bytes",
			inputData: []byte{},
			wantData:  nil, // 空写入后 Buffer.Bytes() 返回 nil
			wantErr:   false,
		},
		{
			name:      "write large bytes",
			inputData: bytes.Repeat([]byte("a"), 10000),
			wantData:  bytes.Repeat([]byte("a"), 10000),
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			writer := &mockWriter{}
			sp := NewDefaultStreamProcessor(&bytes.Buffer{}, writer, nil, nil, nil)

			err := sp.WriteExact(tc.inputData)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantData, writer.Bytes())
			}
		})
	}
}

// TestDefaultStreamProcessor_Close 测试关闭
func TestDefaultStreamProcessor_Close(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		setup func() *DefaultStreamProcessor
	}{
		{
			name: "close with basic setup",
			setup: func() *DefaultStreamProcessor {
				return NewDefaultStreamProcessor(&bytes.Buffer{}, &bytes.Buffer{}, nil, nil, nil)
			},
		},
		{
			name: "close with nil components",
			setup: func() *DefaultStreamProcessor {
				return NewDefaultStreamProcessor(nil, nil, nil, nil, nil)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sp := tc.setup()
			// Close should not panic
			assert.NotPanics(t, func() {
				sp.Close()
			})
		})
	}
}

// TestDefaultStreamProcessor_ReadPacket 测试读取数据包
func TestDefaultStreamProcessor_ReadPacket(t *testing.T) {
	t.Parallel()

	sp := NewDefaultStreamProcessor(&bytes.Buffer{}, &bytes.Buffer{}, nil, nil, nil)
	pkt, n, err := sp.ReadPacket()

	// 当前实现返回 nil, 0, nil
	assert.Nil(t, pkt)
	assert.Equal(t, 0, n)
	assert.NoError(t, err)
}

// TestDefaultStreamProcessor_WritePacket 测试写入数据包
func TestDefaultStreamProcessor_WritePacket(t *testing.T) {
	t.Parallel()

	sp := NewDefaultStreamProcessor(&bytes.Buffer{}, &bytes.Buffer{}, nil, nil, nil)
	n, err := sp.WritePacket(nil, false, 0)

	// 当前实现返回 0, nil
	assert.Equal(t, 0, n)
	assert.NoError(t, err)
}

// TestStreamProcessor_Interface 测试接口实现
func TestStreamProcessor_Interface(t *testing.T) {
	t.Parallel()

	var _ StreamProcessor = (*DefaultStreamProcessor)(nil)
}
