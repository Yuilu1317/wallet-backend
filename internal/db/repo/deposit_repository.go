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
		return false, fmt.Errorf("create confirming deposit idempotently: %w", result.Error)
	}

	return result.RowsAffected == 1, nil
}
