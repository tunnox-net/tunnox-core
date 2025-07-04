package io

import (
	"compress/gzip"
	"context"
	"io"
	"sync"
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
				r.initErr = utils.WrapError(r.initErr, "failed to create gzip reader")
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
	gzipWriter *gzip.Writer
	closed     bool
	closeMutex sync.Mutex
	utils.Dispose
}

func (w *GzipWriter) Write(p []byte) (n int, err error) {
	if w.IsClosed() {
		return 0, utils.ErrStreamClosed
	}

	if w.gzipWriter == nil {
		return 0, utils.WrapError(utils.ErrStreamClosed, "gzip writer not initialized")
	}

	return w.gzipWriter.Write(p)
}

func (w *GzipWriter) onClose() {
	w.closeMutex.Lock()
	defer w.closeMutex.Unlock()

	if !w.closed && w.gzipWriter != nil {
		_ = w.gzipWriter.Close()
		w.gzipWriter = nil
		w.closed = true
	}
}

func NewGzipWriter(writer io.Writer, parentCtx context.Context) *GzipWriter {
	w := &GzipWriter{writer: writer}
	w.gzipWriter = gzip.NewWriter(writer)
	w.SetCtx(parentCtx, w.onClose)
	return w
}
