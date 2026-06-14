package repo

import (
	"context"
	"errors"
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

	if deposit.Status != model.DepositStatusConfirming {
		return false, fmt.Errorf("deposit status must be confirming")
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
	if result.RowsAffected == 1 {
		return true, nil
	}

	if result.RowsAffected == 0 {
		return false, nil
	}

	return false, fmt.Errorf("create deposit credit ledger idempotently: unexpected rows affected: %d", result.RowsAffected)
}

func (r *DepositRepo) LockNextCreditableDeposit(ctx context.Context, chainID int64) (*model.Deposit, bool, error) {
	if chainID <= 0 {
		return nil, false, fmt.Errorf("chain_id must be positive")
	}

	var deposit model.Deposit
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{
			Strength: "UPDATE",
			Options:  "SKIP LOCKED",
		}).
		Where("chain_id = ?", chainID).
		Where("status = ?", model.DepositStatusConfirming).
		Where("credited_at IS NULL").
		Order("block_number ASC").
		Order("id ASC").
		Limit(1).
		Take(&deposit).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		if mapped := mapDBError(err); mapped != nil {
			return nil, false, mapped
		}
		return nil, false, fmt.Errorf("lock next creditable deposit: %w", err)
	}
	return &deposit, true, nil
}

func (r *DepositRepo) MarkDepositCredited(ctx context.Context, depositID int64) error {
	if depositID <= 0 {
		return fmt.Errorf("deposit_id must be positive")
	}

	result := r.db.WithContext(ctx).
		Model(&model.Deposit{}).
		Where("id = ?", depositID).
		Where("status = ?", model.DepositStatusConfirming).
		Where("credited_at IS NULL").
		Updates(map[string]any{
			"status":      model.DepositStatusCredited,
			"credited_at": gorm.Expr("NOW()"),
			"updated_at":  gorm.Expr("NOW()"),
		})

	if result.Error != nil {
		if mapped := mapDBError(result.Error); mapped != nil {
			return mapped
		}
		return fmt.Errorf("mark deposit credited deposit: %w", result.Error)
	}

	if result.RowsAffected != 1 {
		return fmt.Errorf("mark deposit credited affected %d rows", result.RowsAffected)
	}

	return nil
}
