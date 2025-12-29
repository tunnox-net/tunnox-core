package mapping

import (
	"errors"
	"testing"
)

func TestContains(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "llo", true},
		{"hello world", "xyz", false},
		{"hello", "hello", true},
		{"", "", true},
		{"hello", "", true},
		{"", "hello", false},
		{"connection reset by peer", "connection reset", true},
		{"broken pipe", "broken pipe", true},
		{"timeout exceeded", "timeout", true},
		{"deadline exceeded", "deadline exceeded", true},
		{"use of closed network connection", "use of closed", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestDetermineCloseReason(t *testing.T) {
	// 创建一个临时的 BaseMappingHandler 用于测试
	h := &BaseMappingHandler{}

	tests := []struct {
		name     string
		sendErr  error
		recvErr  error
		expected string
	}{
		{
			name:     "both nil - normal close",
			sendErr:  nil,
			recvErr:  nil,
			expected: "normal",
		},
		{
			name:     "EOF - peer closed",
			sendErr:  errors.New("EOF"),
			recvErr:  nil,
			expected: "peer_closed",
		},
		{
			name:     "closed pipe - peer closed",
			sendErr:  nil,
			recvErr:  errors.New("io: read/write on closed pipe"),
			expected: "peer_closed",
		},
		{
			name:     "connection reset",
			sendErr:  errors.New("connection reset by peer"),
			recvErr:  nil,
			expected: "network_error",
		},
		{
			name:     "broken pipe",
			sendErr:  nil,
			recvErr:  errors.New("broken pipe"),
			expected: "network_error",
		},
		{
			name:     "timeout",
			sendErr:  errors.New("operation timeout"),
			recvErr:  nil,
			expected: "timeout",
		},
		{
			name:     "deadline exceeded",
			sendErr:  nil,
			recvErr:  errors.New("context deadline exceeded"),
			expected: "timeout",
		},
		{
			name:     "use of closed connection",
			sendErr:  errors.New("use of closed network connection"),
			recvErr:  nil,
			expected: "closed",
		},
		{
			name:     "unknown error",
			sendErr:  errors.New("some unknown error"),
			recvErr:  nil,
			expected: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.determineCloseReason(tt.sendErr, tt.recvErr)
			if got != tt.expected {
				t.Errorf("determineCloseReason() = %v, want %v", got, tt.expected)
			}
		})
	}
}
