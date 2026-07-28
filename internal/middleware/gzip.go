package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

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
			g.zw = gzip.NewWriter(g.ResponseWriter)
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
	return g.zw.Close()
}

func compressible(contentType string) bool {
	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/html")
}

func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "не удалось распаковать тело запроса", http.StatusBadRequest)
				return
			}
			defer func() { _ = zr.Close() }()
			r.Body = zr
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
