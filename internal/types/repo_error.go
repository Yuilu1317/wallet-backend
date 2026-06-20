package types

import (
	"context"
	"errors"
	"fmt"
)

func MapDBContextError(parentCtx context.Context, dbCtx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return nil
	}
	if dbCtx.Err() != nil {
		dbCause := context.Cause(dbCtx)
		if parentCtx.Err() != nil {
			parentCause := context.Cause(parentCtx)
			if errors.Is(dbCause, parentCause) {
				return fmt.Errorf("db aborted by parent context: %w", parentCause)
			}
		}
		return fmt.Errorf("db context failed: %w", dbCause)
	}
	if parentCtx.Err() != nil {
		return fmt.Errorf("db aborted by parent context: %w", context.Cause(parentCtx))
	}
	return fmt.Errorf("db context error: %w", err)
}
