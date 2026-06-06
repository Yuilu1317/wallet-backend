package repo

import (
	"context"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/model"
)

func newTestDeposit(userID int64, depositAddressID int64, txHash string) *model.Deposit {
	return &model.Deposit{
		UserID:           userID,
		ChainID:          11155111,
		DepositAddressID: depositAddressID,
		TxHash:           txHash,
		BlockNumber:      100,
		BlockHash:        "0xblockhash001",
		FromAddress:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ToAddress:        "0x1111111111111111111111111111111111111111",
		AmountWei:        "1000000000000000000",
		ReceiptStatus:    1,
		Status:           model.DepositStatusConfirming,
	}
}

func TestDepositRepo_CreateConfirmingDepositIdempotently_WithNilDeposit_ReturnsError(t *testing.T) {
	t.Parallel()

	repo := NewDepositRepo(nil)

	created, err := repo.CreateConfirmingDepositIdempotently(context.Background(), nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if created {
		t.Fatal("expected created=false, got true")
	}

	if !strings.Contains(err.Error(), "deposit is required") {
		t.Fatalf("expected deposit is required error, got %q", err.Error())
	}
}

func TestDepositRepo_CreateConfirmingDepositIdempotently_CreatesOnce(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(t, tx, userID, 11155111, "0x1111111111111111111111111111111111111111")

	deposit := newTestDeposit(userID, depositAddressID, "0xtxhash001")
	deposit.Status = ""

	created, err := repo.CreateConfirmingDepositIdempotently(context.Background(), deposit)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !created {
		t.Fatal("expected created=true, got false")
	}

	var status string
	if err := tx.Raw(
		"SELECT status FROM deposits WHERE chain_id = ? AND tx_hash = ?",
		deposit.ChainID,
		deposit.TxHash,
	).Scan(&status).Error; err != nil {
		t.Fatalf("query deposit status: %v", err)
	}

	if status != model.DepositStatusConfirming {
		t.Fatalf("expected status %q, got %q", model.DepositStatusConfirming, status)
	}
}

func TestDepositRepo_CreateConfirmingDepositIdempotently_DuplicateReturnsCreatedFalse(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(t, tx, userID, 11155111, "0x2222222222222222222222222222222222222222")

	first := newTestDeposit(userID, depositAddressID, "0xtxhash002")
	second := newTestDeposit(userID, depositAddressID, "0xtxhash002")

	created, err := repo.CreateConfirmingDepositIdempotently(context.Background(), first)
	if err != nil {
		t.Fatalf("expected nil error on first insert, got %v", err)
	}

	if !created {
		t.Fatal("expected first insert created=true, got false")
	}

	created, err = repo.CreateConfirmingDepositIdempotently(context.Background(), second)
	if err != nil {
		t.Fatalf("expected nil error on duplicate insert, got %v", err)
	}

	if created {
		t.Fatal("expected duplicate insert created=false, got true")
	}

	var count int64
	if err := tx.Raw(
		"SELECT COUNT(*) FROM deposits WHERE chain_id = ? AND tx_hash = ?",
		first.ChainID,
		first.TxHash,
	).Scan(&count).Error; err != nil {
		t.Fatalf("count deposits: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one deposit row, got %d", count)
	}
}
