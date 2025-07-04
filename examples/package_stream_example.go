package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"
	"tunnox-core/internal/stream"
)

func main() {
	// 示例1: 基本的读取和写入
	fmt.Println("=== 示例1: 基本的读取和写入 ===")
	basicExample()

	// 示例2: 大数据量的处理
	fmt.Println("\n=== 示例2: 大数据量的处理 ===")
	largeDataExample()

	// 示例3: 并发访问
	fmt.Println("\n=== 示例3: 并发访问 ===")
	concurrentExample()

	// 示例4: 上下文取消
	fmt.Println("\n=== 示例4: 上下文取消 ===")
	contextCancellationExample()
}

func basicExample() {
	// 准备测试数据
	testData := []byte("Hello, this is a test for PackageStream!")
	reader := bytes.NewReader(testData)
	var buf bytes.Buffer
	writer := &buf

	// 创建PackageStream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := stream.NewPackageStream(reader, writer, ctx)
	defer stream.Close()

	// 读取指定长度的数据
	readLength := 20
	data, err := stream.ReadExact(readLength)
	if err != nil {
		log.Printf("读取失败: %v", err)
		return
	}

	fmt.Printf("读取了 %d 字节: %s\n", len(data), string(data))

	// 写入数据
	writeData := []byte("写入的测试数据")
	err = stream.WriteExact(writeData)
	if err != nil {
		log.Printf("写入失败: %v", err)
		return
	}

	fmt.Printf("写入了 %d 字节: %s\n", len(writeData), string(writeData))
}

func largeDataExample() {
	// 生成大数据
	largeData := make([]byte, 100*1024) // 100KB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	reader := bytes.NewReader(largeData)
	var buf bytes.Buffer
	writer := &buf

	// 创建PackageStream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := stream.NewPackageStream(reader, writer, ctx)
	defer stream.Close()

	// 读取大数据
	start := time.Now()
	readLength := 50 * 1024 // 读取50KB
	data, err := stream.ReadExact(readLength)
	duration := time.Since(start)

	if err != nil {
		log.Printf("读取大数据失败: %v", err)
		return
	}

	fmt.Printf("读取了 %d 字节，耗时: %v\n", len(data), duration)

	// 写入大数据
	start = time.Now()
	err = stream.WriteExact(largeData)
	duration = time.Since(start)

	if err != nil {
		log.Printf("写入大数据失败: %v", err)
		return
	}

	fmt.Printf("写入了 %d 字节，耗时: %v\n", len(largeData), duration)
}

func concurrentExample() {
	testData := []byte("并发访问测试数据")
	reader := bytes.NewReader(testData)
	var buf bytes.Buffer
	writer := &buf

	// 创建PackageStream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := stream.NewPackageStream(reader, writer, ctx)
	defer stream.Close()

	// 并发读取
	go func() {
		data, err := stream.ReadExact(10)
		if err != nil {
			log.Printf("并发读取失败: %v", err)
			return
		}
		fmt.Printf("并发读取: %s\n", string(data))
	}()

	// 并发写入
	go func() {
		writeData := []byte("并发写入数据")
		err := stream.WriteExact(writeData)
		if err != nil {
			log.Printf("并发写入失败: %v", err)
			return
		}
		fmt.Printf("并发写入: %s\n", string(writeData))
	}()

	// 等待一段时间让goroutine执行
	time.Sleep(100 * time.Millisecond)
}

func contextCancellationExample() {
	testData := []byte("上下文取消测试数据")
	reader := bytes.NewReader(testData)
	var buf bytes.Buffer
	writer := &buf

	// 创建PackageStream
	ctx, cancel := context.WithCancel(context.Background())

	stream := stream.NewPackageStream(reader, writer, ctx)
	defer stream.Close()

	// 启动一个goroutine来取消上下文
	go func() {
		time.Sleep(50 * time.Millisecond)
		fmt.Println("取消上下文...")
		cancel()
	}()

	// 尝试读取（应该被上下文取消中断）
	data, err := stream.ReadExact(100)
	if err != nil {
		fmt.Printf("读取被上下文取消: %v\n", err)
	} else {
		fmt.Printf("读取成功: %s\n", string(data))
	}

	// 尝试写入（应该被上下文取消中断）
	writeData := []byte("测试写入")
	err = stream.WriteExact(writeData)
	if err != nil {
		fmt.Printf("写入被上下文取消: %v\n", err)
	} else {
		fmt.Printf("写入成功: %s\n", string(writeData))
	}
}
