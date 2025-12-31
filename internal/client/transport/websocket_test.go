//go:build !no_websocket

package transport

import (
	"testing"
)

func TestNormalizeWebSocketURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "ws scheme preserved",
			input:    "ws://example.com/_tunnox",
			expected: "ws://example.com/_tunnox",
			hasError: false,
		},
		{
			name:     "wss scheme preserved",
			input:    "wss://example.com/_tunnox",
			expected: "wss://example.com/_tunnox",
			hasError: false,
		},
		{
			name:     "http to ws conversion",
			input:    "http://example.com/_tunnox",
			expected: "ws://example.com/_tunnox",
			hasError: false,
		},
		{
			name:     "https to wss conversion",
			input:    "https://example.com/_tunnox",
			expected: "wss://example.com/_tunnox",
			hasError: false,
		},
		{
			name:     "add default path to ws",
			input:    "ws://example.com",
			expected: "ws://example.com/_tunnox",
			hasError: false,
		},
		{
			name:     "add default path to wss",
			input:    "wss://example.com",
			expected: "wss://example.com/_tunnox",
			hasError: false,
		},
		{
			name:     "host:port without scheme",
			input:    "example.com:8080",
			expected: "ws://example.com:8080/_tunnox",
			hasError: false,
		},
		{
			name:     "host:port with path",
			input:    "example.com:8080/custom/path",
			expected: "ws://example.com:8080/custom/path",
			hasError: false,
		},
		{
			name:     "preserve query string",
			input:    "ws://example.com/_tunnox?token=abc",
			expected: "ws://example.com/_tunnox?token=abc",
			hasError: false,
		},
		{
			name:     "localhost address",
			input:    "127.0.0.1:8080",
			expected: "ws://127.0.0.1:8080/_tunnox",
			hasError: false,
		},
		{
			name:     "custom path preserved",
			input:    "ws://example.com/custom",
			expected: "ws://example.com/custom",
			hasError: false,
		},
		{
			name:     "empty path gets default",
			input:    "http://example.com",
			expected: "ws://example.com/_tunnox",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeWebSocketURL(tt.input)
			if tt.hasError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("NormalizeWebSocketURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestWsAddr(t *testing.T) {
	addr := &wsAddr{addr: "ws://example.com/_tunnox"}

	if addr.Network() != "websocket" {
		t.Errorf("Network() = %q, want %q", addr.Network(), "websocket")
	}

	if addr.String() != "ws://example.com/_tunnox" {
		t.Errorf("String() = %q, want %q", addr.String(), "ws://example.com/_tunnox")
	}
}

func TestWebSocketProtocolRegistration(t *testing.T) {
	// 验证 WebSocket 协议已注册
	info, ok := GetProtocol("websocket")
	if !ok {
		t.Fatal("WebSocket protocol should be registered")
	}

	if info.Name != "websocket" {
		t.Errorf("Protocol name = %q, want %q", info.Name, "websocket")
	}

	// WebSocket 优先级应该是 10（最高）
	if info.Priority != 10 {
		t.Errorf("Priority = %d, want %d", info.Priority, 10)
	}

	if info.Dialer == nil {
		t.Error("Dialer should not be nil")
	}
}

func TestDialWebSocket_InvalidURL(t *testing.T) {
	// 测试无效 URL
	tests := []struct {
		name    string
		address string
	}{
		{
			name:    "empty address",
			address: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 这些情况可能会产生不同的行为
			// 主要是验证不会 panic
			_, _ = NormalizeWebSocketURL(tt.address)
		})
	}
}
