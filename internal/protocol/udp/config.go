package udp

import "time"

const (
	// TUTPVersion 协议版本号
	TUTPVersion uint8 = 1

	// Flag 位（位运算）
	FlagACK       uint8 = 0x01
	FlagSYN       uint8 = 0x02
	FlagFIN       uint8 = 0x04
	FlagRetrans   uint8 = 0x08
	FlagUnreliable uint8 = 0x10 // 预留

	// MTU 相关（字节）
	MaxUDPPayloadSize   = 1200 // UDP payload 上限（减去 IP/UDP 头后）
	MaxHeaderSize       = 32   // TUTPHeader 最大长度估算
	MaxDataPerDatagram  = MaxUDPPayloadSize - MaxHeaderSize

	// 窗口 & 重传
	DefaultSendWindowSize    = 64
	DefaultRecvWindowSize    = 64
	DefaultMaxRetransmit     = 5
	DefaultRetransmitTimeout = 500 * time.Millisecond

	// FragmentGroup
	DefaultFragmentGroupTTL            = 10 * time.Second
	DefaultMaxFragmentGroupsPerSession = 1024

	// Session 管理
	DefaultSessionIdleTimeout = 60 * time.Second
)

