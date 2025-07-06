package tests

import (
	"bytes"
	"context"
	"testing"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

func TestBufferRead(t *testing.T) {
	// 测试 bytes.Buffer 的读取行为
	buf := bytes.NewBuffer([]byte("hello"))

	// 读取所有数据
	data := make([]byte, 10)
	n, err := buf.Read(data)
	utils.Infof("Read %d bytes, error: %v", n, err)
	utils.Infof("Data: %s", data[:n])

	// 再次读取，应该返回 EOF
	n2, err2 := buf.Read(data)
	utils.Infof("Second read: %d bytes, error: %v", n2, err2)
}

func TestDebugPackageStream(t *testing.T) {
	// 准备测试数据
	testData := []byte("Hello, this is debug test data!")
	reader := bytes.NewReader(testData)
	var buf bytes.Buffer
	writer := &buf

	// 创建PackageStream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := stream.NewStreamProcessor(reader, writer, ctx)
	defer stream.Close()

	// 读取数据
	n, err := stream.GetReader().Read(make([]byte, 10))
	utils.Infof("Read %d bytes, error: %v", n, err)
	utils.Infof("Data: %s", testData[:n])

	// 再次读取
	n2, err2 := stream.GetReader().Read(make([]byte, 10))
	utils.Infof("Second read: %d bytes, error: %v", n2, err2)
}
