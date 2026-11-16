// Package compress сжимает данные в gzip и переиспользует компрессоры между вызовами.
package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
)

var writers = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

// GetWriter берёт из пула gzip.Writer, который пишет в w, после Close его возвращают через PutWriter.
func GetWriter(w io.Writer) *gzip.Writer {
	zw := writers.Get().(*gzip.Writer)
	zw.Reset(w)
	return zw
}

// PutWriter возвращает zw в пул.
func PutWriter(zw *gzip.Writer) {
	zw.Reset(io.Discard)
	writers.Put(zw)
}

// Gzip возвращает data, сжатые в gzip.
func Gzip(data []byte) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	zw := GetWriter(&buf)
	defer PutWriter(zw)

	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	return &buf, nil
}
