package service

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/model"
	"github.com/Yuilu1317/wallet-backend/internal/types"
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
	dbTimeout   time.Duration
}

func NewDepositCreditService(transaction CreditTxRunner, dbTimeout time.Duration) (*DepositCreditService, error) {
	if dbTimeout <= 0 {
		return nil, fmt.Errorf("deposit credit db timeout must be positive")
	}
	if transaction == nil {
		return nil, fmt.Errorf("transaction runner is nil")
	}
	return &DepositCreditService{
		transaction: transaction,
		dbTimeout:   dbTimeout,
	}, nil
}

func (s *DepositCreditService) lockNextCreditableDeposit(
	ctx context.Context,
	repos DepositCreditRepositories,
	chainID int64,
) (*model.Deposit, bool, error) {
	creditDepositRepositoryCtx, cancel := s.createRepoCtx(ctx)
	defer cancel()
	deposit, found, err := repos.CreditDepositRepository.LockNextCreditableDeposit(
		creditDepositRepositoryCtx,
		chainID,
	)
	if err != nil {
		if mapped := types.MapDBContextError(ctx, creditDepositRepositoryCtx, err); mapped != nil {
			return nil, false, fmt.Errorf("lock next creditable deposit: %w", mapped)
		}
		return nil, false, fmt.Errorf("lock next creditable deposit: %w", err)
	}
	return deposit, found, nil
}

func (s *DepositCreditService) createDepositCreditLedgerIdempotently(
	ctx context.Context,
	repos DepositCreditRepositories,
	ledger *model.BalanceLedger,
) (bool, error) {
	creditBalanceLedgerRepositoryCtx, cancel := s.createRepoCtx(ctx)
	defer cancel()
	ledgerCreated, err := repos.CreditBalanceLedgerRepository.CreateDepositCreditLedgerIdempotently(
		creditBalanceLedgerRepositoryCtx,
		ledger,
	)
	if err != nil {
		if mapped := types.MapDBContextError(ctx, creditBalanceLedgerRepositoryCtx, err); mapped != nil {
			return false, fmt.Errorf("create deposit credit ledger: %w", mapped)
		}
		return false, fmt.Errorf("create deposit credit ledger: %w", err)
	}
	return ledgerCreated, nil
}

func (s *DepositCreditService) addAvailableBalance(
	ctx context.Context,
	repos DepositCreditRepositories,
	account *model.BalanceAccount,
) error {
	creditBalanceAccountRepositoryCtx, cancel := s.createRepoCtx(ctx)
	defer cancel()
	if err := repos.CreditBalanceAccountRepository.AddAvailableBalance(
		creditBalanceAccountRepositoryCtx,
		account,
	); err != nil {
		if mapped := types.MapDBContextError(ctx, creditBalanceAccountRepositoryCtx, err); mapped != nil {
			return fmt.Errorf("add available balance: %w", mapped)
		}
		return fmt.Errorf("add available balance: %w", err)
	}
	return nil
}

func (s *DepositCreditService) markDepositCredited(
	ctx context.Context,
	repos DepositCreditRepositories,
	depositID int64,
) error {
	creditDepositRepositoryCtx, cancel := s.createRepoCtx(ctx)
	defer cancel()
	if err := repos.CreditDepositRepository.MarkDepositCredited(
		creditDepositRepositoryCtx,
		depositID,
	); err != nil {
		if mapped := types.MapDBContextError(ctx, creditDepositRepositoryCtx, err); mapped != nil {
			return fmt.Errorf("mark deposit credited: %w", mapped)
		}
		return fmt.Errorf("mark deposit credited: %w", err)
	}
	return nil
}

func (s *DepositCreditService) CreditNext(ctx context.Context, chainID int64) (bool, error) {
	if chainID <= 0 {
		return false, fmt.Errorf("chain_id must be positive: %d", chainID)
	}
	var credited bool
	err := s.transaction.WithinTransaction(ctx, func(repos DepositCreditRepositories) error {
		deposit, found, err := s.lockNextCreditableDeposit(ctx, repos, chainID)
		if err != nil {
			return err
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

		ledgerCreated, err := s.createDepositCreditLedgerIdempotently(ctx, repos, ledger)
		if err != nil {
			return err
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

		if err := s.addAvailableBalance(ctx, repos, account); err != nil {
			return err
		}

		if err := s.markDepositCredited(ctx, repos, deposit.ID); err != nil {
			return err
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

func (s *DepositCreditService) createRepoCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeoutCause(ctx, s.dbTimeout, types.ErrDBTimeout)
}
