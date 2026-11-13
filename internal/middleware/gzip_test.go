package middleware

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func echoBody(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "не прочитал тело запроса", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func roundTrip(h http.Handler, payload string) (string, error) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte(payload)); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}

	req := httptest.NewRequest(http.MethodPost, "/updates/", &compressed)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "gzip" {
		return "", fmt.Errorf("статус %d, Content-Encoding %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}

	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		return "", err
	}
	got, err := io.ReadAll(zr)
	return string(got), err
}

func TestGzipReaderSurvivesBrokenBody(t *testing.T) {
	h := Gzip(http.HandlerFunc(echoBody))
	const batch = `[{"id":"Alloc","type":"gauge","value":1}]`

	for range 3 {
		req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader("это не gzip"))
		req.Header.Set("Content-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		got, err := roundTrip(h, batch)
		require.NoError(t, err)
		assert.Equal(t, batch, got, "после битого запроса распаковка должна работать как прежде")
	}
}

func TestGzipConcurrentRequestsDoNotMix(t *testing.T) {
	h := Gzip(http.HandlerFunc(echoBody))

	errs := make(chan error, 200)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			for j := range 10 {
				want := strings.Repeat(fmt.Sprintf(`{"id":"g%d_%d","type":"gauge","value":%d},`, i, j, i*j), 20)

				got, err := roundTrip(h, want)
				if err == nil && got != want {
					err = fmt.Errorf("запрос %d/%d получил чужое тело", i, j)
				}
				if err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}
