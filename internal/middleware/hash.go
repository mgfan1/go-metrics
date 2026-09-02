package middleware

import (
	"bytes"
	"io"
	"net/http"

	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/hash"
)

type hashWriter struct {
	http.ResponseWriter
	buf    bytes.Buffer
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

func HashSign(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hw := &hashWriter{ResponseWriter: w}
			defer hw.flush(key)

			next.ServeHTTP(hw, r)
		})
	}
}

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

			body, err := io.ReadAll(r.Body)
			if err != nil {
				log.Warn("не прочитал тело запроса", zap.Error(err))
				http.Error(w, "не прочитал тело запроса", http.StatusBadRequest)
				return
			}
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
