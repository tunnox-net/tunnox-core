package udp

import (
	"encoding/binary"
	"fmt"
)

// TUTPHeader 定义了每个 UDP datagram 前的自定义头部。
// 注意：实际编码为大端字节序。
type TUTPHeader struct {
	Version    uint8  // 固定为 TUTPVersion
	Flags      uint8  // ACK/SYN/FIN/...

	SessionID  uint32 // 服务器与客户端协商的 Session ID
	StreamID   uint32 // 预留，目前可固定为 0

	PacketSeq  uint32 // 逻辑包序号，用于可靠传输与乱序重排
	FragSeq    uint16 // 当前分片序号：0..FragCount-1
	FragCount  uint16 // 分片总数：1 表示未分片

	AckSeq     uint32 // 累积 ACK：表示 <= AckSeq 的包已被确认
	WindowSize uint16 // 接收端通告窗口大小
	Reserved   uint16 // 保留字段

	Timestamp  uint32 // 发送时间戳（毫秒），用于 RTT 估算
}

// HeaderLength 返回固定头长度（单位：字节）。
func HeaderLength() int {
	return 32
}

// Encode 将头部编码到 buf 中，buf 必须至少为 HeaderLength() 大小。
// 返回写入的字节数和错误。
func (h *TUTPHeader) Encode(buf []byte) (int, error) {
	if len(buf) < HeaderLength() {
		return 0, fmt.Errorf("buffer too small: need %d bytes, got %d", HeaderLength(), len(buf))
	}

	offset := 0
	buf[offset] = h.Version
	offset++
	buf[offset] = h.Flags
	offset++

	binary.BigEndian.PutUint32(buf[offset:], h.SessionID)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], h.StreamID)
	offset += 4

	binary.BigEndian.PutUint32(buf[offset:], h.PacketSeq)
	offset += 4
	binary.BigEndian.PutUint16(buf[offset:], h.FragSeq)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], h.FragCount)
	offset += 2

	binary.BigEndian.PutUint32(buf[offset:], h.AckSeq)
	offset += 4
	binary.BigEndian.PutUint16(buf[offset:], h.WindowSize)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], h.Reserved)
	offset += 2

	binary.BigEndian.PutUint32(buf[offset:], h.Timestamp)
	offset += 4

	return HeaderLength(), nil
}

// DecodeHeader 从 buf 中解析 TUTPHeader。
// 返回解析出的头、消耗的长度、错误。
func DecodeHeader(buf []byte) (*TUTPHeader, int, error) {
	if len(buf) < HeaderLength() {
		return nil, 0, fmt.Errorf("buffer too small: need %d bytes, got %d", HeaderLength(), len(buf))
	}

	h := &TUTPHeader{}
	offset := 0

	h.Version = buf[offset]
	offset++
	h.Flags = buf[offset]
	offset++

	// 校验版本号
	if h.Version != TUTPVersion {
		return nil, 0, fmt.Errorf("invalid version: expected %d, got %d", TUTPVersion, h.Version)
	}

	h.SessionID = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	h.StreamID = binary.BigEndian.Uint32(buf[offset:])
	offset += 4

	h.PacketSeq = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	h.FragSeq = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	h.FragCount = binary.BigEndian.Uint16(buf[offset:])
	offset += 2

	// 校验分片数量
	if h.FragCount < 1 {
		return nil, 0, fmt.Errorf("invalid frag count: %d", h.FragCount)
	}

	h.AckSeq = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	h.WindowSize = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	h.Reserved = binary.BigEndian.Uint16(buf[offset:])
	offset += 2

	h.Timestamp = binary.BigEndian.Uint32(buf[offset:])
	offset += 4

	return h, HeaderLength(), nil
}

