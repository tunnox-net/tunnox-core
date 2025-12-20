package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

func main() {
	mode := flag.String("mode", "client", "client or server")
	addr := flag.String("addr", "127.0.0.1:13600", "address")
	size := flag.Int("size", 100, "MB to transfer")
	flag.Parse()

	if *mode == "server" {
		runServer(*addr)
	} else {
		runClient(*addr, *size)
	}
}

func runServer(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("Listen error:", err)
		os.Exit(1)
	}
	fmt.Println("Echo server listening on", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			// 设置 TCP 参数
			if tc, ok := c.(*net.TCPConn); ok {
				tc.SetNoDelay(true)
				tc.SetReadBuffer(512 * 1024)
				tc.SetWriteBuffer(512 * 1024)
			}
			// 使用 io.CopyBuffer 进行高效拷贝
			buf := make([]byte, 512*1024)
			io.CopyBuffer(c, c, buf)
		}(conn)
	}
}

func runClient(addr string, sizeMB int) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("Dial error:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// 设置 TCP 参数
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
		tc.SetReadBuffer(512 * 1024)
		tc.SetWriteBuffer(512 * 1024)
	}

	totalBytes := int64(sizeMB) * 1024 * 1024
	chunkSize := 64 * 1024
	chunk := make([]byte, chunkSize)
	for i := range chunk {
		chunk[i] = byte(i % 256)
	}

	var sent, recv int64
	start := time.Now()

	// 使用单独的 goroutine 发送数据
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 512*1024)
		for recv < totalBytes {
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					fmt.Println("Read error:", err)
				}
				return
			}
			recv += int64(n)
		}
	}()

	// 发送数据
	for sent < totalBytes {
		n, err := conn.Write(chunk)
		if err != nil {
			fmt.Println("Write error:", err)
			break
		}
		sent += int64(n)
	}
	// 关闭写端
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.CloseWrite()
	}

	// 等待接收完成
	<-done
	elapsed := time.Since(start)

	fmt.Printf("Sent: %d MB, Received: %d MB\n", sent/1024/1024, recv/1024/1024)
	fmt.Printf("Duration: %.2f s\n", elapsed.Seconds())
	fmt.Printf("Throughput: %.1f MB/s\n", float64(sent)/elapsed.Seconds()/1024/1024)
}
