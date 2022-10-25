package handling

import (
	"github.com/pkg/errors"
)

func Except(err error, exceptions ...error) error {
	for _, exception := range exceptions {
		if errors.Is(err, exception) {
			return nil
		}
	}
	return err
}
