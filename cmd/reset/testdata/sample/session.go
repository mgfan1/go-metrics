package sample

import (
	"bytes"
	"hash"
	"io"
	"log"
	"time"

	"example.com/sample/inner"
)

// generate:reset
type Session struct {
	Stats
	ID      int64
	Name    string
	Active  bool
	Ratio   float64
	Timeout time.Duration
	Started time.Time
	Tags    []string
	Attrs   map[string]string
	Note    *string
	Limit   *int
	Buf     *bytes.Buffer
	Log     bytes.Buffer
	Logger  *log.Logger
	Child   *Session
	Options Options
	Item    inner.Item
	Digest  checksum
	Window  [4]byte
	Out     io.Writer
	Done    chan struct{}
	OnClose func()
	Meta    struct{ Version int }
	_       int
}

type Stats struct {
	Hits int
}

type checksum struct {
	hash.Hash
	size int
}

// generate:reset
type Options struct {
	Retries int
	Hosts   []string
}

type (
	// generate:reset
	counter struct {
		hits  map[string]int
		total uint64
	}

	Plain struct {
		Value int
	}
)

func (cnt *counter) Add(key string) {
	cnt.hits[key]++
	cnt.total++
}
