package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type recorder struct {
	mu     sync.Mutex
	events []Event
	err    error
	closed bool
	late   bool
}

func (r *recorder) Notify(_ context.Context, e Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		r.late = true
	}
	r.events = append(r.events, e)
	return r.err
}

func (r *recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closed = true
	return nil
}

func (r *recorder) got() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.events)
}

func (r *recorder) notifiedAfterClose() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.late
}

type blocker struct {
	release chan struct{}
}

func (b *blocker) Notify(ctx context.Context, _ Event) error {
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPublisherNotifiesEveryObserver(t *testing.T) {
	first, second := &recorder{}, &recorder{}

	p := NewPublisher(zap.NewNop())
	p.Register(first)
	p.Register(second)

	events := []Event{
		{TS: 1, Metrics: []string{"Alloc"}, IPAddress: "192.0.2.1"},
		{TS: 2, Metrics: []string{"PollCount", "Frees"}, IPAddress: "192.0.2.2"},
	}
	for _, e := range events {
		p.Publish(e)
	}
	require.NoError(t, p.Close())

	assert.Equal(t, events, first.got())
	assert.Equal(t, events, second.got())
	assert.True(t, first.closed && second.closed, "Close должен закрыть наблюдателей")
	assert.False(t, first.notifiedAfterClose() || second.notifiedAfterClose(), "наблюдателя закрыли раньше, чем дорассылка кончилась")
}

func TestPublisherKeepsGoingAfterObserverError(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	broken := &recorder{err: errors.New("приёмник недоступен")}
	healthy := &recorder{}

	p := NewPublisher(zap.New(core))
	p.Register(broken)
	p.Register(healthy)

	p.Publish(Event{TS: 1, Metrics: []string{"Alloc"}})
	require.NoError(t, p.Close())

	assert.Len(t, healthy.got(), 1)
	assert.Equal(t, 1, logs.FilterMessage("не отправил событие аудита").Len())
}

func TestPublisherSlowObserverDoesNotDelayOthers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		slow := &blocker{release: make(chan struct{})}
		fast := &recorder{}

		p := NewPublisher(zap.NewNop())
		p.Register(slow)
		p.Register(fast)

		p.Publish(Event{TS: 1, Metrics: []string{"Alloc"}})
		synctest.Wait()

		assert.Len(t, fast.got(), 1, "событие должно дойти до быстрого наблюдателя, пока медленный занят")

		close(slow.release)
		require.NoError(t, p.Close())
	})
}

func TestPublisherDropsEventsWhenQueueIsFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		core, logs := observer.New(zap.WarnLevel)
		slow := &blocker{release: make(chan struct{})}

		p := NewPublisher(zap.New(core))
		p.Register(slow)

		p.Publish(Event{TS: 1})
		synctest.Wait()

		for i := range queueSize {
			p.Publish(Event{TS: int64(i + 2)})
		}
		p.Publish(Event{TS: -1, Metrics: []string{"Lost"}})

		assert.Equal(t, 1, logs.FilterMessage("очередь аудита переполнена, событие пропущено").Len())

		close(slow.release)
		require.NoError(t, p.Close())
	})
}

func TestPublisherCloseGivesUpOnHangingObserver(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		core, logs := observer.New(zap.WarnLevel)
		hanging := &blocker{release: make(chan struct{})}

		p := NewPublisher(zap.New(core))
		p.Register(hanging)

		for i := range 3 {
			p.Publish(Event{TS: int64(i + 1)})
		}
		synctest.Wait()

		start := time.Now()
		require.NoError(t, p.Close())
		assert.Equal(t, closeTimeout, time.Since(start))

		assert.Equal(t, 1, logs.FilterMessage("не отправил событие аудита").Len())
		stopped := logs.FilterMessage("аудит остановлен по таймауту, часть событий не отправлена").All()
		require.Len(t, stopped, 1)
		assert.Equal(t, int64(2), stopped[0].ContextMap()["skipped"])
	})
}

func TestPublisherIgnoresEventsAfterClose(t *testing.T) {
	r := &recorder{}

	p := NewPublisher(zap.NewNop())
	p.Register(r)
	require.NoError(t, p.Close())

	p.Publish(Event{TS: 1, Metrics: []string{"Alloc"}})
	require.NoError(t, p.Close())

	assert.Empty(t, r.got())
}

func TestFileObserverAppendsLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	require.NoError(t, os.WriteFile(path, []byte(`{"ts":1}`+"\n"), 0o600))

	o, err := NewFileObserver(path)
	require.NoError(t, err)

	require.NoError(t, o.Notify(t.Context(), Event{TS: 2, Metrics: []string{"Alloc", "Frees"}, IPAddress: "192.0.2.1"}))
	require.NoError(t, o.Notify(t.Context(), Event{TS: 3, Metrics: []string{"PollCount"}, IPAddress: "192.0.2.2"}))
	require.NoError(t, o.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	require.Len(t, lines, 3, "старые строки должны остаться, новые дописаться в конец")
	assert.Equal(t, `{"ts":1}`, lines[0])
	assert.JSONEq(t, `{"ts":2,"metrics":["Alloc","Frees"],"ip_address":"192.0.2.1"}`, lines[1])
	assert.JSONEq(t, `{"ts":3,"metrics":["PollCount"],"ip_address":"192.0.2.2"}`, lines[2])
}

func TestFileObserverBadPath(t *testing.T) {
	_, err := NewFileObserver(filepath.Join(t.TempDir(), "нет такой папки", "audit.log"))
	assert.ErrorContains(t, err, "файл аудита")
}

type receivedRequest struct {
	method      string
	contentType string
	event       Event
	decodeErr   error
}

func TestHTTPObserverPostsEvent(t *testing.T) {
	received := make(chan receivedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got := receivedRequest{method: r.Method, contentType: r.Header.Get("Content-Type")}
		got.decodeErr = json.NewDecoder(r.Body).Decode(&got.event)
		received <- got
	}))
	defer srv.Close()

	o, err := NewHTTPObserver(srv.URL + "/audit")
	require.NoError(t, err)

	e := Event{TS: 5, Metrics: []string{"Alloc", "PollCount"}, IPAddress: "192.0.2.1"}
	require.NoError(t, o.Notify(t.Context(), e))

	got := <-received
	require.NoError(t, got.decodeErr)
	assert.Equal(t, http.MethodPost, got.method)
	assert.Equal(t, "application/json", got.contentType)
	assert.Equal(t, e, got.event)
}

func TestHTTPObserverFailsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	o, err := NewHTTPObserver(srv.URL)
	require.NoError(t, err)

	assert.ErrorContains(t, o.Notify(t.Context(), Event{TS: 1}), "503")
}

func TestHTTPObserverFailsWhenReceiverIsDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	o, err := NewHTTPObserver(srv.URL)
	require.NoError(t, err)
	srv.Close()

	assert.Error(t, o.Notify(t.Context(), Event{TS: 1}))
}

func TestNewHTTPObserverRejectsBadURL(t *testing.T) {
	for _, endpoint := range []string{"", "localhost:9000/audit", "ftp://audit.local/events", "http://", "/audit"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := NewHTTPObserver(endpoint)
			assert.ErrorContains(t, err, "адрес приёмника аудита")
		})
	}
}
