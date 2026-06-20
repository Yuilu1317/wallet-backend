package worker

import (
	"context"
	"fmt"
	"log"
)

type CreditDepositService interface {
	CreditNext(ctx context.Context, chainID int64) (bool, error)
}

type CreditWorker struct {
	chainID              int64
	creditDepositService CreditDepositService
}

func NewCreditWorker(
	chainID int64,
	creditDepositService CreditDepositService,
) (*CreditWorker, error) {
	if chainID <= 0 {
		return nil, fmt.Errorf("chain_id must be positive: %d", chainID)
	}
	if creditDepositService == nil {
		return nil, fmt.Errorf("credit deposit service is required")
	}

	return &CreditWorker{
		chainID:              chainID,
		creditDepositService: creditDepositService,
	}, nil
}

func (w *CreditWorker) RunOnce(ctx context.Context) error {
	credited, err := w.creditDepositService.CreditNext(ctx, w.chainID)
	if err != nil {
		return fmt.Errorf("run credit deposit once: %w", err)
	}

	if !credited {
		log.Printf("no creditable deposit found: chain_id=%d", w.chainID)
	}

	return nil
}
