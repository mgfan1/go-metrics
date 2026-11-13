package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/hash"
)

var batchBody = bytes.Repeat([]byte(`{"id":"HeapAlloc","type":"gauge","value":1234.5678},`), 35)

func discardAndReply(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(batchBody)
}

func serveOK(b *testing.B, h http.Handler, req *http.Request) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		b.Fatalf("ответ %d", rec.Code)
	}
}

func BenchmarkGzip(b *testing.B) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write(batchBody)
	require.NoError(b, err)
	require.NoError(b, zw.Close())

	h := Gzip(http.HandlerFunc(discardAndReply))

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(compressed.Bytes()))
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Accept-Encoding", "gzip")
		serveOK(b, h, req)
	}
}

func BenchmarkHash(b *testing.B) {
	const key = "ключ"
	signature := hash.Sign(batchBody, key)

	h := HashSign(key)(HashCheck(key, zap.NewNop())(http.HandlerFunc(discardAndReply)))

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(batchBody))
		req.Header.Set(hash.Header, signature)
		serveOK(b, h, req)
	}
}
