package retry

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type Retrier struct {
	log    *zap.Logger
	delays []time.Duration
}

func New(log *zap.Logger, delays ...time.Duration) *Retrier {
	if len(delays) == 0 {
		delays = []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}
	}
	return &Retrier{log: log, delays: delays}
}

func (r *Retrier) Do(ctx context.Context, retriable func(error) bool, op func() error) error {
	err := op()

	for i, delay := range r.delays {
		if err == nil || !retriable(err) {
			return err
		}

		r.log.Warn("повторная попытка", zap.Int("attempt", i+1), zap.Duration("delay", delay), zap.Error(err))

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		err = op()
	}

	return err
}
