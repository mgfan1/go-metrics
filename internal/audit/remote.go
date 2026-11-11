package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const remoteTimeout = 5 * time.Second

var _ Observer = (*HTTPObserver)(nil)

// HTTPObserver отправляет события POST-запросом на удалённый приёмник.
type HTTPObserver struct {
	endpoint string
	client   *http.Client
}

// NewHTTPObserver проверяет адрес endpoint и создаёт наблюдателя для удалённого приёмника.
func NewHTTPObserver(endpoint string) (*HTTPObserver, error) {
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
	}, nil
}

// Notify отправляет событие в JSON и считает ошибкой любой ответ, кроме 2xx.
func (o *HTTPObserver) Notify(ctx context.Context, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}

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

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("приёмник аудита ответил %s", resp.Status)
	}

	return nil
}
