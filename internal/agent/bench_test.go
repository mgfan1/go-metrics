package agent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkSnapshotAndSplit(b *testing.B) {
	a := newAgent("http://localhost:8080", "")
	a.poll()

	for b.Loop() {
		split(a.snapshot(), 4)
	}
}

func BenchmarkSend(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()

	a := newAgent(srv.URL, "ключ")
	a.poll()
	batch := a.snapshot().batch
	ctx := b.Context()

	for b.Loop() {
		if err := a.send(ctx, batch); err != nil {
			b.Fatal(err)
		}
	}
}
