// Package handling provides utilities for handlers and background workers
package handling

import (
	"context"
	"time"
)

type Worker func() (done bool, err error)

func Repeat(ctx context.Context, interval time.Duration, f func() (done bool, err error)) error {
	if interval == 0 {
		return repeatInstantly(ctx, f)
	}
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

func repeatInstantly(ctx context.Context, f func() (done bool, err error)) error {
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
