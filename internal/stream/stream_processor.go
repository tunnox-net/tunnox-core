package stream

import (
	"context"
	"fmt"
	"io"
	"sync"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/utils"
)

// StreamProcessor 流处理器
// 本文件包含核心结构和初始化逻辑
// 读取相关方法在 stream_processor_read.go
// 写入相关方法在 stream_processor_write.go
type StreamProcessor struct {
	*dispose.ManagerBase
	reader    io.Reader
	writer    io.Writer
	readLock  sync.Mutex // 独立的读锁
	writeLock sync.Mutex // 独立的写锁
	bufferMgr *utils.BufferManager
	// 注意：加密功能已移至 internal/stream/transform 模块
}

func (ps *StreamProcessor) GetReader() io.Reader {
	return ps.reader
}

func (ps *StreamProcessor) GetWriter() io.Writer {
	return ps.writer
}

func NewStreamProcessor(reader io.Reader, writer io.Writer, parentCtx context.Context) *StreamProcessor {
	sp := &StreamProcessor{
		ManagerBase: dispose.NewManager("StreamProcessor", parentCtx),
		reader:      reader,
		writer:      writer,
		bufferMgr:   utils.NewBufferManager(parentCtx),
		// 注意：加密功能已移至 internal/stream/transform 模块
	}
	sp.AddCleanHandler(sp.onClose)
	return sp
}

func (ps *StreamProcessor) onClose() error {
	var errs []error

	// 关闭 buffer manager
	if ps.bufferMgr != nil {
		result := ps.bufferMgr.Close()
		if result.HasErrors() {
			errs = append(errs, fmt.Errorf("buffer manager cleanup failed: %v", result.Error()))
		}
	}

	// 关闭 writer（如果实现了 Close 方法且不是 Disposable，手动关闭）
	// 注意：实现了 Dispose 接口的对象（如 GzipWriter）会在 context 取消时自动关闭
	if ps.writer != nil {
		// 检查是否实现了 Dispose 接口（会自动关闭，不需要手动关闭）
		if _, isDisposable := ps.writer.(dispose.Disposable); !isDisposable {
			// 不是 Disposable，手动关闭
			if closer, ok := ps.writer.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					errs = append(errs, fmt.Errorf("writer close failed: %w", err))
				}
			} else if closer, ok := ps.writer.(interface{ Close() }); ok {
				closer.Close()
			}
		}
	}

	// 关闭 reader（如果实现了 Close 方法且不是 Disposable，手动关闭）
	// 注意：实现了 Dispose 接口的对象（如 GzipReader）会在 context 取消时自动关闭
	if ps.reader != nil {
		// 检查是否实现了 Dispose 接口（会自动关闭，不需要手动关闭）
		if _, isDisposable := ps.reader.(dispose.Disposable); !isDisposable {
			// 不是 Disposable，手动关闭
			if closer, ok := ps.reader.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					errs = append(errs, fmt.Errorf("reader close failed: %w", err))
				}
			} else if closer, ok := ps.reader.(interface{ Close() }); ok {
				closer.Close()
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stream processor cleanup errors: %v", errs)
	}
	return nil
}

// acquireReadLock 获取读取锁并检查状态
// 先获取锁再检查状态，避免 check-then-lock 竞态条件
func (ps *StreamProcessor) acquireReadLock() error {
	ps.readLock.Lock()
	if ps.ResourceBase.Dispose.IsClosed() {
		ps.readLock.Unlock()
		return io.EOF
	}
	if ps.reader == nil {
		ps.readLock.Unlock()
		return errors.ErrReaderNil
	}
	return nil
}

// acquireWriteLock 获取写入锁并检查状态
// 先获取锁再检查状态，避免 check-then-lock 竞态条件
func (ps *StreamProcessor) acquireWriteLock() error {
	ps.writeLock.Lock()
	if ps.ResourceBase.Dispose.IsClosed() {
		ps.writeLock.Unlock()
		return errors.ErrStreamClosed
	}
	if ps.writer == nil {
		ps.writeLock.Unlock()
		return errors.ErrWriterNil
	}
	return nil
}

// Close 关闭流处理器（兼容接口）
func (ps *StreamProcessor) Close() {
	ps.ResourceBase.Dispose.Close()
}

// CloseWithResult 关闭并返回结果（新方法）
func (ps *StreamProcessor) CloseWithResult() *dispose.DisposeResult {
	return ps.ResourceBase.Dispose.Close()
}
