package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "github.com/mgfan1/go-metrics/internal/model"
)

func TestUpdatesJSONStatuses(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"валидный батч", `[{"id":"Alloc","type":"gauge","value":1.5},{"id":"PollCount","type":"counter","delta":2}]`, http.StatusOK},
		{"пустой батч", `[]`, http.StatusOK},
		{"битый JSON", `[{"id":`, http.StatusBadRequest},
		{"не массив", `{"id":"Alloc","type":"gauge","value":1}`, http.StatusBadRequest},
		{"нет имени", `[{"id":"","type":"gauge","value":1}]`, http.StatusNotFound},
		{"неизвестный тип", `[{"id":"Alloc","type":"summary","value":1}]`, http.StatusBadRequest},
		{"gauge без значения", `[{"id":"Alloc","type":"gauge"}]`, http.StatusBadRequest},
		{"counter без значения", `[{"id":"PollCount","type":"counter"}]`, http.StatusBadRequest},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := newTestServer(t)
			defer ts.Close()

			resp, _ := postJSON(t, ts, "/updates/", c.body)
			defer resp.Body.Close()
			assert.Equal(t, c.want, resp.StatusCode)
			if c.want == http.StatusOK {
				assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
			}
		})
	}
}

func TestUpdatesJSONEmptyBatchReturnsEmptyList(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, body := postJSON(t, ts, "/updates/", `[]`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var batch []models.Metrics
	require.NoError(t, json.Unmarshal([]byte(body), &batch))
	assert.Empty(t, batch)
}

func TestUpdatesJSONWithDuplicates(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `[{"id":"PollCount","type":"counter","delta":5},
	          {"id":"Alloc","type":"gauge","value":1.5},
	          {"id":"PollCount","type":"counter","delta":3},
	          {"id":"Alloc","type":"gauge","value":42.1}]`

	resp, _ := postJSON(t, ts, "/updates/", body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, counter := postJSON(t, ts, "/value/", `{"id":"PollCount","type":"counter"}`)
	defer resp.Body.Close()

	var got models.Metrics
	require.NoError(t, json.Unmarshal([]byte(counter), &got))
	require.NotNil(t, got.Delta)
	assert.Equal(t, int64(8), *got.Delta, "дельты counter с одним ID должны сложиться")

	resp, gauge := postJSON(t, ts, "/value/", `{"id":"Alloc","type":"gauge"}`)
	defer resp.Body.Close()
	require.NoError(t, json.Unmarshal([]byte(gauge), &got))
	require.NotNil(t, got.Value)
	assert.Equal(t, 42.1, *got.Value, "у gauge с одним ID должно остаться последнее значение")
}

func TestUpdatesJSONRejectsBatchWholly(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `[{"id":"Alloc","type":"gauge","value":1.5},{"id":"Bad","type":"summary","value":1}]`

	resp, _ := postJSON(t, ts, "/updates/", body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	resp, _ = postJSON(t, ts, "/value/", `{"id":"Alloc","type":"gauge"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "битый батч не должен применяться частично")
}

func TestUpdatesJSONWithoutTrailingSlash(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, _ := postJSON(t, ts, "/updates", `[{"id":"Alloc","type":"gauge","value":1}]`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
