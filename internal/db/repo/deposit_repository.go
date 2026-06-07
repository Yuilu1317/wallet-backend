package repo

import (
	"context"
	"fmt"

	"github.com/Yuilu1317/wallet-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DepositRepo struct {
	db *gorm.DB
}

func NewDepositRepo(db *gorm.DB) *DepositRepo {
	return &DepositRepo{db: db}
}

// CreateConfirmingDepositIdempotently inserts a confirming deposit once.
// The idempotency key is chain_id + tx_hash. If the deposit already exists,
// it returns created=false and nil error.
func (r *DepositRepo) CreateConfirmingDepositIdempotently(
	ctx context.Context,
	deposit *model.Deposit,
) (bool, error) {
	if deposit == nil {
		return false, fmt.Errorf("deposit is required")
	}

	if deposit.Status == "" {
		deposit.Status = model.DepositStatusConfirming
	}

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "chain_id"},
				{Name: "tx_hash"},
			},
			DoNothing: true,
		}).
		Create(deposit)

	if result.Error != nil {
		if mapped := mapDBError(result.Error); mapped != nil {
			return false, fmt.Errorf("create confirming deposit idempotently: %w", mapped)
		}
		return false, fmt.Errorf("create confirming deposit idempotently: %w", result.Error)
	}

	return result.RowsAffected == 1, nil
}
