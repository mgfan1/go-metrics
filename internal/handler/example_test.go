package handler_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/handler"
	"github.com/mgfan1/go-metrics/internal/hash"
	"github.com/mgfan1/go-metrics/internal/storage"
)

func ExampleMetricsHandler_Update() {
	router := handler.New(storage.NewMemStorage(), nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	for _, path := range []string{"/update/counter/PollCount/5", "/update/counter/PollCount/3", "/update/gauge/Alloc/abc"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		fmt.Println(path, rec.Code)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/value/counter/PollCount", nil))
	fmt.Println(rec.Body.String())

	// Output:
	// /update/counter/PollCount/5 200
	// /update/counter/PollCount/3 200
	// /update/gauge/Alloc/abc 400
	// 8
}

func ExampleMetricsHandler_Value() {
	store := storage.NewMemStorage()
	_ = store.UpdateGauge(context.Background(), "Alloc", 13.5)
	router := handler.New(store, nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/value/gauge/Alloc", nil))
	fmt.Println(rec.Code, rec.Body.String())

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/value/gauge/Unknown", nil))
	fmt.Println(rec.Code)

	// Output:
	// 200 13.5
	// 404
}

func ExampleMetricsHandler_UpdateJSON() {
	router := handler.New(storage.NewMemStorage(), nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	for _, body := range []string{
		`{"id":"PollCount","type":"counter","delta":5}`,
		`{"id":"PollCount","type":"counter","delta":3}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/update/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		fmt.Print(rec.Body.String())
	}

	// Output:
	// {"id":"PollCount","type":"counter","delta":5}
	// {"id":"PollCount","type":"counter","delta":8}
}

func ExampleMetricsHandler_UpdatesJSON() {
	router := handler.New(storage.NewMemStorage(), nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	batch := `[{"id":"Alloc","type":"gauge","value":13.5},
		{"id":"PollCount","type":"counter","delta":2},
		{"id":"PollCount","type":"counter","delta":3}]`
	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(batch))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Println(rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/value/", strings.NewReader(`{"id":"PollCount","type":"counter"}`))
	req.Header.Set("Content-Type", "application/json")

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Print(rec.Body.String())

	// Output:
	// 200
	// {"id":"PollCount","type":"counter","delta":5}
}

func ExampleMetricsHandler_ValueJSON() {
	store := storage.NewMemStorage()
	_ = store.UpdateGauge(context.Background(), "HeapAlloc", 1024)
	router := handler.New(store, nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	req := httptest.NewRequest(http.MethodPost, "/value/", strings.NewReader(`{"id":"HeapAlloc","type":"gauge"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	fmt.Print(rec.Body.String())

	// Output:
	// {"id":"HeapAlloc","type":"gauge","value":1024}
}

func ExampleMetricsHandler_List() {
	store := storage.NewMemStorage()
	_ = store.UpdateGauge(context.Background(), "Alloc", 13.5)
	_ = store.AddCounter(context.Background(), "PollCount", 8)
	router := handler.New(store, nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	fmt.Println(rec.Header().Get("Content-Type"))
	fmt.Println(rec.Body.String())

	// Output:
	// text/html; charset=utf-8
	// <!DOCTYPE html>
	// <html lang="ru">
	// <head><meta charset="utf-8"><title>Метрики</title></head>
	// <body>
	// <table>
	// <tr><td>Alloc</td><td>13.5</td></tr>
	// <tr><td>PollCount</td><td>8</td></tr>
	// </table>
	// </body>
	// </html>
}

func ExampleMetricsHandler_Ping() {
	router := handler.New(storage.NewMemStorage(), nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	fmt.Println(rec.Code)

	// Output:
	// 200
}

func ExampleMetricsHandler_Router() {
	const key = "секрет"
	router := handler.New(storage.NewMemStorage(), nil, nil, zap.NewNop()).Router(zap.NewNop(), key)

	body := []byte(`[{"id":"Alloc","type":"gauge","value":13.5}]`)
	for _, signature := range []string{hash.Sign(body, key), hash.Sign(body, "чужой ключ")} {
		req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(hash.Header, signature)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		fmt.Println(rec.Code, hash.Valid(rec.Body.Bytes(), key, rec.Header().Get(hash.Header)))
	}

	// Output:
	// 200 true
	// 400 true
}
