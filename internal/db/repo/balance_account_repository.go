package repo

import (
	"context"
	"fmt"
	"math/big"

	"github.com/Yuilu1317/wallet-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BalanceAccountRepository struct {
	db *gorm.DB
}

func NewBalanceAccountRepository(db *gorm.DB) *BalanceAccountRepository {
	return &BalanceAccountRepository{db: db}
}

func (r *BalanceAccountRepository) AddAvailableBalance(
	ctx context.Context,
	account *model.BalanceAccount,
) error {
	if err := validateAddAvailableBalance(account); err != nil {
		return err
	}

	amountWei := account.AvailableBalance

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"},
				{Name: "chain_id"},
				{Name: "asset_symbol"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"available_balance": gorm.Expr("available_balance + CAST(? AS NUMERIC)", amountWei),
				"updated_at":        gorm.Expr("now()"),
			}),
		}).
		Create(account)

	if result.Error != nil {
		if mapped := mapDBError(result.Error); mapped != nil {
			return fmt.Errorf("add available balance: %w", mapped)
		}
		return fmt.Errorf("add available balance: %w", result.Error)
	}

	if result.RowsAffected != 1 {
		return fmt.Errorf("add available balance affected %d rows", result.RowsAffected)
	}

	return nil

}

func validateAddAvailableBalance(account *model.BalanceAccount) error {
	if account == nil {
		return fmt.Errorf("account is nil")
	}
	if account.ID != 0 {
		return fmt.Errorf("add available balance account id must be zero: %d", account.ID)
	}
	if account.UserID <= 0 {
		return fmt.Errorf("balance account user_id must be positive: %d", account.UserID)
	}
	if account.ChainID <= 0 {
		return fmt.Errorf("balance account chain_id must be positive: %d", account.ChainID)
	}
	if account.AssetSymbol == "" {
		return fmt.Errorf("balance account asset_symbol is empty")
	}

	if account.AssetSymbol != model.AssetSymbolETH {
		return fmt.Errorf("deposit credit account asset_symbol must be ETH: %s", account.AssetSymbol)
	}

	if account.AvailableBalance == "" {
		return fmt.Errorf("balance account available_balance is empty")
	}

	availableBalance, ok := new(big.Int).SetString(account.AvailableBalance, 10)
	if !ok {
		return fmt.Errorf("balance account available_balance has invalid integer format: %s", account.AvailableBalance)
	}
	if availableBalance.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("add available balance amount must be positive: %s", account.AvailableBalance)
	}

	if account.FrozenBalance == "" {
		return fmt.Errorf("balance account frozen_balance is empty")
	}
	if account.FrozenBalance != "0" {
		return fmt.Errorf("add available balance frozen_balance must be 0: %s", account.FrozenBalance)
	}

	return nil
}
