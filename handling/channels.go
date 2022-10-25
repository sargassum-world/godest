package handling

import (
	"context"
)

type Consumer[T any] func(elem T) (done bool, err error)

func Consume[T any](ctx context.Context, ch <-chan T, f Consumer[T]) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case elem, ok := <-ch:
			if err := ctx.Err(); err != nil {
				// Context was also canceled and it should have priority
				return err
			}
			if !ok {
				return nil
			}

			done, err := f(elem)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}
