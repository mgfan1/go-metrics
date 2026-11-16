package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/retry"
)

const remoteTimeout = 5 * time.Second

var _ Observer = (*HTTPObserver)(nil)

// HTTPObserver отправляет события POST-запросом на удалённый приёмник.
type HTTPObserver struct {
	endpoint string
	client   *http.Client
	retrier  *retry.Retrier
}

// NewHTTPObserver проверяет адрес endpoint и создаёт наблюдателя для удалённого приёмника.
func NewHTTPObserver(endpoint string, log *zap.Logger) (*HTTPObserver, error) {
	u, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return nil, fmt.Errorf("адрес приёмника аудита: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("адрес приёмника аудита %q: нужны схема http или https и хост", endpoint)
	}

	return &HTTPObserver{
		endpoint: endpoint,
		client:   &http.Client{Timeout: remoteTimeout},
		retrier:  retry.New(log),
	}, nil
}

// Notify отправляет событие в JSON, при сетевой ошибке и ответе 5xx повторяет попытку, другие ответы кроме 2xx считает ошибкой.
func (o *HTTPObserver) Notify(ctx context.Context, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}

	return o.retrier.Do(ctx, retriable, func() error { return o.send(ctx, body) })
}

type serverError struct {
	status string
}

func (e *serverError) Error() string {
	return "приёмник аудита ответил " + e.status
}

func retriable(err error) bool {
	var netErr net.Error
	var srvErr *serverError
	return errors.As(err, &netErr) || errors.As(err, &srvErr)
}

func (o *HTTPObserver) send(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	switch {
	case resp.StatusCode >= http.StatusInternalServerError:
		return &serverError{status: resp.Status}
	case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
		return fmt.Errorf("приёмник аудита ответил %s", resp.Status)
	}

	return nil
}
