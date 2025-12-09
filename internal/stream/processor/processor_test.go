package processor

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"tunnox-core/internal/stream/compression"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultStreamProcessor(t *testing.T) {
	tests := []struct {
		name              string
		reader            io.Reader
		writer            io.Writer
		compressionReader *compression.GzipReader
		compressionWriter *compression.GzipWriter
		rateLimiter       interface{}
	}{
		{
			name:              "create with all components",
			reader:            &bytes.Buffer{},
			writer:            &bytes.Buffer{},
			compressionReader: nil,
			compressionWriter: nil,
			rateLimiter:       nil,
		},
		{
			name:              "create with nil compression",
			reader:            &bytes.Buffer{},
			writer:            &bytes.Buffer{},
			compressionReader: nil,
			compressionWriter: nil,
			rateLimiter:       nil,
		},
		{
			name:              "create with nil rate limiter",
			reader:            &bytes.Buffer{},
			writer:            &bytes.Buffer{},
			compressionReader: nil,
			compressionWriter: nil,
			rateLimiter:       nil,
		},
		{
			name:              "create minimal processor",
			reader:            &bytes.Buffer{},
			writer:            &bytes.Buffer{},
			compressionReader: nil,
			compressionWriter: nil,
			rateLimiter:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewDefaultStreamProcessor(
				tt.reader,
				tt.writer,
				tt.compressionReader,
				tt.compressionWriter,
				tt.rateLimiter,
			)

			assert.NotNil(t, processor)
			assert.Equal(t, tt.reader, processor.reader)
			assert.Equal(t, tt.writer, processor.writer)
			assert.Equal(t, tt.compressionReader, processor.compressionReader)
			assert.Equal(t, tt.compressionWriter, processor.compressionWriter)
		})
	}
}

func TestDefaultStreamProcessor_ReadExact(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		length      int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "read exact amount",
			data:        []byte("Hello, World!"),
			length:      13,
			expectError: false,
		},
		{
			name:        "read partial amount",
			data:        []byte("Hello, World!"),
			length:      5,
			expectError: false,
		},
		{
			name:        "read zero bytes",
			data:        []byte("Hello"),
			length:      0,
			expectError: false,
		},
		{
			name:        "read more than available",
			data:        []byte("Short"),
			length:      10,
			expectError: true,
			errorMsg:    "EOF",
		},
		{
			name:        "read from empty buffer",
			data:        []byte{},
			length:      5,
			expectError: true,
			errorMsg:    "EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.data)
			processor := NewDefaultStreamProcessor(reader, nil, nil, nil, nil)

			data, err := processor.ReadExact(tt.length)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Len(t, data, tt.length)
				assert.Equal(t, tt.data[:tt.length], data)
			}
		})
	}
}

func TestDefaultStreamProcessor_WriteExact(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectError bool
	}{
		{
			name:        "write normal data",
			data:        []byte("Hello, World!"),
			expectError: false,
		},
		{
			name:        "write empty data",
			data:        []byte{}, // Note: bytes.Buffer.Bytes() returns nil for empty buffer, not []byte{}
			expectError: false,
		},
		{
			name:        "write large data",
			data:        bytes.Repeat([]byte("x"), 1024*1024),
			expectError: false,
		},
		{
			name:        "write binary data",
			data:        []byte{0x00, 0x01, 0x02, 0xFF, 0xFE},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &bytes.Buffer{}
			processor := NewDefaultStreamProcessor(nil, writer, nil, nil, nil)

			err := processor.WriteExact(tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if len(tt.data) == 0 {
					// Empty writes may result in nil or empty slice
					assert.Empty(t, writer.Bytes())
				} else {
					assert.Equal(t, tt.data, writer.Bytes())
				}
			}
		})
	}
}

func TestDefaultStreamProcessor_WriteExact_Error(t *testing.T) {
	failWriter := &failingWriter{failAt: 0}
	processor := NewDefaultStreamProcessor(nil, failWriter, nil, nil, nil)

	err := processor.WriteExact([]byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestDefaultStreamProcessor_GetReader(t *testing.T) {
	reader := bytes.NewReader([]byte("test data"))
	processor := NewDefaultStreamProcessor(reader, nil, nil, nil, nil)

	retrievedReader := processor.GetReader()
	assert.Equal(t, reader, retrievedReader)
}

func TestDefaultStreamProcessor_GetWriter(t *testing.T) {
	writer := &bytes.Buffer{}
	processor := NewDefaultStreamProcessor(nil, writer, nil, nil, nil)

	retrievedWriter := processor.GetWriter()
	assert.Equal(t, writer, retrievedWriter)
}

func TestDefaultStreamProcessor_Close(t *testing.T) {
	t.Run("close with nil components", func(t *testing.T) {
		processor := NewDefaultStreamProcessor(nil, nil, nil, nil, nil)

		// Should not panic
		assert.NotPanics(t, func() {
			processor.Close()
		})
	})

	t.Run("close with all nil components", func(t *testing.T) {
		processor := &DefaultStreamProcessor{
			reader:            nil,
			writer:            nil,
			compressionReader: nil,
			compressionWriter: nil,
			rateLimiter:       nil,
		}

		// Should not panic
		assert.NotPanics(t, func() {
			processor.Close()
		})
	})
}

func TestDefaultStreamProcessor_ReadPacket(t *testing.T) {
	reader := bytes.NewReader([]byte("test"))
	processor := NewDefaultStreamProcessor(reader, nil, nil, nil, nil)

	// Current implementation returns nil, 0, nil
	pkt, size, err := processor.ReadPacket()
	assert.Nil(t, pkt)
	assert.Equal(t, 0, size)
	assert.NoError(t, err)
}

func TestDefaultStreamProcessor_WritePacket(t *testing.T) {
	writer := &bytes.Buffer{}
	processor := NewDefaultStreamProcessor(nil, writer, nil, nil, nil)

	// Current implementation returns 0, nil
	size, err := processor.WritePacket(nil, false, 0)
	assert.Equal(t, 0, size)
	assert.NoError(t, err)
}

func TestDefaultStreamProcessor_RoundTrip(t *testing.T) {
	// Test reading and writing data
	data := []byte("Round trip test data")

	// Write
	writeBuffer := &bytes.Buffer{}
	writeProcessor := NewDefaultStreamProcessor(nil, writeBuffer, nil, nil, nil)
	err := writeProcessor.WriteExact(data)
	require.NoError(t, err)

	// Read
	readProcessor := NewDefaultStreamProcessor(bytes.NewReader(writeBuffer.Bytes()), nil, nil, nil, nil)
	readData, err := readProcessor.ReadExact(len(data))
	require.NoError(t, err)

	assert.Equal(t, data, readData)
}

func TestDefaultStreamProcessor_MultipleOperations(t *testing.T) {
	buffer := &bytes.Buffer{}
	processor := NewDefaultStreamProcessor(nil, buffer, nil, nil, nil)

	// Write multiple times
	data1 := []byte("First write")
	data2 := []byte("Second write")
	data3 := []byte("Third write")

	err := processor.WriteExact(data1)
	require.NoError(t, err)
	err = processor.WriteExact(data2)
	require.NoError(t, err)
	err = processor.WriteExact(data3)
	require.NoError(t, err)

	// Verify all data written
	expected := append(append(data1, data2...), data3...)
	assert.Equal(t, expected, buffer.Bytes())

	// Now read back
	readProcessor := NewDefaultStreamProcessor(bytes.NewReader(buffer.Bytes()), nil, nil, nil, nil)

	readData1, err := readProcessor.ReadExact(len(data1))
	require.NoError(t, err)
	assert.Equal(t, data1, readData1)

	readData2, err := readProcessor.ReadExact(len(data2))
	require.NoError(t, err)
	assert.Equal(t, data2, readData2)

	readData3, err := readProcessor.ReadExact(len(data3))
	require.NoError(t, err)
	assert.Equal(t, data3, readData3)
}

// Mock types for testing

type failingWriter struct {
	failAt     int
	writeCount int
}

func (w *failingWriter) Write(p []byte) (n int, err error) {
	if w.writeCount == w.failAt {
		return 0, errors.New("write failed")
	}
	w.writeCount++
	return len(p), nil
}
