package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/Yuilu1317/wallet-backend/internal/model"
	"gorm.io/gorm"
)

type DepositAddressRepo struct {
	db *gorm.DB
}

func NewDepositAddressRepo(db *gorm.DB) *DepositAddressRepo {
	return &DepositAddressRepo{db: db}
}

func (r *DepositAddressRepo) FindActiveByChainIDAndAddressLower(
	ctx context.Context,
	chainID int64,
	addressLower string,
) (*model.DepositAddress, bool, error) {
	if chainID <= 0 {
		return nil, false, fmt.Errorf("chain_id must be positive")
	}
	if addressLower == "" {
		return nil, false, fmt.Errorf("lower-case address must not be empty")
	}

	var depositAddress model.DepositAddress
	if err := r.db.WithContext(ctx).
		Where("chain_id = ? AND address_lower = ? AND status = ?", chainID, addressLower, model.DepositAddressStatusActive).
		Take(&depositAddress).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		if mapped := mapDBError(err); mapped != nil {
			return nil, false, fmt.Errorf("find active deposit address: %w", mapped)
		}
		return nil, false, fmt.Errorf("find active deposit address: %w", err)
	}
	return &depositAddress, true, nil
}
