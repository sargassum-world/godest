// Package handling provides utilities for handlers and background workers
package handling

import (
	"context"
	"time"
)

type Worker func() (done bool, err error)

func Repeat(ctx context.Context, interval time.Duration, f Worker) error {
	if interval == 0 {
		return repeatInstantly(ctx, f)
	}
	return repeatDelayed(ctx, interval, f)
}

func RepeatImmediate(ctx context.Context, interval time.Duration, f Worker) error {
	done, err := f()
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	return Repeat(ctx, interval, f)
}

func repeatInstantly(ctx context.Context, f Worker) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := ctx.Err(); err != nil {
				// Context was also canceled and it should have priority
				return err
			}

			done, err := f()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}

func repeatDelayed(ctx context.Context, interval time.Duration, f Worker) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				// Context was also canceled and it should have priority
				return err
			}

			done, err := f()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}
