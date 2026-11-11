package middleware

import (
	"bufio"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

var gzipWriters = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

var gzipReaders = sync.Pool{
	New: func() any { return &gzipReader{buf: bufio.NewReader(nil)} },
}

type gzipReader struct {
	buf *bufio.Reader
	zr  gzip.Reader
}

func (g *gzipReader) reset(r io.Reader) error {
	g.buf.Reset(r)
	return g.zr.Reset(g.buf)
}

func (g *gzipReader) release() {
	g.buf.Reset(nil)
	gzipReaders.Put(g)
}

type gzipWriter struct {
	http.ResponseWriter
	zw      *gzip.Writer
	decided bool
}

func (g *gzipWriter) WriteHeader(status int) {
	if !g.decided {
		g.decided = true
		if compressible(g.Header().Get("Content-Type")) {
			g.Header().Del("Content-Length")
			g.Header().Set("Content-Encoding", "gzip")
			g.zw = gzipWriters.Get().(*gzip.Writer)
			g.zw.Reset(g.ResponseWriter)
		}
	}
	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	if !g.decided {
		g.WriteHeader(http.StatusOK)
	}
	if g.zw != nil {
		return g.zw.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipWriter) Close() error {
	if g.zw == nil {
		return nil
	}

	err := g.zw.Close()
	g.zw.Reset(io.Discard)
	gzipWriters.Put(g.zw)
	g.zw = nil

	return err
}

func compressible(contentType string) bool {
	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/html")
}

func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			gr := gzipReaders.Get().(*gzipReader)
			defer gr.release()

			if err := gr.reset(r.Body); err != nil {
				http.Error(w, "не удалось распаковать тело запроса", http.StatusBadRequest)
				return
			}
			r.Body = &gr.zr
		}

		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Vary", "Accept-Encoding")

		gw := &gzipWriter{ResponseWriter: w}
		defer func() { _ = gw.Close() }()

		next.ServeHTTP(gw, r)
	})
}
