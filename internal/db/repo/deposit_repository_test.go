package repo

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestDepositRepo_CreateConfirmingDepositIdempotently_AllowsSameTxHashOnDifferentChain(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)

	depositAddressID1 := insertTestDepositAddress(
		t,
		tx,
		userID,
		11155111,
		"0x3333333333333333333333333333333333333333",
	)

	depositAddressID2 := insertTestDepositAddress(
		t,
		tx,
		userID,
		1,
		"0x3333333333333333333333333333333333333333",
	)

	first := newTestDeposit(userID, depositAddressID1, "0xsametxhash")
	first.ChainID = 11155111

	second := newTestDeposit(userID, depositAddressID2, "0xsametxhash")
	second.ChainID = 1

	created, err := repo.CreateConfirmingDepositIdempotently(context.Background(), first)
	if err != nil {
		t.Fatalf("expected nil error on first insert, got %v", err)
	}
	if !created {
		t.Fatal("expected first insert created=true, got false")
	}

	created, err = repo.CreateConfirmingDepositIdempotently(context.Background(), second)
	if err != nil {
		t.Fatalf("expected nil error on second chain insert, got %v", err)
	}
	if !created {
		t.Fatal("expected second chain insert created=true, got false")
	}

	var count int64
	if err := tx.Raw(
		"SELECT COUNT(*) FROM deposits WHERE tx_hash = ?",
		"0xsametxhash",
	).Scan(&count).Error; err != nil {
		t.Fatalf("count deposits: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected two deposits with same tx_hash on different chains, got %d", count)
	}
}

func TestDepositRepo_LockNextCreditableDeposit_WithInvalidChainID_ReturnsError(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	deposit, found, err := repo.LockNextCreditableDeposit(context.Background(), 0)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if deposit != nil {
		t.Fatalf("expected nil deposit, got %+v", deposit)
	}

	if found {
		t.Fatal("expected found=false, got true")
	}

	if !strings.Contains(err.Error(), "chain_id must be positive") {
		t.Fatalf("expected chain_id error, got %q", err.Error())
	}
}

func TestDepositRepo_LockNextCreditableDeposit_WhenNoCreditableDeposit_ReturnsFalse(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	deposit, found, err := repo.LockNextCreditableDeposit(context.Background(), 11155111)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if found {
		t.Fatal("expected found=false, got true")
	}

	if deposit != nil {
		t.Fatalf("expected nil deposit, got %+v", deposit)
	}
}

func TestDepositRepo_LockNextCreditableDeposit_ReturnsConfirmingUncreditedDeposit(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(
		t,
		tx,
		userID,
		11155111,
		"0x4444444444444444444444444444444444444444",
	)

	expected := newTestDeposit(userID, depositAddressID, "0xcreditable001")
	expected.BlockNumber = 120
	expected.Status = model.DepositStatusConfirming
	expected.CreditedAt = nil

	if err := tx.Create(expected).Error; err != nil {
		t.Fatalf("insert deposit: %v", err)
	}

	got, found, err := repo.LockNextCreditableDeposit(context.Background(), 11155111)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !found {
		t.Fatal("expected found=true, got false")
	}

	if got == nil {
		t.Fatal("expected deposit, got nil")
	}

	if got.ID != expected.ID {
		t.Fatalf("expected deposit id %d, got %d", expected.ID, got.ID)
	}

	if got.UserID != userID {
		t.Fatalf("expected user id %d, got %d", userID, got.UserID)
	}

	if got.ChainID != 11155111 {
		t.Fatalf("expected chain id 11155111, got %d", got.ChainID)
	}

	if got.TxHash != "0xcreditable001" {
		t.Fatalf("expected tx hash 0xcreditable001, got %q", got.TxHash)
	}

	if got.Status != model.DepositStatusConfirming {
		t.Fatalf("expected status %q, got %q", model.DepositStatusConfirming, got.Status)
	}

	if got.CreditedAt != nil {
		t.Fatalf("expected credited_at nil, got %v", got.CreditedAt)
	}
}

func TestDepositRepo_LockNextCreditableDeposit_SkipsNonCreditableDeposits(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)

	depositAddressID := insertTestDepositAddress(
		t,
		tx,
		userID,
		11155111,
		"0x5555555555555555555555555555555555555555",
	)

	otherChainDepositAddressID := insertTestDepositAddress(
		t,
		tx,
		userID,
		1,
		"0x5555555555555555555555555555555555555555",
	)

	now := time.Now()

	credited := newTestDeposit(userID, depositAddressID, "0xskipcredited")
	credited.Status = model.DepositStatusCredited
	credited.CreditedAt = &now
	if err := tx.Create(credited).Error; err != nil {
		t.Fatalf("insert credited deposit: %v", err)
	}

	creditedAtNotNil := newTestDeposit(userID, depositAddressID, "0xskipcreditedat")
	creditedAtNotNil.Status = model.DepositStatusConfirming
	creditedAtNotNil.CreditedAt = &now
	if err := tx.Create(creditedAtNotNil).Error; err != nil {
		t.Fatalf("insert credited_at deposit: %v", err)
	}

	wrongChain := newTestDeposit(userID, otherChainDepositAddressID, "0xskipwrongchain")
	wrongChain.ChainID = 1
	wrongChain.Status = model.DepositStatusConfirming
	wrongChain.CreditedAt = nil
	if err := tx.Create(wrongChain).Error; err != nil {
		t.Fatalf("insert wrong chain deposit: %v", err)
	}

	deposit, found, err := repo.LockNextCreditableDeposit(context.Background(), 11155111)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if found {
		t.Fatalf("expected found=false, got true with deposit %+v", deposit)
	}

	if deposit != nil {
		t.Fatalf("expected nil deposit, got %+v", deposit)
	}
}

func TestDepositRepo_LockNextCreditableDeposit_OrdersByBlockNumberAndIDAscending(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(
		t,
		tx,
		userID,
		11155111,
		"0x6666666666666666666666666666666666666666",
	)

	laterBlock := newTestDeposit(userID, depositAddressID, "0xorderlaterblock")
	laterBlock.BlockNumber = 101
	if err := tx.Create(laterBlock).Error; err != nil {
		t.Fatalf("insert later block deposit: %v", err)
	}

	firstAtBlock100 := newTestDeposit(userID, depositAddressID, "0xorderfirst")
	firstAtBlock100.BlockNumber = 100
	if err := tx.Create(firstAtBlock100).Error; err != nil {
		t.Fatalf("insert first block 100 deposit: %v", err)
	}

	secondAtBlock100 := newTestDeposit(userID, depositAddressID, "0xordersecond")
	secondAtBlock100.BlockNumber = 100
	if err := tx.Create(secondAtBlock100).Error; err != nil {
		t.Fatalf("insert second block 100 deposit: %v", err)
	}

	got, found, err := repo.LockNextCreditableDeposit(context.Background(), 11155111)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !found {
		t.Fatal("expected found=true, got false")
	}

	if got == nil {
		t.Fatal("expected deposit, got nil")
	}

	if got.ID != firstAtBlock100.ID {
		t.Fatalf("expected first block 100 deposit id %d, got %d", firstAtBlock100.ID, got.ID)
	}

	if got.TxHash != firstAtBlock100.TxHash {
		t.Fatalf("expected tx hash %q, got %q", firstAtBlock100.TxHash, got.TxHash)
	}
}

func TestDepositRepo_MarkDepositCredited_WithInvalidDepositID_ReturnsError(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	err := repo.MarkDepositCredited(context.Background(), 0)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "deposit_id must be positive") {
		t.Fatalf("expected deposit_id error, got %q", err.Error())
	}
}

func TestDepositRepo_MarkDepositCredited_MarksConfirmingUncreditedDeposit(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(
		t,
		tx,
		userID,
		11155111,
		"0x7777777777777777777777777777777777777777",
	)

	deposit := newTestDeposit(userID, depositAddressID, "0xmarkcredited001")
	deposit.Status = model.DepositStatusConfirming
	deposit.CreditedAt = nil

	if err := tx.Create(deposit).Error; err != nil {
		t.Fatalf("insert deposit: %v", err)
	}

	err := repo.MarkDepositCredited(context.Background(), deposit.ID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	var got model.Deposit
	if err := tx.Where("id = ?", deposit.ID).Take(&got).Error; err != nil {
		t.Fatalf("query deposit: %v", err)
	}

	if got.Status != model.DepositStatusCredited {
		t.Fatalf("expected status %q, got %q", model.DepositStatusCredited, got.Status)
	}

	if got.CreditedAt == nil {
		t.Fatal("expected credited_at to be set, got nil")
	}
}

func TestDepositRepo_MarkDepositCredited_WithMissingDeposit_ReturnsError(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	err := repo.MarkDepositCredited(context.Background(), 999999)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "mark deposit credited affected 0 rows") {
		t.Fatalf("expected affected 0 rows error, got %q", err.Error())
	}
}

func TestDepositRepo_MarkDepositCredited_WithAlreadyCreditedDeposit_ReturnsError(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(
		t,
		tx,
		userID,
		11155111,
		"0x8888888888888888888888888888888888888888",
	)

	now := time.Now()

	deposit := newTestDeposit(userID, depositAddressID, "0xalreadycredited001")
	deposit.Status = model.DepositStatusCredited
	deposit.CreditedAt = &now

	if err := tx.Create(deposit).Error; err != nil {
		t.Fatalf("insert deposit: %v", err)
	}

	err := repo.MarkDepositCredited(context.Background(), deposit.ID)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "mark deposit credited affected 0 rows") {
		t.Fatalf("expected affected 0 rows error, got %q", err.Error())
	}
}

func TestDepositRepo_MarkDepositCredited_WithCreditedAtAlreadySet_ReturnsError(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositRepo(tx)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(
		t,
		tx,
		userID,
		11155111,
		"0x9999999999999999999999999999999999999999",
	)

	now := time.Now()

	deposit := newTestDeposit(userID, depositAddressID, "0xcreditedatalreadyset001")
	deposit.Status = model.DepositStatusConfirming
	deposit.CreditedAt = &now

	if err := tx.Create(deposit).Error; err != nil {
		t.Fatalf("insert deposit: %v", err)
	}

	err := repo.MarkDepositCredited(context.Background(), deposit.ID)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "mark deposit credited affected 0 rows") {
		t.Fatalf("expected affected 0 rows error, got %q", err.Error())
	}
}
