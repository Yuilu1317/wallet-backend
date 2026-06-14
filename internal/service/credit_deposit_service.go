package service

import (
	"context"
	"fmt"
	"math/big"

	"github.com/Yuilu1317/wallet-backend/internal/model"
)

// CreditDepositRepository from deposit_repository.go
type CreditDepositRepository interface {
	LockNextCreditableDeposit(ctx context.Context, chainID int64) (*model.Deposit, bool, error)
	MarkDepositCredited(ctx context.Context, depositID int64) error
}

// CreditBalanceLedgerRepository from balance_ledger_repository.go
type CreditBalanceLedgerRepository interface {
	CreateDepositCreditLedgerIdempotently(
		ctx context.Context,
		ledger *model.BalanceLedger) (bool, error)
}

// CreditBalanceAccountRepository from balance_account_repository.go
type CreditBalanceAccountRepository interface {
	AddAvailableBalance(
		ctx context.Context,
		account *model.BalanceAccount,
	) error
}

type DepositCreditRepositories struct {
	CreditDepositRepository        CreditDepositRepository
	CreditBalanceLedgerRepository  CreditBalanceLedgerRepository
	CreditBalanceAccountRepository CreditBalanceAccountRepository
}

type CreditTxRunner interface {
	WithinTransaction(
		ctx context.Context,
		fn func(repos DepositCreditRepositories) error,
	) error
}

type DepositCreditService struct {
	transaction CreditTxRunner
}

func NewDepositCreditService(transaction CreditTxRunner) *DepositCreditService {
	return &DepositCreditService{
		transaction: transaction,
	}
}

func (s *DepositCreditService) CreditNext(ctx context.Context, chainID int64) (bool, error) {
	if chainID <= 0 {
		return false, fmt.Errorf("chain_id must be positive: %d", chainID)
	}
	if s.transaction == nil {
		return false, fmt.Errorf("transaction runner is nil")
	}
	var credited bool
	err := s.transaction.WithinTransaction(ctx, func(repos DepositCreditRepositories) error {
		deposit, found, err := repos.CreditDepositRepository.LockNextCreditableDeposit(ctx, chainID)
		if err != nil {
			return fmt.Errorf("lock next creditable deposit: %w", err)
		}

		if !found {
			credited = false
			return nil
		}

		if err := validateCreditableDeposit(chainID, deposit); err != nil {
			return fmt.Errorf("validate creditable deposit: %w", err)
		}
		ledger := &model.BalanceLedger{
			UserID:      deposit.UserID,
			ChainID:     deposit.ChainID,
			AssetSymbol: model.AssetSymbolETH,
			AmountWei:   deposit.AmountWei,
			Direction:   model.LedgerDirectionCredit,
			Reason:      model.LedgerReasonDepositCredit,
			SourceType:  model.LedgerSourceTypeDeposit,
			SourceID:    deposit.ID,
		}
		ledgerCreated, err := repos.CreditBalanceLedgerRepository.CreateDepositCreditLedgerIdempotently(ctx, ledger)
		if err != nil {
			return fmt.Errorf("create deposit credit ledger: %w", err)
		}
		if !ledgerCreated {
			return fmt.Errorf("deposit credit ledger already exists: deposit_id=%d", deposit.ID)
		}

		account := &model.BalanceAccount{
			UserID:           deposit.UserID,
			ChainID:          deposit.ChainID,
			AssetSymbol:      model.AssetSymbolETH,
			AvailableBalance: deposit.AmountWei,
			FrozenBalance:    "0",
		}
		if err := repos.CreditBalanceAccountRepository.AddAvailableBalance(ctx, account); err != nil {
			return fmt.Errorf("add available balance: %w", err)
		}

		if err := repos.CreditDepositRepository.MarkDepositCredited(ctx, deposit.ID); err != nil {
			return fmt.Errorf("mark deposit credited: %w", err)
		}
		credited = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("credit next deposit: %w", err)
	}
	return credited, nil
}

func validateCreditableDeposit(chainID int64, deposit *model.Deposit) error {
	if deposit == nil {
		return fmt.Errorf("deposit is nil")
	}
	if deposit.ID <= 0 {
		return fmt.Errorf("deposit id must be positive: %d", deposit.ID)
	}
	if deposit.UserID <= 0 {
		return fmt.Errorf("deposit user_id must be positive: %d", deposit.UserID)
	}
	if deposit.ChainID != chainID {
		return fmt.Errorf("deposit chain_id does not match: got %d, want %d", deposit.ChainID, chainID)
	}
	if deposit.DepositAddressID <= 0 {
		return fmt.Errorf("deposit deposit_address_id must be positive: %d", deposit.DepositAddressID)
	}
	if deposit.Status != model.DepositStatusConfirming {
		return fmt.Errorf("deposit status must be confirming: %s", deposit.Status)
	}
	if deposit.CreditedAt != nil {
		return fmt.Errorf("deposit credited_at must be nil")
	}
	amountWei, ok := new(big.Int).SetString(deposit.AmountWei, 10)
	if !ok {
		return fmt.Errorf("deposit amount_wei has invalid integer format: %s", deposit.AmountWei)
	}
	if amountWei.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("deposit amount_wei must be positive: %s", deposit.AmountWei)
	}
	if deposit.ReceiptStatus != 1 {
		return fmt.Errorf("deposit receipt_status must be 1: %d", deposit.ReceiptStatus)
	}
	return nil
}
