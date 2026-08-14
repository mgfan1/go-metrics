package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/storage"
)

type fakePinger struct {
	err error
}

func (p fakePinger) PingContext(_ context.Context) error {
	return p.err
}

func TestPing(t *testing.T) {
	cases := []struct {
		name string
		db   Pinger
		want int
	}{
		{"база отвечает", fakePinger{}, http.StatusOK},
		{"база не отвечает", fakePinger{err: errors.New("нет связи")}, http.StatusInternalServerError},
		{"база не настроена", nil, http.StatusInternalServerError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := New(storage.NewMemStorage(), c.db, zap.NewNop())

			w := httptest.NewRecorder()
			h.Ping(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

			assert.Equal(t, c.want, w.Code)
		})
	}
}

func TestPingRoute(t *testing.T) {
	h := New(storage.NewMemStorage(), fakePinger{}, zap.NewNop())
	ts := httptest.NewServer(h.Router(zap.NewNop()))
	defer ts.Close()

	code, body := do(t, ts, http.MethodGet, "/ping")
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, body)
}
