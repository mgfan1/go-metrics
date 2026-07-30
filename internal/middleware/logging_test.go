package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func logged(t *testing.T, h http.HandlerFunc) map[string]any {
	t.Helper()

	core, logs := observer.New(zap.InfoLevel)
	Logging(zap.New(core))(h).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/value/gauge/Alloc", nil))

	entries := logs.All()
	require.Len(t, entries, 1)
	return entries[0].ContextMap()
}

func TestLoggingStatusWithoutWriteHeader(t *testing.T) {
	body := "13.5"

	fields := logged(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})

	assert.Equal(t, int64(http.StatusOK), fields["status"])
	assert.Equal(t, int64(len(body)), fields["size"])
}

func TestLoggingStatusForEmptyResponse(t *testing.T) {
	fields := logged(t, func(http.ResponseWriter, *http.Request) {})

	assert.Equal(t, int64(http.StatusOK), fields["status"])
	assert.Equal(t, int64(0), fields["size"])
}

func TestLoggingExplicitStatus(t *testing.T) {
	fields := logged(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "нет такой метрики", http.StatusNotFound)
	})

	assert.Equal(t, int64(http.StatusNotFound), fields["status"])
	assert.Equal(t, "/value/gauge/Alloc", fields["uri"])
	assert.Equal(t, http.MethodGet, fields["method"])
}
