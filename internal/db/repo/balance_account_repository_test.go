package repo

import (
	"context"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/model"
)

func newTestBalanceAccount(userID int64, amountWei string) *model.BalanceAccount {
	return &model.BalanceAccount{
		UserID:           userID,
		ChainID:          11155111,
		AssetSymbol:      model.AssetSymbolETH,
		AvailableBalance: amountWei,
		FrozenBalance:    "0",
	}
}

func TestValidateAddAvailableBalance_WithValidAccount_ReturnsNil(t *testing.T) {
	account := newTestBalanceAccount(1, "1000000000000000000")

	err := validateAddAvailableBalance(account)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateAddAvailableBalance_WithInvalidAccount_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		account *model.BalanceAccount
		wantErr string
	}{
		{
			name:    "nil account",
			account: nil,
			wantErr: "account is nil",
		},
		{
			name: "account id is not zero",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "1000000000000000000")
				account.ID = 1
				return account
			}(),
			wantErr: "account id must be zero",
		},
		{
			name: "user id is zero",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(0, "1000000000000000000")
				return account
			}(),
			wantErr: "user_id must be positive",
		},
		{
			name: "chain id is zero",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "1000000000000000000")
				account.ChainID = 0
				return account
			}(),
			wantErr: "chain_id must be positive",
		},
		{
			name: "asset symbol is empty",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "1000000000000000000")
				account.AssetSymbol = ""
				return account
			}(),
			wantErr: "asset_symbol is empty",
		},
		{
			name: "asset symbol is not ETH",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "1000000000000000000")
				account.AssetSymbol = "USDT"
				return account
			}(),
			wantErr: "asset_symbol must be ETH",
		},
		{
			name: "available balance is empty",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "")
				return account
			}(),
			wantErr: "available_balance is empty",
		},
		{
			name: "available balance is invalid integer",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "abc")
				return account
			}(),
			wantErr: "invalid integer format",
		},
		{
			name: "available balance is zero",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "0")
				return account
			}(),
			wantErr: "amount must be positive",
		},
		{
			name: "available balance is negative",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "-1")
				return account
			}(),
			wantErr: "amount must be positive",
		},
		{
			name: "frozen balance is empty",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "1000000000000000000")
				account.FrozenBalance = ""
				return account
			}(),
			wantErr: "frozen_balance is empty",
		},
		{
			name: "frozen balance is not zero",
			account: func() *model.BalanceAccount {
				account := newTestBalanceAccount(1, "1000000000000000000")
				account.FrozenBalance = "1"
				return account
			}(),
			wantErr: "frozen_balance must be 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddAvailableBalance(tt.account)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestBalanceAccountRepository_UniqueConstraint_PreventsDuplicateAccountRows(t *testing.T) {
	tx := beginTestTransaction(t)

	userID := insertTestUser(t, tx)

	first := newTestBalanceAccount(userID, "1000000000000000000")
	second := newTestBalanceAccount(userID, "2000000000000000000")

	if err := tx.Create(first).Error; err != nil {
		t.Fatalf("insert first balance account: %v", err)
	}

	err := tx.Create(second).Error
	if err == nil {
		t.Fatal("expected duplicate balance account insert to fail, got nil")
	}

	var count int64
	if err := tx.Raw(
		`
		SELECT COUNT(*)
		FROM balance_accounts
		WHERE user_id = ? AND chain_id = ? AND asset_symbol = ?
		`,
		userID,
		int64(11155111),
		model.AssetSymbolETH,
	).Scan(&count).Error; err != nil {
		t.Fatalf("count balance accounts: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one balance account row, got %d", count)
	}
}

func TestBalanceAccountRepository_AddAvailableBalance_CreatesAccount(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewBalanceAccountRepository(tx)

	userID := insertTestUser(t, tx)

	account := newTestBalanceAccount(userID, "1000000000000000000")

	err := repo.AddAvailableBalance(context.Background(), account)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	var count int64
	if err := tx.Raw(
		`
		SELECT COUNT(*)
		FROM balance_accounts
		WHERE user_id = ? AND chain_id = ? AND asset_symbol = ?
		`,
		userID,
		int64(11155111),
		model.AssetSymbolETH,
	).Scan(&count).Error; err != nil {
		t.Fatalf("count balance accounts: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one balance account row, got %d", count)
	}

	var availableBalance string
	var frozenBalance string

	if err := tx.Raw(
		`
		SELECT available_balance::TEXT, frozen_balance::TEXT
		FROM balance_accounts
		WHERE user_id = ? AND chain_id = ? AND asset_symbol = ?
		`,
		userID,
		int64(11155111),
		model.AssetSymbolETH,
	).Row().Scan(&availableBalance, &frozenBalance); err != nil {
		t.Fatalf("query balance account: %v", err)
	}

	if availableBalance != "1000000000000000000" {
		t.Fatalf("expected available_balance=1000000000000000000, got %s", availableBalance)
	}

	if frozenBalance != "0" {
		t.Fatalf("expected frozen_balance=0, got %s", frozenBalance)
	}
}

func TestBalanceAccountRepository_AddAvailableBalance_ExistingAccountAddsAvailableBalance(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewBalanceAccountRepository(tx)

	userID := insertTestUser(t, tx)

	first := newTestBalanceAccount(userID, "1000000000000000000")
	if err := repo.AddAvailableBalance(context.Background(), first); err != nil {
		t.Fatalf("first add available balance: %v", err)
	}

	second := newTestBalanceAccount(userID, "2000000000000000000")
	if err := repo.AddAvailableBalance(context.Background(), second); err != nil {
		t.Fatalf("second add available balance: %v", err)
	}

	var count int64
	if err := tx.Raw(
		`
		SELECT COUNT(*)
		FROM balance_accounts
		WHERE user_id = ? AND chain_id = ? AND asset_symbol = ?
		`,
		userID,
		int64(11155111),
		model.AssetSymbolETH,
	).Scan(&count).Error; err != nil {
		t.Fatalf("count balance accounts: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one balance account row after repeated add, got %d", count)
	}

	var availableBalance string
	var frozenBalance string

	if err := tx.Raw(
		`
		SELECT available_balance::TEXT, frozen_balance::TEXT
		FROM balance_accounts
		WHERE user_id = ? AND chain_id = ? AND asset_symbol = ?
		`,
		userID,
		int64(11155111),
		model.AssetSymbolETH,
	).Row().Scan(&availableBalance, &frozenBalance); err != nil {
		t.Fatalf("query balance account: %v", err)
	}

	if availableBalance != "3000000000000000000" {
		t.Fatalf("expected available_balance=3000000000000000000, got %s", availableBalance)
	}

	if frozenBalance != "0" {
		t.Fatalf("expected frozen_balance=0, got %s", frozenBalance)
	}
}
