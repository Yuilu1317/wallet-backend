package repo

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrDBOperationCanceled = errors.New("database operation canceled")
	ErrDBOperationTimeout  = errors.New("database operation timeout")
)

func mapDBError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w", ErrDBOperationCanceled, err)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrDBOperationTimeout, err)
	}

	return nil
}
