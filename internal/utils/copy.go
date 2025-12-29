package utils

import (
	"io"

	"tunnox-core/internal/utils/iocopy"
)

// 类型别名，保持向后兼容
type CloseWriter = iocopy.CloseWriter
type BidirectionalCopyOptions = iocopy.Options
type BidirectionalCopyResult = iocopy.Result

// 错误别名
var (
	ErrNilReader = iocopy.ErrNilReader
	ErrNilWriter = iocopy.ErrNilWriter
)

// NewReadWriteCloser 创建 ReadWriteCloser 适配器
// 如果 Reader 或 Writer 为 nil，会返回错误
func NewReadWriteCloser(r io.Reader, w io.Writer, closeFunc func() error) (io.ReadWriteCloser, error) {
	return iocopy.NewReadWriteCloser(r, w, closeFunc)
}

// NewReadWriteCloserWithCloseWrite 创建支持半关闭的 ReadWriteCloser 适配器
func NewReadWriteCloserWithCloseWrite(r io.Reader, w io.Writer, closeFunc func() error, closeWriteFunc func() error) (io.ReadWriteCloser, error) {
	return iocopy.NewReadWriteCloserWithCloseWrite(r, w, closeFunc, closeWriteFunc)
}

// BidirectionalCopy 通用双向数据拷贝
func BidirectionalCopy(connA, connB io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	return iocopy.Bidirectional(connA, connB, options)
}

// SimpleBidirectionalCopy 简化版本（无转换器）
func SimpleBidirectionalCopy(connA, connB io.ReadWriteCloser, logPrefix string) *BidirectionalCopyResult {
	return iocopy.Simple(connA, connB, logPrefix)
}

// UDPBidirectionalCopy UDP 专用双向拷贝（保持包边界）
func UDPBidirectionalCopy(udpConn io.ReadWriteCloser, tunnelConn io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	return iocopy.UDP(udpConn, tunnelConn, options)
}
