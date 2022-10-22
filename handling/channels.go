package handling

import (
	"context"
)

func Consume[T any](
	ctx context.Context, ch <-chan T, f func(elem T) (done bool, err error),
) error {
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