package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func always(error) bool { return true }

func never(error) bool { return false }

func TestDo(t *testing.T) {
	boom := errors.New("сбой")
	last := errors.New("последний сбой")

	cases := []struct {
		name      string
		retriable func(error) bool
		results   []error
		calls     int
		want      error
	}{
		{
			name:      "успех с первой попытки",
			retriable: always,
			results:   []error{nil},
			calls:     1,
		},
		{
			name:      "повторяемая ошибка на всех попытках",
			retriable: always,
			results:   []error{boom, boom, boom, last},
			calls:     4,
			want:      last,
		},
		{
			name:      "ошибка без повторов",
			retriable: never,
			results:   []error{boom},
			calls:     1,
			want:      boom,
		},
		{
			name:      "успех со второй попытки",
			retriable: always,
			results:   []error{boom, nil},
			calls:     2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := New(zap.NewNop(), time.Microsecond, time.Microsecond, time.Microsecond)

			calls := 0
			err := r.Do(t.Context(), c.retriable, func() error {
				result := c.results[calls]
				calls++
				return result
			})

			assert.Equal(t, c.calls, calls)
			if c.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, c.want)
		})
	}
}

func TestDoStopsOnCanceledContext(t *testing.T) {
	r := New(zap.NewNop(), time.Second, time.Second, time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	time.AfterFunc(20*time.Millisecond, cancel)

	calls := 0
	start := time.Now()

	err := r.Do(ctx, always, func() error {
		calls++
		return errors.New("сбой")
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls, "после отмены контекста операция не повторяется")
	assert.Less(t, time.Since(start), time.Second, "пауза должна прерваться отменой")
}
