package crossnode

import (
	"context"
	"net"
	"testing"
	"time"

	"tunnox-core/internal/core/dispose"
)

func TestNewConn(t *testing.T) {
	ctx := context.Background()

	// 创建一个监听器来获取 TCP 连接
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// 启动服务端
	serverDone := make(chan struct{})
	var serverConn net.Conn
	go func() {
		defer close(serverDone)
		serverConn, _ = listener.Accept()
	}()

	// 客户端连接
	clientConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer clientConn.Close()

	// 等待服务端接受连接
	<-serverDone
	defer serverConn.Close()

	// 转换为 TCPConn
	tcpConn := serverConn.(*net.TCPConn)

	// 创建 Conn
	conn := NewConn(ctx, "test-node", tcpConn, nil)
	if conn == nil {
		t.Fatal("NewConn returned nil")
	}
	defer conn.Close()

	// 验证字段
	if conn.NodeID() != "test-node" {
		t.Errorf("NodeID = %s, want test-node", conn.NodeID())
	}
	if conn.GetTCPConn() == nil {
		t.Error("GetTCPConn should not return nil")
	}
}

func TestConn_MarkBroken(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
	}

	// 初始状态
	if conn.IsBroken() {
		t.Error("Connection should not be broken initially")
	}

	// 标记为损坏
	conn.MarkBroken()

	if !conn.IsBroken() {
		t.Error("Connection should be broken after MarkBroken")
	}
}

func TestConn_MarkInUse(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
	}

	// 初始设置为空闲
	conn.MarkIdle()
	if conn.IsInUse() {
		t.Error("Connection should not be in use after MarkIdle")
	}

	// 标记为使用中
	conn.MarkInUse()
	if !conn.IsInUse() {
		t.Error("Connection should be in use after MarkInUse")
	}

	// 再次标记为空闲
	conn.MarkIdle()
	if conn.IsInUse() {
		t.Error("Connection should not be in use after MarkIdle")
	}
}

func TestConn_GetLastUsed(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
		lastUsed:    time.Now().Add(-1 * time.Hour),
	}

	oldTime := conn.GetLastUsed()

	// 更新 lastUsed
	conn.MarkInUse()

	newTime := conn.GetLastUsed()
	if !newTime.After(oldTime) {
		t.Error("GetLastUsed should return updated time after MarkInUse")
	}
}

func TestConn_SetDeadline_NilConn(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
		tcpConn:     nil,
	}

	err := conn.SetDeadline(time.Now().Add(time.Second))
	if err == nil {
		t.Error("SetDeadline should return error for nil tcpConn")
	}

	err = conn.SetReadDeadline(time.Now().Add(time.Second))
	if err == nil {
		t.Error("SetReadDeadline should return error for nil tcpConn")
	}

	err = conn.SetWriteDeadline(time.Now().Add(time.Second))
	if err == nil {
		t.Error("SetWriteDeadline should return error for nil tcpConn")
	}
}

func TestConn_LocalAddr_NilConn(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
		tcpConn:     nil,
	}

	if conn.LocalAddr() != nil {
		t.Error("LocalAddr should return nil for nil tcpConn")
	}

	if conn.RemoteAddr() != nil {
		t.Error("RemoteAddr should return nil for nil tcpConn")
	}
}

func TestConn_Read_NilConn(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
		tcpConn:     nil,
	}

	buf := make([]byte, 10)
	_, err := conn.Read(buf)
	if err == nil {
		t.Error("Read should return error for nil tcpConn")
	}
}

func TestConn_Write_NilConn(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
		tcpConn:     nil,
	}

	_, err := conn.Write([]byte("test"))
	if err == nil {
		t.Error("Write should return error for nil tcpConn")
	}
}

func TestConn_IsHealthy_Broken(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
		broken:      true,
	}

	if conn.IsHealthy() {
		t.Error("IsHealthy should return false for broken connection")
	}
}

func TestConn_IsHealthy_NilTcpConn(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
		tcpConn:     nil,
	}

	if conn.IsHealthy() {
		t.Error("IsHealthy should return false for nil tcpConn")
	}
}

func TestConn_IsHealthy_IdleTimeout(t *testing.T) {
	ctx := context.Background()
	conn := &Conn{
		ServiceBase: dispose.NewService("TestConn", ctx),
		nodeID:      "test-node",
		lastUsed:    time.Now().Add(-10 * time.Minute), // 超过 5 分钟空闲
	}

	if conn.IsHealthy() {
		t.Error("IsHealthy should return false for idle connection")
	}
}
