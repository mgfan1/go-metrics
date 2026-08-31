package handler

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rawClient() *http.Client {
	return &http.Client{Transport: &http.Transport{DisableCompression: true}}
}

func gzipBody(t *testing.T, body string) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return &buf
}

func TestGzipResponse(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	seed, _ := postJSON(t, ts, "/update/", `{"id":"Alloc","type":"gauge","value":13.5}`)
	defer seed.Body.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/value/",
		strings.NewReader(`{"id":"Alloc","type":"gauge"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := rawClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

	zr, err := gzip.NewReader(resp.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)

	assert.Contains(t, string(body), `"value":13.5`)
}

func TestGzipRequest(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/update/",
		gzipBody(t, `{"id":"Alloc","type":"gauge","value":77.5}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := rawClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := postJSON(t, ts, "/value/", `{"id":"Alloc","type":"gauge"}`)
	defer resp.Body.Close()
	assert.Contains(t, body, `"value":77.5`)
}

func TestGzipBrokenRequestBody(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/update/",
		strings.NewReader("это не gzip"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := rawClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGzipListPage(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	seed, _ := postJSON(t, ts, "/update/", `{"id":"Alloc","type":"gauge","value":1}`)
	defer seed.Body.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	require.NoError(t, err)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := rawClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	zr, err := gzip.NewReader(resp.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)

	assert.Contains(t, string(body), "Alloc")
}

func TestNoGzipWithoutAcceptEncoding(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	require.NoError(t, err)

	resp, err := rawClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, resp.Header.Get("Content-Encoding"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<!DOCTYPE html>")
}
