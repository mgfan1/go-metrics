package pool

import (
	"bytes"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type counter struct {
	n      int
	resets int
}

func (c *counter) Reset() {
	c.n = 0
	c.resets++
}

func TestGetCreatesObjectWhenPoolIsEmpty(t *testing.T) {
	created := 0
	p := New(func() *counter {
		created++
		return &counter{}
	})

	c := p.Get()

	require.NotNil(t, c)
	assert.Equal(t, 1, created)
}

func TestPutResetsObject(t *testing.T) {
	p := New(func() *counter { return &counter{} })

	c := p.Get()
	c.n = 42
	p.Put(c)

	assert.Zero(t, c.n)
	assert.Equal(t, 1, c.resets)
}

func TestConcurrentGetPut(t *testing.T) {
	p := New(func() *bytes.Buffer { return new(bytes.Buffer) })

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			buf := p.Get()
			defer p.Put(buf)

			buf.WriteString(strconv.Itoa(i))
			assert.Equal(t, strconv.Itoa(i), buf.String())
		})
	}
	wg.Wait()
}
