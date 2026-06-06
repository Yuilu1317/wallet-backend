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

// FindActiveByChainIDAndAddressLower finds an active deposit address by chain ID
// and normalized lowercase address.
//
// Returning found=false is not an error. Most chain transactions are unrelated
// to platform deposit addresses, so the scanner should skip them when no active
// address is found.
func (r *DepositAddressRepo) FindActiveByChainIDAndAddressLower(
	ctx context.Context,
	chainID int64,
	addressLower string,
) (*model.DepositAddress, bool, error) {
	var address model.DepositAddress

	if err := r.db.WithContext(ctx).
		Where("chain_id = ? ", chainID).
		Where("address_lower = ?", addressLower).
		Where("status = ?", model.DepositAddressStatusActive).
		Take(&address).
		Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("find active deposit address: %w", err)
	}
	return &address, true, nil
}
