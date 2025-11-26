package utils

import (
	"io"
	"sync"
	"tunnox-core/internal/stream/transform"
)

// readWriteCloser 适配器：将 io.Reader 和 io.Writer 组合成 io.ReadWriteCloser
type readWriteCloser struct {
	io.Reader
	io.Writer
	closeFunc func() error
}

func (rw *readWriteCloser) Close() error {
	if rw.closeFunc != nil {
		return rw.closeFunc()
	}
	return nil
}

// NewReadWriteCloser 创建 ReadWriteCloser 适配器
func NewReadWriteCloser(r io.Reader, w io.Writer, closeFunc func() error) io.ReadWriteCloser {
	return &readWriteCloser{
		Reader:    r,
		Writer:    w,
		closeFunc: closeFunc,
	}
}

// BidirectionalCopyOptions 双向拷贝配置选项
type BidirectionalCopyOptions struct {
	// 流转换器（处理压缩、加密）
	Transformer transform.StreamTransformer
	
	// 日志前缀（用于区分不同的拷贝场景）
	LogPrefix string
	
	// 拷贝完成后的回调（可选）
	OnComplete func(sent, received int64, err error)
}

// BidirectionalCopyResult 双向拷贝结果
type BidirectionalCopyResult struct {
	BytesSent     int64 // A→B 发送字节数
	BytesReceived int64 // B→A 接收字节数
	SendError     error // A→B 错误
	ReceiveError  error // B→A 错误
}

// BidirectionalCopy 通用双向数据拷贝
// connA 和 connB 是两个需要双向传输的连接
// options 包含转换器配置和日志前缀
// 
// 数据流向：
//   A → B: 从 connA 读取 → 应用转换器（压缩、加密） → 写入 connB
//   B → A: 从 connB 读取 → 应用转换器（解密、解压） → 写入 connA
//
// 返回拷贝结果，包含发送/接收字节数和错误信息
func BidirectionalCopy(connA, connB io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	// 默认选项
	if options == nil {
		options = &BidirectionalCopyOptions{}
	}
	if options.LogPrefix == "" {
		options.LogPrefix = "BidirectionalCopy"
	}
	if options.Transformer == nil {
		options.Transformer = &transform.NoOpTransformer{}
	}

	result := &BidirectionalCopyResult{}
	var wg sync.WaitGroup
	wg.Add(2)

	// A → B（压缩 + 加密）
	go func() {
		defer wg.Done()
		defer connB.Close() // 关闭写端

		// 包装 Writer：压缩 → 加密
		writerB, err := options.Transformer.WrapWriter(connB)
		if err != nil {
			Errorf("%s: failed to wrap writer: %v", options.LogPrefix, err)
			result.SendError = err
			return
		}
		defer writerB.Close() // 确保 flush 缓冲
		
		written, err := io.Copy(writerB, connA)
		result.BytesSent = written
		result.SendError = err

		if err != nil && err != io.EOF {
			Debugf("%s: A→B error: %v", options.LogPrefix, err)
		}
	}()

	// B → A（解密 + 解压）
	go func() {
		defer wg.Done()
		defer connA.Close() // 关闭写端
		
		// 包装 Reader：解密 → 解压
		readerB, err := options.Transformer.WrapReader(connB)
		if err != nil {
			Errorf("%s: failed to wrap reader: %v", options.LogPrefix, err)
			result.ReceiveError = err
			return
		}
		
		written, err := io.Copy(connA, readerB)
		result.BytesReceived = written
		result.ReceiveError = err

		if err != nil && err != io.EOF {
			Debugf("%s: B→A error: %v", options.LogPrefix, err)
		}
	}()

	wg.Wait()
	Debugf("%s: completed (sent: %d, received: %d)", 
		options.LogPrefix, result.BytesSent, result.BytesReceived)

	// 执行回调
	if options.OnComplete != nil {
		var err error
		if result.SendError != nil {
			err = result.SendError
		} else if result.ReceiveError != nil {
			err = result.ReceiveError
		}
		options.OnComplete(result.BytesSent, result.BytesReceived, err)
	}

	return result
}

// SimpleBidirectionalCopy 简化版本（无转换器）
func SimpleBidirectionalCopy(connA, connB io.ReadWriteCloser, logPrefix string) *BidirectionalCopyResult {
	return BidirectionalCopy(connA, connB, &BidirectionalCopyOptions{
		LogPrefix: logPrefix,
	})
}

