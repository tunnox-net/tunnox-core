package main

import (
	"bytes"
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/io"
	"tunnox-core/internal/utils"
)

func improvedExample() {
	fmt.Println("=== 改进后的功能示例 ===")

	// 示例1: 使用工厂模式
	fmt.Println("\n--- 示例1: 工厂模式 ---")
	factoryExample()

	// 示例2: 错误处理
	fmt.Println("\n--- 示例2: 错误处理 ---")
	errorHandlingExample()

	// 示例3: 资源管理
	fmt.Println("\n--- 示例3: 资源管理 ---")
	resourceManagementExample()
}

func factoryExample() {
	// 创建工厂
	factory := io.NewDefaultStreamFactory()

	// 准备测试数据
	testData := []byte("Hello, Factory Pattern!")
	reader := bytes.NewReader(testData)
	var buf bytes.Buffer
	writer := &buf

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 使用工厂创建各种流
	stream := factory.NewPackageStream(reader, writer, ctx)
	defer stream.Close()

	// 创建限速读取器
	rateReader, _ := factory.NewRateLimiterReader(reader, 1024, ctx)
	defer rateReader.Close()

	// 创建压缩写入器
	compWriter := factory.NewCompressionWriter(writer, ctx)
	defer compWriter.Close()

	fmt.Println("✓ 工厂模式创建成功")
}

func errorHandlingExample() {
	// 测试不同类型的错误
	testCases := []struct {
		name string
		test func() error
	}{
		{
			name: "无效速率限制",
			test: func() error {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("✓ 捕获到panic: %v\n", r)
					}
				}()

				// 这会触发panic
				io.NewRateLimiterReader(nil, 0, context.Background())
				return nil
			},
		},
		{
			name: "流关闭错误",
			test: func() error {
				stream := io.NewPackageStream(nil, nil, context.Background())
				stream.Close()

				_, err := stream.ReadExact(10)
				if err != nil {
					fmt.Printf("✓ 流关闭错误: %v\n", err)
					return err
				}
				return nil
			},
		},
		{
			name: "错误类型判断",
			test: func() error {
				err := utils.ErrStreamClosed

				if utils.IsFatalError(err) {
					fmt.Printf("✓ 致命错误判断正确: %v\n", err)
				}

				if !utils.IsTemporaryError(err) {
					fmt.Printf("✓ 临时错误判断正确: %v\n", err)
				}

				return err
			},
		},
	}

	for _, tc := range testCases {
		fmt.Printf("测试: %s\n", tc.name)
		tc.test()
	}
}

func resourceManagementExample() {
	// 测试资源管理
	fmt.Println("测试资源管理...")

	// 创建压缩读取器
	reader := bytes.NewReader([]byte("test data"))
	ctx, cancel := context.WithCancel(context.Background())

	gzipReader := io.NewGzipReader(reader, ctx)

	// 测试并发访问
	go func() {
		buffer := make([]byte, 10)
		_, err := gzipReader.Read(buffer)
		if err != nil {
			fmt.Printf("✓ 并发读取错误处理: %v\n", err)
		}
	}()

	// 等待一下
	time.Sleep(10 * time.Millisecond)

	// 关闭资源
	gzipReader.Close()
	cancel()

	fmt.Println("✓ 资源管理测试完成")
}

// 演示如何使用新的错误类型
func demonstrateErrorTypes() {
	// 创建数据包错误
	packetErr := utils.NewPacketError("test", "packet error", nil)
	fmt.Printf("数据包错误: %v\n", packetErr)

	// 创建流错误
	streamErr := utils.NewStreamError("read", "stream error", nil)
	fmt.Printf("流错误: %v\n", streamErr)

	// 创建限速错误
	rateErr := utils.NewRateLimitError(1024, "rate limit error", nil)
	fmt.Printf("限速错误: %v\n", rateErr)

	// 创建压缩错误
	compErr := utils.NewCompressionError("compress", "compression error", nil)
	fmt.Printf("压缩错误: %v\n", compErr)
}
