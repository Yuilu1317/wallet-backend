package repo

import (
	"context"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/model"
)

func newTestDepositCreditLedger(userID int64, sourceID int64) *model.BalanceLedger {
	return &model.BalanceLedger{
		UserID:      userID,
		ChainID:     11155111,
		AssetSymbol: model.AssetSymbolETH,
		AmountWei:   "1000000000000000000",
		Direction:   model.LedgerDirectionCredit,
		Reason:      model.LedgerReasonDepositCredit,
		SourceType:  model.LedgerSourceTypeDeposit,
		SourceID:    sourceID,
	}
}

func TestBalanceLedgerRepository_CreateDepositCreditLedgerIdempotently_CreatesOnce(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewBalanceLedgerRepository(tx)

	userID := insertTestUser(t, tx)

	ledger := newTestDepositCreditLedger(userID, 1001)

	created, err := repo.CreateDepositCreditLedgerIdempotently(context.Background(), ledger)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !created {
		t.Fatal("expected created=true, got false")
	}

	var count int64
	if err := tx.Raw(
		"SELECT COUNT(*) FROM balance_ledgers WHERE source_type = ? AND source_id = ?",
		model.LedgerSourceTypeDeposit,
		int64(1001),
	).Scan(&count).Error; err != nil {
		t.Fatalf("count balance ledgers: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one balance ledger row, got %d", count)
	}
}

func TestBalanceLedgerRepository_CreateDepositCreditLedgerIdempotently_DuplicateReturnsCreatedFalse(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewBalanceLedgerRepository(tx)

	userID := insertTestUser(t, tx)

	first := newTestDepositCreditLedger(userID, 1002)
	second := newTestDepositCreditLedger(userID, 1002)
	second.AmountWei = "999999999999999999"

	created, err := repo.CreateDepositCreditLedgerIdempotently(context.Background(), first)
	if err != nil {
		t.Fatalf("expected nil error on first insert, got %v", err)
	}

	if !created {
		t.Fatal("expected first insert created=true, got false")
	}

	created, err = repo.CreateDepositCreditLedgerIdempotently(context.Background(), second)
	if err != nil {
		t.Fatalf("expected nil error on duplicate insert, got %v", err)
	}

	if created {
		t.Fatal("expected duplicate insert created=false, got true")
	}

	var count int64
	if err := tx.Raw(
		"SELECT COUNT(*) FROM balance_ledgers WHERE source_type = ? AND source_id = ?",
		model.LedgerSourceTypeDeposit,
		int64(1002),
	).Scan(&count).Error; err != nil {
		t.Fatalf("count balance ledgers: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one balance ledger row, got %d", count)
	}

	var amountWei string
	if err := tx.Raw(
		"SELECT amount_wei FROM balance_ledgers WHERE source_type = ? AND source_id = ?",
		model.LedgerSourceTypeDeposit,
		int64(1002),
	).Scan(&amountWei).Error; err != nil {
		t.Fatalf("query amount_wei: %v", err)
	}

	if amountWei != "1000000000000000000" {
		t.Fatalf("expected original amount_wei unchanged, got %s", amountWei)
	}
}

func TestBalanceLedgerRepository_CreateDepositCreditLedgerIdempotently_WithNilLedger_ReturnsError(t *testing.T) {
	t.Parallel()

	repo := NewBalanceLedgerRepository(nil)

	created, err := repo.CreateDepositCreditLedgerIdempotently(context.Background(), nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if created {
		t.Fatal("expected created=false, got true")
	}

	if !strings.Contains(err.Error(), "ledger is nil") {
		t.Fatalf("expected ledger is nil error, got %q", err.Error())
	}
}

func TestBalanceLedgerRepository_CreateDepositCreditLedgerIdempotently_WithInvalidAmount_ReturnsError(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewBalanceLedgerRepository(tx)

	userID := insertTestUser(t, tx)

	ledger := newTestDepositCreditLedger(userID, 1003)
	ledger.AmountWei = "0"

	created, err := repo.CreateDepositCreditLedgerIdempotently(context.Background(), ledger)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if created {
		t.Fatal("expected created=false, got true")
	}

	if !strings.Contains(err.Error(), "amount_wei must be positive") {
		t.Fatalf("expected amount_wei error, got %q", err.Error())
	}
}

func TestBalanceLedgerRepository_CreateDepositCreditLedgerIdempotently_WithWrongSourceType_ReturnsError(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewBalanceLedgerRepository(tx)

	userID := insertTestUser(t, tx)

	ledger := newTestDepositCreditLedger(userID, 1004)
	ledger.SourceType = "withdraw"

	created, err := repo.CreateDepositCreditLedgerIdempotently(context.Background(), ledger)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if created {
		t.Fatal("expected created=false, got true")
	}

	if !strings.Contains(err.Error(), "source_type must be deposit") {
		t.Fatalf("expected source_type error, got %q", err.Error())
	}
}
