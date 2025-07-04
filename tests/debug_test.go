package tests

import (
	"bytes"
	"fmt"
	"testing"
)

func TestBufferRead(t *testing.T) {
	// 测试 bytes.Buffer 的读取行为
	buf := bytes.NewBuffer([]byte("hello"))

	// 读取所有数据
	data := make([]byte, 10)
	n, err := buf.Read(data)
	fmt.Printf("Read %d bytes, error: %v\n", n, err)
	fmt.Printf("Data: %s\n", data[:n])

	// 再次读取，应该返回 EOF
	n2, err2 := buf.Read(data)
	fmt.Printf("Second read: %d bytes, error: %v\n", n2, err2)
}
