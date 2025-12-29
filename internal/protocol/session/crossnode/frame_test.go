package crossnode

import (
	"bytes"
	"testing"
)

func TestTunnelIDFromString(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"short string", "test"},
		{"16 char string", "1234567890123456"},
		{"long string (truncated)", "12345678901234567890"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := TunnelIDFromString(tt.input)
			if err != nil {
				t.Errorf("TunnelIDFromString(%q) returned error: %v", tt.input, err)
			}

			// 验证结果长度
			if len(id) != 16 {
				t.Errorf("TunnelIDFromString result should be 16 bytes, got %d", len(id))
			}
		})
	}
}

func TestTunnelIDToString(t *testing.T) {
	tests := []struct {
		name     string
		input    [16]byte
		expected string
	}{
		{
			name:     "normal string",
			input:    [16]byte{'h', 'e', 'l', 'l', 'o'},
			expected: "hello",
		},
		{
			name:     "full 16 bytes",
			input:    [16]byte{'1', '2', '3', '4', '5', '6', '7', '8', '9', '0', 'a', 'b', 'c', 'd', 'e', 'f'},
			expected: "1234567890abcdef",
		},
		{
			name:     "empty",
			input:    [16]byte{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TunnelIDToString(tt.input)
			if result != tt.expected {
				t.Errorf("TunnelIDToString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTunnelIDRoundTrip(t *testing.T) {
	original := "my-tunnel-id"
	id, err := TunnelIDFromString(original)
	if err != nil {
		t.Fatalf("TunnelIDFromString failed: %v", err)
	}

	result := TunnelIDToString(id)
	if result != original {
		t.Errorf("Round trip failed: got %q, want %q", result, original)
	}
}

func TestEncodeDecodeTargetReadyMessage(t *testing.T) {
	tunnelID := "tunnel-123"
	targetNodeID := "node-456"

	// 编码
	encoded := EncodeTargetReadyMessage(tunnelID, targetNodeID)

	// 解码
	decodedTunnelID, decodedNodeID, err := DecodeTargetReadyMessage(encoded)
	if err != nil {
		t.Fatalf("DecodeTargetReadyMessage failed: %v", err)
	}

	if decodedTunnelID != tunnelID {
		t.Errorf("TunnelID = %q, want %q", decodedTunnelID, tunnelID)
	}
	if decodedNodeID != targetNodeID {
		t.Errorf("TargetNodeID = %q, want %q", decodedNodeID, targetNodeID)
	}
}

func TestDecodeTargetReadyMessage_Invalid(t *testing.T) {
	// 无效格式（没有分隔符）
	_, _, err := DecodeTargetReadyMessage([]byte("no-separator"))
	if err == nil {
		t.Error("DecodeTargetReadyMessage should return error for invalid format")
	}
}

func TestWriteFrameToWriter_NilWriter(t *testing.T) {
	var tunnelID [16]byte
	copy(tunnelID[:], "test-tunnel")

	err := WriteFrameToWriter(nil, tunnelID, FrameTypeData, []byte("test"))
	if err == nil {
		t.Error("WriteFrameToWriter should return error for nil writer")
	}
}

func TestWriteFrameToWriter_FrameTooLarge(t *testing.T) {
	var tunnelID [16]byte
	copy(tunnelID[:], "test-tunnel")

	// 创建一个超大的数据
	largeData := make([]byte, MaxFrameSize+1)

	var buf bytes.Buffer
	err := WriteFrameToWriter(&buf, tunnelID, FrameTypeData, largeData)
	if err == nil {
		t.Error("WriteFrameToWriter should return error for oversized frame")
	}
}

func TestWriteReadFrame(t *testing.T) {
	var tunnelID [16]byte
	copy(tunnelID[:], "test-tunnel-id")
	testData := []byte("hello world")

	// 写入帧
	var buf bytes.Buffer
	err := WriteFrameToWriter(&buf, tunnelID, FrameTypeData, testData)
	if err != nil {
		t.Fatalf("WriteFrameToWriter failed: %v", err)
	}

	// 读取帧
	readTunnelID, frameType, data, err := ReadFrameFromReader(&buf)
	if err != nil {
		t.Fatalf("ReadFrameFromReader failed: %v", err)
	}

	// 验证
	if readTunnelID != tunnelID {
		t.Errorf("TunnelID mismatch")
	}
	if frameType != FrameTypeData {
		t.Errorf("FrameType = %d, want %d", frameType, FrameTypeData)
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("Data = %q, want %q", data, testData)
	}
}

func TestReadFrameFromReader_NilReader(t *testing.T) {
	_, _, _, err := ReadFrameFromReader(nil)
	if err == nil {
		t.Error("ReadFrameFromReader should return error for nil reader")
	}
}

func TestReadFrameFromReader_EmptyData(t *testing.T) {
	var tunnelID [16]byte
	copy(tunnelID[:], "test-tunnel")

	// 写入空数据帧
	var buf bytes.Buffer
	err := WriteFrameToWriter(&buf, tunnelID, FrameTypeAck, nil)
	if err != nil {
		t.Fatalf("WriteFrameToWriter failed: %v", err)
	}

	// 读取
	_, frameType, data, err := ReadFrameFromReader(&buf)
	if err != nil {
		t.Fatalf("ReadFrameFromReader failed: %v", err)
	}

	if frameType != FrameTypeAck {
		t.Errorf("FrameType = %d, want %d", frameType, FrameTypeAck)
	}
	if len(data) != 0 {
		t.Errorf("Data should be empty, got %d bytes", len(data))
	}
}

func TestFrameTypeConstants(t *testing.T) {
	// 验证帧类型常量
	if FrameTypeData != 0x01 {
		t.Errorf("FrameTypeData = %x, want 0x01", FrameTypeData)
	}
	if FrameTypeTargetReady != 0x02 {
		t.Errorf("FrameTypeTargetReady = %x, want 0x02", FrameTypeTargetReady)
	}
	if FrameTypeClose != 0x03 {
		t.Errorf("FrameTypeClose = %x, want 0x03", FrameTypeClose)
	}
	if FrameTypeAck != 0x04 {
		t.Errorf("FrameTypeAck = %x, want 0x04", FrameTypeAck)
	}
}

func TestFrameHeaderSize(t *testing.T) {
	// TunnelID(16) + FrameType(1) + Length(4) = 21
	if FrameHeaderSize != 21 {
		t.Errorf("FrameHeaderSize = %d, want 21", FrameHeaderSize)
	}
}

func TestMaxFrameSize(t *testing.T) {
	// 64KB
	if MaxFrameSize != 64*1024 {
		t.Errorf("MaxFrameSize = %d, want %d", MaxFrameSize, 64*1024)
	}
}
