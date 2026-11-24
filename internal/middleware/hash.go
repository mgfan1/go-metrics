package middleware

import (
	"bytes"
	"io"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/hash"
)

const maxPooledBuffer = 64 << 10

var buffers = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func getBuffer() *bytes.Buffer {
	buf := buffers.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() > maxPooledBuffer {
		return
	}
	buffers.Put(buf)
}

type hashWriter struct {
	http.ResponseWriter
	buf    *bytes.Buffer
	status int
}

func (h *hashWriter) WriteHeader(status int) {
	if h.status == 0 {
		h.status = status
	}
}

func (h *hashWriter) Write(b []byte) (int, error) {
	return h.buf.Write(b)
}

func (h *hashWriter) flush(key string) {
	if h.status == 0 {
		h.status = http.StatusOK
	}

	body := h.buf.Bytes()
	h.Header().Set(hash.Header, hash.Sign(body, key))

	h.ResponseWriter.WriteHeader(h.status)
	_, _ = h.ResponseWriter.Write(body)
}

// HashSign подписывает каждый ответ ключом key, пустой key подпись отключает.
func HashSign(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buf := getBuffer()
			defer putBuffer(buf)

			hw := &hashWriter{ResponseWriter: w, buf: buf}
			defer hw.flush(key)

			next.ServeHTTP(hw, r)
		})
	}
}

// HashCheck отклоняет кодом 400 запрос с неверной подписью тела, запрос без подписи пропускает.
func HashCheck(key string, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get(hash.Header)
			if got == "" {
				next.ServeHTTP(w, r)
				return
			}

			buf := getBuffer()
			defer putBuffer(buf)

			if _, err := buf.ReadFrom(r.Body); err != nil {
				log.Warn("не прочитал тело запроса", zap.Error(err))
				http.Error(w, "не прочитал тело запроса", http.StatusBadRequest)
				return
			}
			body := buf.Bytes()
			if !hash.Valid(body, key, got) {
				log.Warn("подпись запроса не совпала", zap.String("uri", r.RequestURI))
				http.Error(w, "подпись не совпала", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			next.ServeHTTP(w, r)
		})
	}
}
