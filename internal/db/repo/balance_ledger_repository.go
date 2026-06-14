package repo

import (
	"context"
	"fmt"
	"math/big"

	"github.com/Yuilu1317/wallet-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BalanceLedgerRepository struct {
	db *gorm.DB
}

func NewBalanceLedgerRepository(db *gorm.DB) *BalanceLedgerRepository {
	return &BalanceLedgerRepository{db: db}
}

func (r *BalanceLedgerRepository) CreateDepositCreditLedgerIdempotently(
	ctx context.Context,
	ledger *model.BalanceLedger) (bool, error) {
	if err := validateCreateDepositCreditLedger(ledger); err != nil {
		return false, err
	}

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "source_type"},
				{Name: "source_id"},
			},
			DoNothing: true,
		}).
		Create(ledger)
	if result.Error != nil {
		if mapped := mapDBError(result.Error); mapped != nil {
			return false, fmt.Errorf("create deposit credit ledger idempotently: %w", mapped)
		}
		return false, fmt.Errorf("create deposit credit ledger idempotently: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return true, nil
	}

	if result.RowsAffected == 0 {
		return false, nil
	}

	return false, fmt.Errorf("create deposit credit ledger idempotently: unexpected rows affected: %d", result.RowsAffected)
}

func validateBalanceLedgerCommon(ledger *model.BalanceLedger) error {
	if ledger == nil {
		return fmt.Errorf("ledger is nil")
	}
	if ledger.UserID <= 0 {
		return fmt.Errorf("balance ledger user_id must be positive: %d", ledger.UserID)
	}
	if ledger.ChainID <= 0 {
		return fmt.Errorf("balance ledger chain_id must be positive: %d", ledger.ChainID)
	}
	if ledger.AssetSymbol == "" {
		return fmt.Errorf("balance ledger asset_symbol is empty")
	}
	if ledger.AmountWei == "" {
		return fmt.Errorf("balance ledger amount_wei is empty")
	}

	amountWei, ok := new(big.Int).SetString(ledger.AmountWei, 10)
	if !ok {
		return fmt.Errorf("balance ledger amount_wei has invalid integer format: %s", ledger.AmountWei)
	}
	if amountWei.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("balance ledger amount_wei must be positive: %s", ledger.AmountWei)
	}

	if ledger.Direction != model.LedgerDirectionCredit && ledger.Direction != model.LedgerDirectionDebit {
		return fmt.Errorf("balance ledger direction is invalid: %s", ledger.Direction)
	}

	if ledger.Reason == "" {
		return fmt.Errorf("balance ledger reason is empty")
	}
	if ledger.SourceType == "" {
		return fmt.Errorf("balance ledger source_type is empty")
	}
	if ledger.SourceID <= 0 {
		return fmt.Errorf("balance ledger source_id must be positive: %d", ledger.SourceID)
	}

	return nil
}

func validateCreateDepositCreditLedger(ledger *model.BalanceLedger) error {
	if err := validateBalanceLedgerCommon(ledger); err != nil {
		return err
	}

	if ledger.Direction != model.LedgerDirectionCredit {
		return fmt.Errorf("deposit credit ledger direction must be credit: %s", ledger.Direction)
	}
	if ledger.Reason != model.LedgerReasonDepositCredit {
		return fmt.Errorf("deposit credit ledger reason must be %s: %s", model.LedgerReasonDepositCredit, ledger.Reason)
	}
	if ledger.SourceType != model.LedgerSourceTypeDeposit {
		return fmt.Errorf("deposit credit ledger source_type must be deposit: %s", ledger.SourceType)
	}
	if ledger.AssetSymbol != model.AssetSymbolETH {
		return fmt.Errorf("deposit credit ledger asset_symbol must be ETH: %s", ledger.AssetSymbol)
	}

	return nil
}
