package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/hash"
	"github.com/mgfan1/go-metrics/internal/storage"
)

const key = "ключ"

func newSignedServer(t *testing.T) *httptest.Server {
	t.Helper()
	h := New(storage.NewMemStorage(), nil, zap.NewNop())
	return httptest.NewServer(h.Router(zap.NewNop(), key))
}

func TestHashRejectsGzipBatchWithBadSignature(t *testing.T) {
	ts := newSignedServer(t)
	defer ts.Close()

	batch := `[{"id":"Alloc","type":"gauge","value":13.5}]`

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/updates/", gzipBody(t, batch))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set(hash.Header, hash.Sign([]byte(batch), "чужой"))

	resp, err := rawClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	code, _ := do(t, ts, http.MethodGet, "/value/gauge/Alloc")
	assert.Equal(t, http.StatusNotFound, code, "метрика с неверной подписью не должна сохраниться")
}

func TestHashAcceptsGzipBatchWithValidSignature(t *testing.T) {
	ts := newSignedServer(t)
	defer ts.Close()

	batch := `[{"id":"Alloc","type":"gauge","value":13.5}]`

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/updates/", gzipBody(t, batch))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set(hash.Header, hash.Sign([]byte(batch), key))

	resp, err := rawClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	code, body := do(t, ts, http.MethodGet, "/value/gauge/Alloc")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "13.5", body)
}

func TestHashSignsResponseWithoutBody(t *testing.T) {
	ts := newSignedServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/update/gauge/Alloc/13.5", nil)
	require.NoError(t, err)

	resp, err := rawClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Empty(t, body)

	signature := resp.Header.Get(hash.Header)
	require.NotEmpty(t, signature, "ответ без тела не подписан")
	assert.True(t, hash.Valid(body, key, signature))
}
