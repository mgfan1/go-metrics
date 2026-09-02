package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/handler/mocks"
	"github.com/mgfan1/go-metrics/internal/storage"
)

func TestPing(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"база отвечает", nil, http.StatusOK},
		{"база не отвечает", errors.New("нет связи"), http.StatusInternalServerError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := mocks.NewPinger(t)
			db.On("PingContext", mock.Anything).Return(c.err)

			h := New(storage.NewMemStorage(), db, zap.NewNop())

			w := httptest.NewRecorder()
			h.Ping(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

			assert.Equal(t, c.want, w.Code)
		})
	}

	t.Run("база не настроена", func(t *testing.T) {
		h := New(storage.NewMemStorage(), nil, zap.NewNop())

		w := httptest.NewRecorder()
		h.Ping(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestPingRoute(t *testing.T) {
	db := mocks.NewPinger(t)
	db.On("PingContext", mock.Anything).Return(nil)

	h := New(storage.NewMemStorage(), db, zap.NewNop())
	ts := httptest.NewServer(h.Router(zap.NewNop(), ""))
	defer ts.Close()

	code, body := do(t, ts, http.MethodGet, "/ping")
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, body)
}
