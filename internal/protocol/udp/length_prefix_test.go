package udp

import (
	"bytes"
	"testing"
)

func TestReadWriteLengthPrefixedPacket(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "normal data",
			data:    []byte("hello world"),
			wantErr: false,
		},
		{
			name:    "single byte",
			data:    []byte("a"),
			wantErr: false,
		},
		{
			name:    "large data",
			data:    make([]byte, 10000),
			wantErr: false,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true, // 空数据应该返回错误
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// 写入测试
			err := WriteLengthPrefixedPacket(&buf, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("WriteLengthPrefixedPacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// 读取测试
			got, err := ReadLengthPrefixedPacket(&buf)
			if err != nil {
				t.Errorf("ReadLengthPrefixedPacket() error = %v", err)
				return
			}

			// 验证数据一致性
			if !bytes.Equal(got, tt.data) {
				t.Errorf("ReadLengthPrefixedPacket() got = %v, want %v", got, tt.data)
			}
		})
	}
}

func TestReadLengthPrefixedPacket_InvalidLength(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantErr  bool
		errMsg   string
	}{
		{
			name:    "length too large",
			data:    []byte{0xFF, 0xFF, 0xFF, 0xFF}, // 超大长度
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name:    "zero length",
			data:    []byte{0x00, 0x00, 0x00, 0x00}, // 零长度
			wantErr: true,
			errMsg:  "invalid data length: 0",
		},
		{
			name:    "incomplete length prefix",
			data:    []byte{0x00, 0x00}, // 只有2字节
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBuffer(tt.data)
			_, err := ReadLengthPrefixedPacket(buf)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadLengthPrefixedPacket() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReadLengthPrefixedPacketWithMaxSize(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		maxSize uint32
		wantErr bool
	}{
		{
			name:    "within max size",
			data:    []byte("hello"),
			maxSize: 100,
			wantErr: false,
		},
		{
			name:    "exceeds max size",
			data:    make([]byte, 1000),
			maxSize: 100,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			
			// 先写入数据
			if err := WriteLengthPrefixedPacket(&buf, tt.data); err != nil {
				if !tt.wantErr {
					t.Fatalf("WriteLengthPrefixedPacket() error = %v", err)
				}
				return
			}

			// 使用自定义最大大小读取
			_, err := ReadLengthPrefixedPacketWithMaxSize(&buf, tt.maxSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadLengthPrefixedPacketWithMaxSize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWriteLengthPrefixedPacket_TooLarge(t *testing.T) {
	// 创建一个超过最大大小的数据包
	data := make([]byte, MaxUDPPacketSize+1)
	
	var buf bytes.Buffer
	err := WriteLengthPrefixedPacket(&buf, data)
	
	if err == nil {
		t.Error("WriteLengthPrefixedPacket() should return error for oversized data")
	}
}

func BenchmarkWriteLengthPrefixedPacket(b *testing.B) {
	data := make([]byte, 1024)
	var buf bytes.Buffer
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		WriteLengthPrefixedPacket(&buf, data)
	}
}

func BenchmarkReadLengthPrefixedPacket(b *testing.B) {
	data := make([]byte, 1024)
	var buf bytes.Buffer
	WriteLengthPrefixedPacket(&buf, data)
	
	testData := buf.Bytes()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := bytes.NewBuffer(testData)
		ReadLengthPrefixedPacket(buf)
	}
}

