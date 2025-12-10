package reliable

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReassembler_WriteRead(t *testing.T) {
	r := NewReassembler()
	defer r.Close()

	// 写入分片数据
	data1 := []byte("Hello, ")
	data2 := []byte("World!")

	go func() {
		err := r.Write(data1)
		require.NoError(t, err)
		err = r.Write(data2)
		require.NoError(t, err)
	}()

	// 读取完整数据
	buf := make([]byte, 13)
	n, err := io.ReadFull(r, buf)

	assert.NoError(t, err)
	assert.Equal(t, 13, n)
	assert.Equal(t, "Hello, World!", string(buf))
}

func TestReassembler_LargeData(t *testing.T) {
	r := NewReassembler()
	defer r.Close()

	// 写入大量分片数据
	totalSize := 1024 * 1024 // 1MB
	chunkSize := 1024        // 1KB per chunk
	chunks := totalSize / chunkSize

	go func() {
		for i := 0; i < chunks; i++ {
			data := make([]byte, chunkSize)
			for j := range data {
				data[j] = byte(i % 256)
			}
			err := r.Write(data)
			require.NoError(t, err)
		}
	}()

	// 读取所有数据
	received := make([]byte, totalSize)
	n, err := io.ReadFull(r, received)

	assert.NoError(t, err)
	assert.Equal(t, totalSize, n)

	// 验证数据正确性
	for i := 0; i < chunks; i++ {
		for j := 0; j < chunkSize; j++ {
			expected := byte(i % 256)
			actual := received[i*chunkSize+j]
			assert.Equal(t, expected, actual, "mismatch at chunk %d, byte %d", i, j)
		}
	}
}

func TestReassembler_Close(t *testing.T) {
	r := NewReassembler()

	// 在goroutine中写入数据，避免阻塞
	go func() {
		err := r.Write([]byte("test"))
		assert.NoError(t, err)
		// 写入后关闭写端
		r.Close()
	}()

	// 读取应该返回之前的数据
	buf := make([]byte, 4)
	n, err := r.Read(buf)
	assert.Equal(t, 4, n)
	assert.NoError(t, err)
	assert.Equal(t, "test", string(buf))

	// 再次读取应该返回 EOF（pipe已关闭）
	n, err = r.Read(buf)
	assert.Equal(t, 0, n)
	assert.Error(t, err) // 可能是 EOF 或 "read/write on closed pipe"
	
	// 再次写入应该失败
	err = r.Write([]byte("test"))
	assert.Error(t, err)
}

func TestReassembler_ConcurrentWrites(t *testing.T) {
	r := NewReassembler()
	defer r.Close()

	// 并发写入
	numWriters := 10
	writesPerWriter := 100
	dataSize := 100

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < numWriters; i++ {
			go func(writerID int) {
				for j := 0; j < writesPerWriter; j++ {
					data := make([]byte, dataSize)
					for k := range data {
						data[k] = byte(writerID)
					}
					err := r.Write(data)
					require.NoError(t, err)
				}
			}(i)
		}
	}()

	// 读取所有数据
	totalSize := numWriters * writesPerWriter * dataSize
	received := make([]byte, totalSize)
	n, err := io.ReadFull(r, received)

	assert.NoError(t, err)
	assert.Equal(t, totalSize, n)

	<-done
}
