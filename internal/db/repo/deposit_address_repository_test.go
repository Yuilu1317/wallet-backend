package repo

import (
	"context"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/model"
)

func TestDepositAddressRepo_FindActiveByChainIDAndAddressLower_Found(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositAddressRepo(tx)

	chainID := int64(11155111)
	address := "0x3333333333333333333333333333333333333333"
	addressLower := strings.ToLower(address)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(t, tx, userID, chainID, address)

	got, found, err := repo.FindActiveByChainIDAndAddressLower(
		context.Background(),
		chainID,
		addressLower,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !found {
		t.Fatal("expected found=true, got false")
	}

	if got == nil {
		t.Fatal("expected deposit address, got nil")
	}

	if got.ID != depositAddressID {
		t.Fatalf("expected id %d, got %d", depositAddressID, got.ID)
	}

	if got.UserID != userID {
		t.Fatalf("expected user id %d, got %d", userID, got.UserID)
	}

	if got.ChainID != chainID {
		t.Fatalf("expected chain id %d, got %d", chainID, got.ChainID)
	}

	if got.AddressLower != addressLower {
		t.Fatalf("expected address lower %q, got %q", addressLower, got.AddressLower)
	}

	if got.Status != model.DepositAddressStatusActive {
		t.Fatalf("expected status %q, got %q", model.DepositAddressStatusActive, got.Status)
	}
}

func TestDepositAddressRepo_FindActiveByChainIDAndAddressLower_NotFound_ReturnsFalse(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositAddressRepo(tx)

	got, found, err := repo.FindActiveByChainIDAndAddressLower(
		context.Background(),
		11155111,
		"0x4444444444444444444444444444444444444444",
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if found {
		t.Fatal("expected found=false, got true")
	}

	if got != nil {
		t.Fatalf("expected nil deposit address, got %+v", got)
	}
}

func TestDepositAddressRepo_FindActiveByChainIDAndAddressLower_DisabledAddress_ReturnsFalse(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositAddressRepo(tx)

	chainID := int64(11155111)
	address := "0x5555555555555555555555555555555555555555"
	addressLower := strings.ToLower(address)

	userID := insertTestUser(t, tx)
	depositAddressID := insertTestDepositAddress(t, tx, userID, chainID, address)

	if err := tx.Exec(
		"UPDATE deposit_addresses SET status = ? WHERE id = ?",
		model.DepositAddressStatusDisabled,
		depositAddressID,
	).Error; err != nil {
		t.Fatalf("disable deposit address: %v", err)
	}

	got, found, err := repo.FindActiveByChainIDAndAddressLower(
		context.Background(),
		chainID,
		addressLower,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if found {
		t.Fatal("expected found=false for disabled address, got true")
	}

	if got != nil {
		t.Fatalf("expected nil deposit address, got %+v", got)
	}
}

func TestDepositAddressRepo_FindActiveByChainIDAndAddressLower_WrongChain_ReturnsFalse(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewDepositAddressRepo(tx)

	address := "0x6666666666666666666666666666666666666666"
	addressLower := strings.ToLower(address)

	userID := insertTestUser(t, tx)
	insertTestDepositAddress(t, tx, userID, 11155111, address)

	got, found, err := repo.FindActiveByChainIDAndAddressLower(
		context.Background(),
		1,
		addressLower,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if found {
		t.Fatal("expected found=false for wrong chain, got true")
	}

	if got != nil {
		t.Fatalf("expected nil deposit address, got %+v", got)
	}
}
