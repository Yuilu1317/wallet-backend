package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/model"
)

func TestDepositCreditService_CreditNext_WithInvalidChainID_ReturnsError(t *testing.T) {
	svc, err := NewDepositCreditService(&fakeTransactionRunner{}, time.Second)

	credited, err := svc.CreditNext(context.Background(), 0)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if credited {
		t.Fatal("expected credited=false, got true")
	}

	if !strings.Contains(err.Error(), "chain_id must be positive") {
		t.Fatalf("expected chain_id error, got %q", err.Error())
	}
}

func TestNewDepositCreditService_WithNilTransactionRunner_ReturnsError(t *testing.T) {
	svc, err := NewDepositCreditService(nil, time.Second)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if svc != nil {
		t.Fatal("expected service to be nil")
	}
}

func TestDepositCreditService_CreditNext_WhenNoCreditableDeposit_ReturnsFalse(t *testing.T) {
	calls := []string{}

	tx := &fakeTransactionRunner{
		repos: DepositCreditRepositories{
			CreditDepositRepository: &fakeCreditDepositRepository{
				calls: callsPtr(&calls),
				found: false,
			},
			CreditBalanceLedgerRepository: &fakeCreditBalanceLedgerRepository{
				calls: callsPtr(&calls),
			},
			CreditBalanceAccountRepository: &fakeCreditBalanceAccountRepository{
				calls: callsPtr(&calls),
			},
		},
	}

	svc, err := NewDepositCreditService(tx, time.Second)

	credited, err := svc.CreditNext(context.Background(), 11155111)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if credited {
		t.Fatal("expected credited=false, got true")
	}

	wantCalls := []string{
		"LockNextCreditableDeposit",
	}

	assertCalls(t, calls, wantCalls)
}

func TestDepositCreditService_CreditNext_Success(t *testing.T) {
	calls := []string{}

	deposit := newValidCreditableDeposit()

	depositRepo := &fakeCreditDepositRepository{
		calls:   callsPtr(&calls),
		deposit: deposit,
		found:   true,
	}

	ledgerRepo := &fakeCreditBalanceLedgerRepository{
		calls:         callsPtr(&calls),
		ledgerCreated: true,
	}

	accountRepo := &fakeCreditBalanceAccountRepository{
		calls: callsPtr(&calls),
	}

	tx := &fakeTransactionRunner{
		repos: DepositCreditRepositories{
			CreditDepositRepository:        depositRepo,
			CreditBalanceLedgerRepository:  ledgerRepo,
			CreditBalanceAccountRepository: accountRepo,
		},
	}

	svc, err := NewDepositCreditService(tx, time.Second)

	credited, err := svc.CreditNext(context.Background(), 11155111)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !credited {
		t.Fatal("expected credited=true, got false")
	}

	wantCalls := []string{
		"LockNextCreditableDeposit",
		"CreateDepositCreditLedgerIdempotently",
		"AddAvailableBalance",
		"MarkDepositCredited",
	}

	assertCalls(t, calls, wantCalls)

	if ledgerRepo.gotLedger == nil {
		t.Fatal("expected ledger to be created, got nil")
	}

	if ledgerRepo.gotLedger.UserID != deposit.UserID {
		t.Fatalf("expected ledger user_id=%d, got %d", deposit.UserID, ledgerRepo.gotLedger.UserID)
	}

	if ledgerRepo.gotLedger.ChainID != deposit.ChainID {
		t.Fatalf("expected ledger chain_id=%d, got %d", deposit.ChainID, ledgerRepo.gotLedger.ChainID)
	}

	if ledgerRepo.gotLedger.AssetSymbol != model.AssetSymbolETH {
		t.Fatalf("expected ledger asset_symbol=%s, got %s", model.AssetSymbolETH, ledgerRepo.gotLedger.AssetSymbol)
	}

	if ledgerRepo.gotLedger.AmountWei != deposit.AmountWei {
		t.Fatalf("expected ledger amount_wei=%s, got %s", deposit.AmountWei, ledgerRepo.gotLedger.AmountWei)
	}

	if ledgerRepo.gotLedger.Direction != model.LedgerDirectionCredit {
		t.Fatalf("expected ledger direction=%s, got %s", model.LedgerDirectionCredit, ledgerRepo.gotLedger.Direction)
	}

	if ledgerRepo.gotLedger.Reason != model.LedgerReasonDepositCredit {
		t.Fatalf("expected ledger reason=%s, got %s", model.LedgerReasonDepositCredit, ledgerRepo.gotLedger.Reason)
	}

	if ledgerRepo.gotLedger.SourceType != model.LedgerSourceTypeDeposit {
		t.Fatalf("expected ledger source_type=%s, got %s", model.LedgerSourceTypeDeposit, ledgerRepo.gotLedger.SourceType)
	}

	if ledgerRepo.gotLedger.SourceID != deposit.ID {
		t.Fatalf("expected ledger source_id=%d, got %d", deposit.ID, ledgerRepo.gotLedger.SourceID)
	}

	if accountRepo.gotAccount == nil {
		t.Fatal("expected account to be added, got nil")
	}

	if accountRepo.gotAccount.UserID != deposit.UserID {
		t.Fatalf("expected account user_id=%d, got %d", deposit.UserID, accountRepo.gotAccount.UserID)
	}

	if accountRepo.gotAccount.ChainID != deposit.ChainID {
		t.Fatalf("expected account chain_id=%d, got %d", deposit.ChainID, accountRepo.gotAccount.ChainID)
	}

	if accountRepo.gotAccount.AssetSymbol != model.AssetSymbolETH {
		t.Fatalf("expected account asset_symbol=%s, got %s", model.AssetSymbolETH, accountRepo.gotAccount.AssetSymbol)
	}

	if accountRepo.gotAccount.AvailableBalance != deposit.AmountWei {
		t.Fatalf("expected account available_balance=%s, got %s", deposit.AmountWei, accountRepo.gotAccount.AvailableBalance)
	}

	if accountRepo.gotAccount.FrozenBalance != "0" {
		t.Fatalf("expected account frozen_balance=0, got %s", accountRepo.gotAccount.FrozenBalance)
	}

	if depositRepo.markedDepositID != deposit.ID {
		t.Fatalf("expected marked deposit id=%d, got %d", deposit.ID, depositRepo.markedDepositID)
	}
}

func TestDepositCreditService_CreditNext_WhenLedgerAlreadyExists_ReturnsErrorAndDoesNotAddBalance(t *testing.T) {
	calls := []string{}

	deposit := newValidCreditableDeposit()

	tx := &fakeTransactionRunner{
		repos: DepositCreditRepositories{
			CreditDepositRepository: &fakeCreditDepositRepository{
				calls:   callsPtr(&calls),
				deposit: deposit,
				found:   true,
			},
			CreditBalanceLedgerRepository: &fakeCreditBalanceLedgerRepository{
				calls:         callsPtr(&calls),
				ledgerCreated: false,
			},
			CreditBalanceAccountRepository: &fakeCreditBalanceAccountRepository{
				calls: callsPtr(&calls),
			},
		},
	}

	svc, err := NewDepositCreditService(tx, time.Second)

	credited, err := svc.CreditNext(context.Background(), 11155111)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if credited {
		t.Fatal("expected credited=false, got true")
	}

	if !strings.Contains(err.Error(), "deposit credit ledger already exists") {
		t.Fatalf("expected ledger already exists error, got %q", err.Error())
	}

	wantCalls := []string{
		"LockNextCreditableDeposit",
		"CreateDepositCreditLedgerIdempotently",
	}

	assertCalls(t, calls, wantCalls)

	if !tx.rolledBack {
		t.Fatal("expected transaction rollback flag to be true")
	}
}

func TestDepositCreditService_CreditNext_WhenAddAvailableBalanceFails_DoesNotMarkDepositCredited(t *testing.T) {
	calls := []string{}

	deposit := newValidCreditableDeposit()

	tx := &fakeTransactionRunner{
		repos: DepositCreditRepositories{
			CreditDepositRepository: &fakeCreditDepositRepository{
				calls:   callsPtr(&calls),
				deposit: deposit,
				found:   true,
			},
			CreditBalanceLedgerRepository: &fakeCreditBalanceLedgerRepository{
				calls:         callsPtr(&calls),
				ledgerCreated: true,
			},
			CreditBalanceAccountRepository: &fakeCreditBalanceAccountRepository{
				calls: callsPtr(&calls),
				err:   errors.New("balance account write failed"),
			},
		},
	}

	svc, err := NewDepositCreditService(tx, time.Second)

	credited, err := svc.CreditNext(context.Background(), 11155111)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if credited {
		t.Fatal("expected credited=false, got true")
	}

	if !strings.Contains(err.Error(), "add available balance") {
		t.Fatalf("expected add available balance error, got %q", err.Error())
	}

	wantCalls := []string{
		"LockNextCreditableDeposit",
		"CreateDepositCreditLedgerIdempotently",
		"AddAvailableBalance",
	}

	assertCalls(t, calls, wantCalls)

	if !tx.rolledBack {
		t.Fatal("expected transaction rollback flag to be true")
	}
}

func TestDepositCreditService_CreditNext_WhenMarkDepositCreditedFails_ReturnsError(t *testing.T) {
	calls := []string{}

	deposit := newValidCreditableDeposit()

	tx := &fakeTransactionRunner{
		repos: DepositCreditRepositories{
			CreditDepositRepository: &fakeCreditDepositRepository{
				calls:   callsPtr(&calls),
				deposit: deposit,
				found:   true,
				markErr: errors.New("mark deposit failed"),
			},
			CreditBalanceLedgerRepository: &fakeCreditBalanceLedgerRepository{
				calls:         callsPtr(&calls),
				ledgerCreated: true,
			},
			CreditBalanceAccountRepository: &fakeCreditBalanceAccountRepository{
				calls: callsPtr(&calls),
			},
		},
	}

	svc, err := NewDepositCreditService(tx, time.Second)

	credited, err := svc.CreditNext(context.Background(), 11155111)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if credited {
		t.Fatal("expected credited=false, got true")
	}

	if !strings.Contains(err.Error(), "mark deposit credited") {
		t.Fatalf("expected mark deposit credited error, got %q", err.Error())
	}

	wantCalls := []string{
		"LockNextCreditableDeposit",
		"CreateDepositCreditLedgerIdempotently",
		"AddAvailableBalance",
		"MarkDepositCredited",
	}

	assertCalls(t, calls, wantCalls)

	if !tx.rolledBack {
		t.Fatal("expected transaction rollback flag to be true")
	}
}

func TestValidateCreditableDeposit_WithInvalidDeposit_ReturnsError(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		chainID int64
		deposit *model.Deposit
		wantErr string
	}{
		{
			name:    "nil deposit",
			chainID: 11155111,
			deposit: nil,
			wantErr: "deposit is nil",
		},
		{
			name:    "deposit id is zero",
			chainID: 11155111,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.ID = 0
				return deposit
			}(),
			wantErr: "deposit id must be positive",
		},
		{
			name:    "user id is zero",
			chainID: 11155111,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.UserID = 0
				return deposit
			}(),
			wantErr: "deposit user_id must be positive",
		},
		{
			name:    "chain id does not match",
			chainID: 1,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.ChainID = 11155111
				return deposit
			}(),
			wantErr: "deposit chain_id does not match",
		},
		{
			name:    "deposit address id is zero",
			chainID: 11155111,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.DepositAddressID = 0
				return deposit
			}(),
			wantErr: "deposit deposit_address_id must be positive",
		},
		{
			name:    "status is not confirming",
			chainID: 11155111,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.Status = model.DepositStatusCredited
				return deposit
			}(),
			wantErr: "deposit status must be confirming",
		},
		{
			name:    "credited_at is not nil",
			chainID: 11155111,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.CreditedAt = &now
				return deposit
			}(),
			wantErr: "deposit credited_at must be nil",
		},
		{
			name:    "amount wei is invalid",
			chainID: 11155111,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.AmountWei = "abc"
				return deposit
			}(),
			wantErr: "deposit amount_wei has invalid integer format",
		},
		{
			name:    "amount wei is zero",
			chainID: 11155111,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.AmountWei = "0"
				return deposit
			}(),
			wantErr: "deposit amount_wei must be positive",
		},
		{
			name:    "receipt status is failed",
			chainID: 11155111,
			deposit: func() *model.Deposit {
				deposit := newValidCreditableDeposit()
				deposit.ReceiptStatus = 0
				return deposit
			}(),
			wantErr: "deposit receipt_status must be 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreditableDeposit(tt.chainID, tt.deposit)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestValidateCreditableDeposit_WithValidDeposit_ReturnsNil(t *testing.T) {
	deposit := newValidCreditableDeposit()

	err := validateCreditableDeposit(11155111, deposit)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func newValidCreditableDeposit() *model.Deposit {
	return &model.Deposit{
		ID:               1001,
		UserID:           1,
		ChainID:          11155111,
		DepositAddressID: 10,
		TxHash:           "0xdeposit001",
		BlockNumber:      100,
		BlockHash:        "0xblockhash001",
		FromAddress:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ToAddress:        "0x1111111111111111111111111111111111111111",
		AmountWei:        "1000000000000000000",
		ReceiptStatus:    1,
		Status:           model.DepositStatusConfirming,
		CreditedAt:       nil,
	}
}

type fakeTransactionRunner struct {
	repos      DepositCreditRepositories
	committed  bool
	rolledBack bool
}

func (r *fakeTransactionRunner) WithinTransaction(
	ctx context.Context,
	fn func(repos DepositCreditRepositories) error,
) error {
	err := fn(r.repos)
	if err != nil {
		r.rolledBack = true
		return err
	}

	r.committed = true
	return nil
}

type fakeCreditDepositRepository struct {
	calls *[]string

	deposit *model.Deposit
	found   bool
	lockErr error

	markedDepositID int64
	markErr         error
}

func (r *fakeCreditDepositRepository) LockNextCreditableDeposit(
	ctx context.Context,
	chainID int64,
) (*model.Deposit, bool, error) {
	appendCall(r.calls, "LockNextCreditableDeposit")

	if r.lockErr != nil {
		return nil, false, r.lockErr
	}

	return r.deposit, r.found, nil
}

func (r *fakeCreditDepositRepository) MarkDepositCredited(
	ctx context.Context,
	depositID int64,
) error {
	appendCall(r.calls, "MarkDepositCredited")

	if r.markErr != nil {
		return r.markErr
	}

	r.markedDepositID = depositID
	return nil
}

type fakeCreditBalanceLedgerRepository struct {
	calls *[]string

	ledgerCreated bool
	err           error
	gotLedger     *model.BalanceLedger
}

func (r *fakeCreditBalanceLedgerRepository) CreateDepositCreditLedgerIdempotently(
	ctx context.Context,
	ledger *model.BalanceLedger,
) (bool, error) {
	appendCall(r.calls, "CreateDepositCreditLedgerIdempotently")

	if r.err != nil {
		return false, r.err
	}

	r.gotLedger = ledger
	return r.ledgerCreated, nil
}

type fakeCreditBalanceAccountRepository struct {
	calls *[]string

	err        error
	gotAccount *model.BalanceAccount
}

func (r *fakeCreditBalanceAccountRepository) AddAvailableBalance(
	ctx context.Context,
	account *model.BalanceAccount,
) error {
	appendCall(r.calls, "AddAvailableBalance")

	if r.err != nil {
		return r.err
	}

	r.gotAccount = account
	return nil
}

func callsPtr(calls *[]string) *[]string {
	return calls
}

func appendCall(calls *[]string, call string) {
	if calls == nil {
		return
	}

	*calls = append(*calls, call)
}

func assertCalls(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected calls %v, got %v", want, got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected calls %v, got %v", want, got)
		}
	}
}
