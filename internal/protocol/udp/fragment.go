package udp

import (
	"encoding/binary"
	"fmt"
	"time"
)

const (
	// UDPFragmentMagic 分片包魔数
	UDPFragmentMagic = 0x554E // "UN" = UDP Network
	// UDPACKMagic ACK 包魔数
	UDPACKMagic = 0x5541 // "UA" = UDP ACK
	// UDPFragmentVersion 分片协议版本
	UDPFragmentVersion = 0x01

	// UDPFragmentHeaderSize 分片包头部大小
	UDPFragmentHeaderSize = 24
	// UDPACKHeaderSize ACK 包头部大小
	UDPACKHeaderSize = 16

	// UDPFragmentSize 每个分片最大大小（考虑 MTU）
	UDPFragmentSize = 1400
	// UDPFragmentThreshold 分片阈值（超过此大小才分片）
	UDPFragmentThreshold = 1200

	// UDP 超时参数
	UDPInitialRTT       = 200 * time.Millisecond
	UDPMaxRTT           = 2000 * time.Millisecond
	UDPRetryTimeout     = 100 * time.Millisecond
	UDPMaxRetries       = 5
	UDPBufferTimeout    = 30 * time.Second

	// UDP 缓冲区限制
	UDPMaxSendBuffers   = 100
	UDPMaxReceiveBuffers = 100
)

// FragmentFlags 分片标志位
type FragmentFlags uint8

const (
	FlagIsFragment FragmentFlags = 1 << iota // 是否为分片
	FlagIsFirst                               // 是否为第一片
	FlagIsLast                                // 是否为最后一片
	FlagNeedACK                               // 是否需要 ACK
)

// UDPFragmentPacket 分片包结构
type UDPFragmentPacket struct {
	Magic          uint16        // 魔数
	Version        uint8         // 版本
	Flags          FragmentFlags // 标志位
	FragmentGroupID uint64       // 分片组ID
	FragmentIndex  uint16       // 分片索引（0-based）
	TotalFragments uint16       // 总分片数
	OriginalSize   uint32       // 原始数据总大小
	FragmentSize   uint16       // 当前分片大小
	SequenceNum    uint16       // 序列号（用于重传检测）
	Data           []byte       // 分片数据
}

// Marshal 序列化分片包
func (p *UDPFragmentPacket) Marshal() ([]byte, error) {
	if len(p.Data) > UDPFragmentSize {
		return nil, fmt.Errorf("fragment size too large: %d > %d", len(p.Data), UDPFragmentSize)
	}

	buf := make([]byte, UDPFragmentHeaderSize+len(p.Data))
	offset := 0

	// Magic (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:], UDPFragmentMagic)
	offset += 2

	// Version (1 byte)
	buf[offset] = UDPFragmentVersion
	offset += 1

	// Flags (1 byte)
	buf[offset] = uint8(p.Flags)
	offset += 1

	// FragmentGroupID (8 bytes)
	binary.BigEndian.PutUint64(buf[offset:], p.FragmentGroupID)
	offset += 8

	// FragmentIndex (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:], p.FragmentIndex)
	offset += 2

	// TotalFragments (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:], p.TotalFragments)
	offset += 2

	// OriginalSize (4 bytes)
	binary.BigEndian.PutUint32(buf[offset:], p.OriginalSize)
	offset += 4

	// FragmentSize (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(p.Data)))
	offset += 2

	// SequenceNum (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:], p.SequenceNum)
	offset += 2

	// Data
	copy(buf[offset:], p.Data)

	return buf, nil
}

// Unmarshal 反序列化分片包
func UnmarshalFragmentPacket(data []byte) (*UDPFragmentPacket, error) {
	if len(data) < UDPFragmentHeaderSize {
		return nil, fmt.Errorf("packet too short: %d < %d", len(data), UDPFragmentHeaderSize)
	}

	p := &UDPFragmentPacket{}
	offset := 0

	// Magic (2 bytes)
	magic := binary.BigEndian.Uint16(data[offset:])
	if magic != UDPFragmentMagic {
		return nil, fmt.Errorf("invalid magic: 0x%04X", magic)
	}
	offset += 2

	// Version (1 byte)
	p.Version = data[offset]
	if p.Version != UDPFragmentVersion {
		return nil, fmt.Errorf("unsupported version: %d", p.Version)
	}
	offset += 1

	// Flags (1 byte)
	p.Flags = FragmentFlags(data[offset])
	offset += 1

	// FragmentGroupID (8 bytes)
	p.FragmentGroupID = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// FragmentIndex (2 bytes)
	p.FragmentIndex = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// TotalFragments (2 bytes)
	p.TotalFragments = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// OriginalSize (4 bytes)
	p.OriginalSize = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// FragmentSize (2 bytes)
	p.FragmentSize = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// SequenceNum (2 bytes)
	p.SequenceNum = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// Data
	expectedSize := int(p.FragmentSize)
	if len(data)-offset < expectedSize {
		return nil, fmt.Errorf("insufficient data: %d < %d", len(data)-offset, expectedSize)
	}
	p.Data = make([]byte, expectedSize)
	copy(p.Data, data[offset:offset+expectedSize])

	return p, nil
}

// UDPACKPacket ACK 包结构
type UDPACKPacket struct {
	Magic            uint16 // 魔数
	Version          uint8  // 版本
	Flags            uint8  // 标志位（保留）
	FragmentGroupID  uint64 // 分片组ID
	ReceivedBits     uint16 // 位图（最多 16 个分片）
	LastReceivedIndex uint16 // 最后接收的分片索引
}

// Marshal 序列化 ACK 包
func (a *UDPACKPacket) Marshal() ([]byte, error) {
	buf := make([]byte, UDPACKHeaderSize)
	offset := 0

	// Magic (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:], UDPACKMagic)
	offset += 2

	// Version (1 byte)
	buf[offset] = UDPFragmentVersion
	offset += 1

	// Flags (1 byte)
	buf[offset] = a.Flags
	offset += 1

	// FragmentGroupID (8 bytes)
	binary.BigEndian.PutUint64(buf[offset:], a.FragmentGroupID)
	offset += 8

	// ReceivedBits (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:], a.ReceivedBits)
	offset += 2

	// LastReceivedIndex (2 bytes)
	binary.BigEndian.PutUint16(buf[offset:], a.LastReceivedIndex)

	return buf, nil
}

// UnmarshalACKPacket 反序列化 ACK 包
func UnmarshalACKPacket(data []byte) (*UDPACKPacket, error) {
	if len(data) < UDPACKHeaderSize {
		return nil, fmt.Errorf("ACK packet too short: %d < %d", len(data), UDPACKHeaderSize)
	}

	a := &UDPACKPacket{}
	offset := 0

	// Magic (2 bytes)
	magic := binary.BigEndian.Uint16(data[offset:])
	if magic != UDPACKMagic {
		return nil, fmt.Errorf("invalid ACK magic: 0x%04X", magic)
	}
	offset += 2

	// Version (1 byte)
	a.Version = data[offset]
	offset += 1

	// Flags (1 byte)
	a.Flags = data[offset]
	offset += 1

	// FragmentGroupID (8 bytes)
	a.FragmentGroupID = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// ReceivedBits (2 bytes)
	a.ReceivedBits = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// LastReceivedIndex (2 bytes)
	a.LastReceivedIndex = binary.BigEndian.Uint16(data[offset:])

	return a, nil
}

// IsFragmentPacket 检查是否为分片包
func IsFragmentPacket(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	magic := binary.BigEndian.Uint16(data)
	return magic == UDPFragmentMagic
}

// IsACKPacket 检查是否为 ACK 包
func IsACKPacket(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	magic := binary.BigEndian.Uint16(data)
	return magic == UDPACKMagic
}

// CalculateFragments 计算分片参数
func CalculateFragments(dataSize int) (fragmentSize int, totalFragments int) {
	if dataSize <= UDPFragmentThreshold {
		return dataSize, 1 // 不分片
	}

	// 计算分片数（向上取整）
	totalFragments = (dataSize + UDPFragmentSize - 1) / UDPFragmentSize
	return UDPFragmentSize, totalFragments
}

// GetFragmentData 获取指定索引的分片数据
func GetFragmentData(data []byte, fragmentIndex int, fragmentSize int, totalFragments int) []byte {
	start := fragmentIndex * fragmentSize
	end := start + fragmentSize

	// 最后一片可能小于 fragmentSize
	if fragmentIndex == totalFragments-1 {
		end = len(data)
	}

	if end > len(data) {
		end = len(data)
	}

	if start >= len(data) {
		return nil
	}

	return data[start:end]
}

