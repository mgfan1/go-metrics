package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/hash"
)

const body = `[{"id":"Alloc","type":"gauge","value":13.5}]`

func echo(called *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*called = true

		got, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "не прочитал тело запроса", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(got)
	}
}

func serve(key, signature string) (*httptest.ResponseRecorder, bool) {
	called := false

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	if signature != "" {
		req.Header.Set(hash.Header, signature)
	}

	rec := httptest.NewRecorder()
	Hash(key, zap.NewNop())(echo(&called)).ServeHTTP(rec, req)

	return rec, called
}

func TestHashWithoutKeyPassesEverything(t *testing.T) {
	rec, called := serve("", hash.Sign([]byte("совсем другое"), "чужой"))

	assert.True(t, called, "без ключа middleware не должен мешать")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, body, rec.Body.String())
	assert.Empty(t, rec.Header().Get(hash.Header), "без ключа ответ не подписывается")
}

func TestHashAcceptsValidSignature(t *testing.T) {
	rec, called := serve("ключ", hash.Sign([]byte(body), "ключ"))

	require.True(t, called, "запрос с верной подписью должен дойти до хендлера")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, body, rec.Body.String(), "тело запроса должно достаться хендлеру целиком")
}

func TestHashAcceptsRequestWithoutSignature(t *testing.T) {
	rec, called := serve("ключ", "")

	assert.True(t, called, "запрос без заголовка подписи отбрасывать нельзя")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHashRejectsBadSignature(t *testing.T) {
	cases := []struct {
		name      string
		signature string
	}{
		{"подписано другим ключом", hash.Sign([]byte(body), "чужой")},
		{"подписано другое тело", hash.Sign([]byte("подмена"), "ключ")},
		{"мусор вместо hex", "не подпись"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, called := serve("ключ", c.signature)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.False(t, called, "данные с неверной подписью должны отбрасываться")
		})
	}
}

func TestHashSignsResponse(t *testing.T) {
	rec, _ := serve("ключ", hash.Sign([]byte(body), "ключ"))

	signature := rec.Header().Get(hash.Header)
	require.NotEmpty(t, signature, "ответ не подписан")
	assert.True(t, hash.Valid(rec.Body.Bytes(), "ключ", signature))
}

func TestHashSignsEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	Hash("ключ", zap.NewNop())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, hash.Sign(nil, "ключ"), rec.Header().Get(hash.Header), "ответ без тела тоже подписывается")
}

func TestHashSignsOwnBadRequest(t *testing.T) {
	rec, called := serve("ключ", hash.Sign([]byte("подмена"), "ключ"))

	require.False(t, called)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	signature := rec.Header().Get(hash.Header)
	require.NotEmpty(t, signature, "ответ middleware не подписан")
	assert.True(t, hash.Valid(rec.Body.Bytes(), "ключ", signature))
}

func TestHashKeepsFirstStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/value/gauge/Alloc", nil)
	rec := httptest.NewRecorder()

	Hash("ключ", zap.NewNop())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
