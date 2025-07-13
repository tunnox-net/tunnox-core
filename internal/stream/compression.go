package stream

import (
	"compress/gzip"
	"context"
	"io"
	"sync"
	"tunnox-core/internal/errors"
	"tunnox-core/internal/utils"
)

type GzipReader struct {
	reader     io.Reader
	gzipReader *gzip.Reader
	initOnce   sync.Once
	initErr    error
	utils.Dispose
}

func (r *GzipReader) Read(p []byte) (n int, err error) {
	if r.IsClosed() {
		return 0, io.EOF
	}

	// 延迟初始化，确保线程安全
	r.initOnce.Do(func() {
		if r.gzipReader == nil {
			r.gzipReader, r.initErr = gzip.NewReader(r.reader)
			if r.initErr != nil {
				r.initErr = errors.WrapError(r.initErr, "failed to create gzip reader")
			}
		}
	})

	if r.initErr != nil {
		return 0, r.initErr
	}

	return r.gzipReader.Read(p)
}

func (r *GzipReader) onClose() {
	if r.gzipReader != nil {
		_ = r.gzipReader.Close()
		r.gzipReader = nil
	}
}

func NewGzipReader(reader io.Reader, parentCtx context.Context) *GzipReader {
	sReader := &GzipReader{reader: reader}
	sReader.SetCtx(parentCtx, sReader.onClose)
	return sReader
}

type GzipWriter struct {
	writer     io.Writer
	gWriter    *gzip.Writer
	closed     bool
	closeMutex sync.Mutex
	utils.Dispose
}

func (w *GzipWriter) Write(p []byte) (n int, err error) {
	if w.IsClosed() {
		return 0, errors.ErrStreamClosed
	}

	if w.gWriter == nil {
		return 0, errors.WrapError(errors.ErrStreamClosed, "gzip writer not initialized")
	}

	return w.gWriter.Write(p)
}

func (w *GzipWriter) onClose() {
	w.closeMutex.Lock()
	defer w.closeMutex.Unlock()

	if w.closed {
		return
	}
	w.closed = true

	if w.gWriter != nil {
		_ = w.gWriter.Close()
		w.gWriter = nil
	}
}

func NewGzipWriter(writer io.Writer, parentCtx context.Context) *GzipWriter {
	w := &GzipWriter{writer: writer}
	w.gWriter = gzip.NewWriter(writer)
	w.SetCtx(parentCtx, w.onClose)
	return w
}

// Close 关闭Gzip读取器（兼容接口）
func (r *GzipReader) Close() {
	r.Dispose.Close()
}

// Close 关闭Gzip写入器（兼容接口）
func (w *GzipWriter) Close() {
	w.Dispose.Close()
}
