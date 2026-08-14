package retry

import (
	"context"
	"time"

	"go.uber.org/zap"
)

var delays = []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}

func Do(ctx context.Context, log *zap.Logger, retriable func(error) bool, op func() error) error {
	err := op()

	for i, delay := range delays {
		if err == nil || !retriable(err) {
			return err
		}

		log.Warn("повторная попытка", zap.Int("attempt", i+1), zap.Duration("delay", delay), zap.Error(err))

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		err = op()
	}

	return err
}
