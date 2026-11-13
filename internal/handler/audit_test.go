package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/audit"
	"github.com/mgfan1/go-metrics/internal/storage"
	"github.com/mgfan1/go-metrics/internal/storage/mocks"
)

type auditSpy struct {
	mu     sync.Mutex
	events []audit.Event
}

func (s *auditSpy) Publish(e audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, e)
}

func (s *auditSpy) got() []audit.Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.events)
}

func serve(router http.Handler, path, body string) int {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestAuditAfterStoredMetrics(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
		want []string
	}{
		{"метрика в адресе", "/update/gauge/Alloc/13.5", "", []string{"Alloc"}},
		{"метрика в JSON", "/update/", `{"id":"PollCount","type":"counter","delta":5}`, []string{"PollCount"}},
		{"батч", "/updates/", `[{"id":"Alloc","type":"gauge","value":1},{"id":"PollCount","type":"counter","delta":2}]`, []string{"Alloc", "PollCount"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				spy := &auditSpy{}
				router := New(storage.NewMemStorage(), nil, spy, zap.NewNop()).Router(zap.NewNop(), "")

				require.Equal(t, http.StatusOK, serve(router, c.path, c.body))

				events := spy.got()
				require.Len(t, events, 1)
				assert.Equal(t, c.want, events[0].Metrics)
				assert.Equal(t, "192.0.2.1", events[0].IPAddress)
				assert.Equal(t, time.Now().Unix(), events[0].TS)
			})
		})
	}
}

func TestNoAuditWithoutStoredMetrics(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
		want int
	}{
		{"неизвестный тип", "/update/summary/Alloc/1", "", http.StatusBadRequest},
		{"неизвестный тип в JSON", "/update/", `{"id":"Alloc","type":"summary","value":1}`, http.StatusBadRequest},
		{"битый JSON", "/update/", `{"id":`, http.StatusBadRequest},
		{"батч с ошибкой", "/updates/", `[{"id":"Alloc","type":"gauge","value":1},{"id":"PollCount","type":"counter"}]`, http.StatusBadRequest},
		{"пустой батч", "/updates/", `[]`, http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spy := &auditSpy{}
			router := New(storage.NewMemStorage(), nil, spy, zap.NewNop()).Router(zap.NewNop(), "")

			require.Equal(t, c.want, serve(router, c.path, c.body))
			assert.Empty(t, spy.got())
		})
	}
}

func TestNoAuditWhenStorageFails(t *testing.T) {
	boom := errors.New("база упала")

	cases := []struct {
		name   string
		path   string
		body   string
		expect func(*mocks.Repository)
	}{
		{"метрика в адресе", "/update/gauge/Alloc/1", "", func(s *mocks.Repository) {
			s.On("UpdateGauge", mock.Anything, "Alloc", 1.0).Return(boom)
		}},
		{"метрика в JSON", "/update/", `{"id":"PollCount","type":"counter","delta":1}`, func(s *mocks.Repository) {
			s.On("AddCounter", mock.Anything, "PollCount", int64(1)).Return(boom)
		}},
		{"батч", "/updates/", `[{"id":"Alloc","type":"gauge","value":1}]`, func(s *mocks.Repository) {
			s.On("UpdateBatch", mock.Anything, mock.Anything).Return(boom)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := mocks.NewRepository(t)
			c.expect(store)

			spy := &auditSpy{}
			router := New(store, nil, spy, zap.NewNop()).Router(zap.NewNop(), "")

			require.Equal(t, http.StatusInternalServerError, serve(router, c.path, c.body))
			assert.Empty(t, spy.got())
		})
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		remote string
		want   string
	}{
		{"192.0.2.1:52100", "192.0.2.1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"192.0.2.1", "192.0.2.1"},
	}

	for _, c := range cases {
		t.Run(c.remote, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/updates/", nil)
			r.RemoteAddr = c.remote
			assert.Equal(t, c.want, clientIP(r))
		})
	}
}
