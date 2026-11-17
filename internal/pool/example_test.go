package pool_test

import (
	"bytes"
	"fmt"

	"github.com/mgfan1/go-metrics/internal/pool"
)

func ExamplePool() {
	buffers := pool.New(func() *bytes.Buffer { return new(bytes.Buffer) })

	buf := buffers.Get()
	buf.WriteString("тело ответа")
	fmt.Println(buf.String())
	buffers.Put(buf)

	next := buffers.Get()
	fmt.Println(next.Len())

	// Output:
	// тело ответа
	// 0
}
