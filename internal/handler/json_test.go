package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "github.com/mgfan1/go-metrics/internal/model"
)

func postJSON(t *testing.T, ts *httptest.Server, path, body string) (*http.Response, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, string(data)
}

func TestUpdateJSON(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"валидный gauge", `{"id":"Alloc","type":"gauge","value":13.5}`, http.StatusOK},
		{"валидный counter", `{"id":"PollCount","type":"counter","delta":5}`, http.StatusOK},
		{"неизвестный тип", `{"id":"Alloc","type":"summary","value":1}`, http.StatusBadRequest},
		{"битый JSON", `{"id":`, http.StatusBadRequest},
		{"нет имени", `{"id":"","type":"gauge","value":1}`, http.StatusNotFound},
		{"gauge без значения", `{"id":"Alloc","type":"gauge"}`, http.StatusBadRequest},
		{"counter без значения", `{"id":"PollCount","type":"counter"}`, http.StatusBadRequest},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := newTestServer(t)
			defer ts.Close()

			resp, _ := postJSON(t, ts, "/update/", c.body)
			defer resp.Body.Close()
			assert.Equal(t, c.want, resp.StatusCode)
			if c.want == http.StatusOK {
				assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
			}
		})
	}
}

func TestUpdateJSONReturnsMetric(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, body := postJSON(t, ts, "/update/", `{"id":"Alloc","type":"gauge","value":13.5}`)
	defer resp.Body.Close()

	var m models.Metrics
	require.NoError(t, json.Unmarshal([]byte(body), &m))
	require.NotNil(t, m.Value)
	assert.Equal(t, 13.5, *m.Value)
	assert.Nil(t, m.Delta)
}

func TestUpdateJSONCounterAccumulates(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	first, _ := postJSON(t, ts, "/update/", `{"id":"PollCount","type":"counter","delta":5}`)
	defer first.Body.Close()

	resp, body := postJSON(t, ts, "/update/", `{"id":"PollCount","type":"counter","delta":3}`)
	defer resp.Body.Close()

	var m models.Metrics
	require.NoError(t, json.Unmarshal([]byte(body), &m))
	require.NotNil(t, m.Delta)
	assert.Equal(t, int64(8), *m.Delta)
}

func TestValueJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	seed, _ := postJSON(t, ts, "/update/", `{"id":"Alloc","type":"gauge","value":42.1}`)
	defer seed.Body.Close()

	resp, body := postJSON(t, ts, "/value/", `{"id":"Alloc","type":"gauge"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var m models.Metrics
	require.NoError(t, json.Unmarshal([]byte(body), &m))
	require.NotNil(t, m.Value)
	assert.Equal(t, 42.1, *m.Value)
	assert.Nil(t, m.Delta)

	resp, _ = postJSON(t, ts, "/value/", `{"id":"Unknown","type":"gauge"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	resp, _ = postJSON(t, ts, "/value/", `{"id":"Alloc","type":"summary"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestJSONAndPlainShareStorage(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	code, _ := do(t, ts, http.MethodPost, "/update/gauge/Alloc/42.1")
	require.Equal(t, http.StatusOK, code)

	resp, body := postJSON(t, ts, "/value/", `{"id":"Alloc","type":"gauge"}`)
	defer resp.Body.Close()

	var m models.Metrics
	require.NoError(t, json.Unmarshal([]byte(body), &m))
	require.NotNil(t, m.Value)
	assert.Equal(t, 42.1, *m.Value)
}

func TestUpdateJSONWithoutTrailingSlash(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, _ := postJSON(t, ts, "/update", `{"id":"Alloc","type":"gauge","value":1}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
