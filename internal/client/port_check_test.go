package client

import (
	"fmt"
	"net"
	"testing"

	coreerrors "tunnox-core/internal/core/errors"
)

func TestCheckPortAvailable_Available(t *testing.T) {
	// 使用一个随机可用端口
	err := CheckPortAvailable("127.0.0.1", 0)
	if err != nil {
		// 端口 0 表示让系统分配，所以这里期望成功
		// 但由于我们检查后立即关闭，实际上端口 0 的测试没意义
		// 改为测试一个大概率可用的端口
	}

	// 找一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port
	listener.Close()

	// 现在端口应该可用
	err = CheckPortAvailable("127.0.0.1", port)
	if err != nil {
		t.Errorf("Expected port %d to be available after closing listener, got error: %v", port, err)
	}
}

func TestCheckPortAvailable_InUse(t *testing.T) {
	// 创建一个监听器占用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	// 端口应该不可用
	err = CheckPortAvailable("127.0.0.1", port)
	if err == nil {
		t.Errorf("Expected error for port %d in use, got nil", port)
	}

	// 验证错误码
	if !coreerrors.IsCode(err, coreerrors.CodePortConflict) {
		t.Errorf("Expected CodePortConflict error, got: %v", err)
	}
}

func TestCheckPortAvailable_InvalidPort(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		wantError bool
	}{
		{"negative", -1, true},
		{"zero", 0, true},       // CheckPortAvailable 不允许端口 0
		{"too_large", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPortAvailable("127.0.0.1", tt.port)
			if tt.wantError && err == nil {
				t.Errorf("Expected error for invalid port %d, got nil", tt.port)
			}
			if !tt.wantError && err != nil {
				t.Errorf("Expected no error for port %d, got: %v", tt.port, err)
			}
		})
	}
}

func TestCanBindPort_Valid(t *testing.T) {
	// 找一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port
	listener.Close()

	// 测试 CanBindPort
	listenAddr := "127.0.0.1:" + itoa(port)
	err = CanBindPort(listenAddr)
	if err != nil {
		t.Errorf("Expected port to be available, got error: %v", err)
	}
}

func TestCanBindPort_InUse(t *testing.T) {
	// 创建一个监听器占用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	listenAddr := "127.0.0.1:" + itoa(addr.Port)

	// 测试 CanBindPort
	err = CanBindPort(listenAddr)
	if err == nil {
		t.Errorf("Expected error for port in use, got nil")
	}

	// 验证错误码
	if !coreerrors.IsCode(err, coreerrors.CodePortConflict) {
		t.Errorf("Expected CodePortConflict error, got: %v", err)
	}
}

func TestCanBindPort_Empty(t *testing.T) {
	err := CanBindPort("")
	if err == nil {
		t.Error("Expected error for empty listen address, got nil")
	}
}

func TestCanBindPort_AutoAssign(t *testing.T) {
	// 端口 0 表示自动分配，CanBindPort 应该跳过检查并返回成功
	err := CanBindPort("127.0.0.1:0")
	if err != nil {
		t.Errorf("Expected port 0 (auto-assign) to succeed, got error: %v", err)
	}

	// 同样测试 0.0.0.0:0
	err = CanBindPort("0.0.0.0:0")
	if err != nil {
		t.Errorf("Expected 0.0.0.0:0 (auto-assign) to succeed, got error: %v", err)
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
