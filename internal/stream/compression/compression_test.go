package compression

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"testing"
	"time"
	io2 "tunnox-core/internal/stream"
)

func TestGzipReader(t *testing.T) {
	// 准备测试数据
	originalData := []byte("Hello, this is a test string for gzip compression!")

	// 压缩数据
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	_, err := gzipWriter.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write gzip data: %v", err)
	}
	gzipWriter.Close()

	compressedData := buf.Bytes()

	// 创建GzipReader
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := io2.NewGzipReader(bytes.NewReader(compressedData), ctx)
	defer reader.Close()

	// 读取解压缩后的数据
	var result bytes.Buffer
	_, err = io.Copy(&result, reader)
	if err != nil {
		t.Fatalf("Failed to read from GzipReader: %v", err)
	}

	// 验证结果
	if !bytes.Equal(result.Bytes(), originalData) {
		t.Errorf("Expected %s, got %s", string(originalData), string(result.Bytes()))
	}
}

func TestGzipReaderClose(t *testing.T) {
	// 测试资源释放
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := io2.NewGzipReader(bytes.NewReader([]byte{}), ctx)

	// 关闭reader
	reader.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !reader.IsClosed() {
		t.Error("Reader should be closed after calling Close()")
	}
}

func TestGzipReaderWithLargeData(t *testing.T) {
	// 测试大数据量的压缩和解压缩
	largeData := make([]byte, 1024*1024) // 1MB数据
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// 压缩数据
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	_, err := gzipWriter.Write(largeData)
	if err != nil {
		t.Fatalf("Failed to write gzip data: %v", err)
	}
	gzipWriter.Close()

	compressedData := buf.Bytes()

	// 创建GzipReader
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := io2.NewGzipReader(bytes.NewReader(compressedData), ctx)
	defer reader.Close()

	// 读取解压缩后的数据
	var result bytes.Buffer
	_, err = io.Copy(&result, reader)
	if err != nil {
		t.Fatalf("Failed to read from GzipReader: %v", err)
	}

	// 验证结果
	if !bytes.Equal(result.Bytes(), largeData) {
		t.Error("Large data decompression failed")
	}
}

func TestGzipReaderInvalidData(t *testing.T) {
	// 测试无效的gzip数据
	invalidData := []byte("This is not valid gzip data")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := io2.NewGzipReader(bytes.NewReader(invalidData), ctx)
	defer reader.Close()

	// 尝试读取应该会失败
	var result bytes.Buffer
	_, err := io.Copy(&result, reader)
	if err == nil {
		t.Error("Expected error when reading invalid gzip data")
	}
}

func TestGzipReaderMultipleReads(t *testing.T) {
	// 测试多次读取
	originalData := []byte("Multiple reads test data")

	// 压缩数据
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	_, err := gzipWriter.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write gzip data: %v", err)
	}
	gzipWriter.Close()

	compressedData := buf.Bytes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := io2.NewGzipReader(bytes.NewReader(compressedData), ctx)
	defer reader.Close()

	// 多次读取
	buffer := make([]byte, 10)
	var result bytes.Buffer

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			result.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read: %v", err)
		}
	}

	// 验证结果
	if !bytes.Equal(result.Bytes(), originalData) {
		t.Errorf("Expected %s, got %s", string(originalData), string(result.Bytes()))
	}
}

func TestGzipReaderAfterClose(t *testing.T) {
	// 测试关闭后读取
	originalData := []byte("Test data")

	// 压缩数据
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	_, err := gzipWriter.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write gzip data: %v", err)
	}
	gzipWriter.Close()

	compressedData := buf.Bytes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := io2.NewGzipReader(bytes.NewReader(compressedData), ctx)

	// 关闭reader
	reader.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 尝试读取应该返回EOF
	buffer := make([]byte, 10)
	n, err := reader.Read(buffer)
	if n != 0 || err != io.EOF {
		t.Errorf("Expected EOF after close, got n=%d, err=%v", n, err)
	}
}

func TestGzipWriter(t *testing.T) {
	// 准备测试数据
	originalData := []byte("Hello, this is a test string for gzip compression!")

	// 创建GzipWriter
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := io2.NewGzipWriter(&buf, ctx)

	// 写入数据
	_, err := writer.Write(originalData)
	if err != nil {
		t.Fatalf("Failed to write to GzipWriter: %v", err)
	}

	// 关闭writer
	writer.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 解压缩验证
	gzipReader, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()

	var result bytes.Buffer
	_, err = io.Copy(&result, gzipReader)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	// 验证结果
	if !bytes.Equal(result.Bytes(), originalData) {
		t.Errorf("Expected %s, got %s", string(originalData), string(result.Bytes()))
	}
}

func TestGzipWriterClose(t *testing.T) {
	// 测试资源释放
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := io2.NewGzipWriter(&buf, ctx)

	// 关闭writer
	writer.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !writer.IsClosed() {
		t.Error("Writer should be closed after calling Close()")
	}
}

func TestGzipWriterWithLargeData(t *testing.T) {
	// 测试大数据量的压缩
	largeData := make([]byte, 1024*1024) // 1MB数据
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := io2.NewGzipWriter(&buf, ctx)

	// 写入大数据
	_, err := writer.Write(largeData)
	if err != nil {
		t.Fatalf("Failed to write large data: %v", err)
	}

	// 关闭writer
	writer.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 解压缩验证
	gzipReader, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()

	var result bytes.Buffer
	_, err = io.Copy(&result, gzipReader)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	// 验证结果
	if !bytes.Equal(result.Bytes(), largeData) {
		t.Error("Large data compression/decompression failed")
	}
}

func TestGzipWriterMultipleWrites(t *testing.T) {
	// 测试多次写入
	testData1 := []byte("First part of data")
	testData2 := []byte("Second part of data")
	testData3 := []byte("Third part of data")

	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := io2.NewGzipWriter(&buf, ctx)

	// 多次写入
	_, err := writer.Write(testData1)
	if err != nil {
		t.Fatalf("Failed to write first part: %v", err)
	}

	_, err = writer.Write(testData2)
	if err != nil {
		t.Fatalf("Failed to write second part: %v", err)
	}

	_, err = writer.Write(testData3)
	if err != nil {
		t.Fatalf("Failed to write third part: %v", err)
	}

	// 关闭writer
	writer.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 解压缩验证
	gzipReader, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()

	var result bytes.Buffer
	_, err = io.Copy(&result, gzipReader)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	// 验证结果
	expectedData := append(testData1, append(testData2, testData3...)...)
	if !bytes.Equal(result.Bytes(), expectedData) {
		t.Errorf("Expected %s, got %s", string(expectedData), string(result.Bytes()))
	}
}

func TestGzipWriterAfterClose(t *testing.T) {
	// 测试关闭后写入
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := io2.NewGzipWriter(&buf, ctx)

	// 关闭writer
	writer.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 尝试写入应该返回错误
	_, err := writer.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error when writing after close")
	}
}

func TestGzipWriterContextCancellation(t *testing.T) {
	// 测试上下文取消时的行为
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())

	writer := io2.NewGzipWriter(&buf, ctx)

	// 取消上下文
	cancel()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !writer.IsClosed() {
		t.Error("Writer should be closed after context cancellation")
	}
}
