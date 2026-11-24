package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gunzip(t *testing.T, data []byte) string {
	t.Helper()

	zr, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	plain, err := io.ReadAll(zr)
	require.NoError(t, err)
	return string(plain)
}

func TestGzipRoundTrip(t *testing.T) {
	for _, body := range []string{"", `[{"id":"Alloc","type":"gauge","value":13.5}]`, strings.Repeat("PollCount", 10000)} {
		compressed, err := Gzip([]byte(body))
		require.NoError(t, err)
		assert.Equal(t, body, gunzip(t, compressed.Bytes()))
	}
}

func TestWriterFromPoolStartsClean(t *testing.T) {
	var first, second bytes.Buffer

	zw := GetWriter(&first)
	_, err := zw.Write([]byte("первый"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	PutWriter(zw)

	zw = GetWriter(&second)
	_, err = zw.Write([]byte("второй"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	PutWriter(zw)

	assert.Equal(t, "первый", gunzip(t, first.Bytes()))
	assert.Equal(t, "второй", gunzip(t, second.Bytes()), "компрессор из пула не должен тащить данные прошлого потока")
}
