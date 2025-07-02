package io

import (
	"compress/gzip"
	"context"
	"io"
	"tunnox-core/internal/utils"
)

type GzipReader struct {
	reader     io.Reader
	gzipReader *gzip.Reader
	utils.Dispose
}

func (r *GzipReader) Read(p []byte) (n int, err error) {
	if r.IsClosed() {
		return 0, io.EOF
	}

	if r.gzipReader == nil {
		// 延迟初始化gzip reader
		gzipReader, err := gzip.NewReader(r.reader)
		if err != nil {
			return 0, err
		}
		r.gzipReader = gzipReader
	}

	return r.gzipReader.Read(p)
}

func (r *GzipReader) onClose() {
	if r.gzipReader != nil {
		r.gzipReader.Close()
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
	utils.Dispose
}

func (w *GzipWriter) Write(p []byte) (n int, err error) {
	if w.IsClosed() {
		return 0, io.ErrClosedPipe
	}
	return w.gzipWriter.Write(p)
}

func (w *GzipWriter) onClose() {
	if w.gzipWriter != nil {
		w.gzipWriter.Close()
		w.gzipWriter = nil
	}
}

func NewGzipWriter(writer io.Writer, parentCtx context.Context) *GzipWriter {
	w := &GzipWriter{writer: writer}
	w.gzipWriter = gzip.NewWriter(writer)
	w.SetCtx(parentCtx, w.onClose)
	return w
}
