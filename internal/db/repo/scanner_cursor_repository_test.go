package repo

import (
	"context"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/model"
)

func TestScannerCursorRepo_GetByChainIDAndScannerName_Found(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewScannerCursorRepo(tx)

	cursor := &model.WalletScannerCursor{
		ChainID:                11155111,
		ScannerName:            "native_eth_deposit_scanner",
		LastScannedBlockNumber: 100,
		LastScannedBlockHash:   "0xblockhash100",
	}

	if err := repo.UpsertAfterBlockProcessed(context.Background(), cursor); err != nil {
		t.Fatalf("upsert scanner cursor: %v", err)
	}

	got, found, err := repo.GetByChainIDAndScannerName(
		context.Background(),
		11155111,
		"native_eth_deposit_scanner",
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !found {
		t.Fatal("expected found=true, got false")
	}

	if got == nil {
		t.Fatal("expected cursor, got nil")
	}

	if got.ChainID != cursor.ChainID {
		t.Fatalf("expected chain id %d, got %d", cursor.ChainID, got.ChainID)
	}

	if got.ScannerName != cursor.ScannerName {
		t.Fatalf("expected scanner name %q, got %q", cursor.ScannerName, got.ScannerName)
	}

	if got.LastScannedBlockNumber != cursor.LastScannedBlockNumber {
		t.Fatalf("expected block number %d, got %d", cursor.LastScannedBlockNumber, got.LastScannedBlockNumber)
	}

	if got.LastScannedBlockHash != cursor.LastScannedBlockHash {
		t.Fatalf("expected block hash %q, got %q", cursor.LastScannedBlockHash, got.LastScannedBlockHash)
	}
}

func TestScannerCursorRepo_GetByChainIDAndScannerName_NotFound_ReturnsFalse(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewScannerCursorRepo(tx)

	got, found, err := repo.GetByChainIDAndScannerName(
		context.Background(),
		11155111,
		"native_eth_deposit_scanner",
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if found {
		t.Fatal("expected found=false, got true")
	}

	if got != nil {
		t.Fatalf("expected nil cursor, got %+v", got)
	}
}

func TestScannerCursorRepo_GetByChainIDAndScannerName_WrongChain_ReturnsFalse(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewScannerCursorRepo(tx)

	cursor := &model.WalletScannerCursor{
		ChainID:                11155111,
		ScannerName:            "native_eth_deposit_scanner",
		LastScannedBlockNumber: 100,
		LastScannedBlockHash:   "0xblockhash100",
	}

	if err := repo.UpsertAfterBlockProcessed(context.Background(), cursor); err != nil {
		t.Fatalf("upsert scanner cursor: %v", err)
	}

	got, found, err := repo.GetByChainIDAndScannerName(
		context.Background(),
		1,
		"native_eth_deposit_scanner",
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if found {
		t.Fatal("expected found=false for wrong chain, got true")
	}

	if got != nil {
		t.Fatalf("expected nil cursor, got %+v", got)
	}
}

func TestScannerCursorRepo_UpsertAfterBlockProcessed_InsertsThenUpdatesSameCursor(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewScannerCursorRepo(tx)

	first := &model.WalletScannerCursor{
		ChainID:                11155111,
		ScannerName:            "native_eth_deposit_scanner",
		LastScannedBlockNumber: 100,
		LastScannedBlockHash:   "0xblockhash100",
	}

	if err := repo.UpsertAfterBlockProcessed(context.Background(), first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := &model.WalletScannerCursor{
		ChainID:                11155111,
		ScannerName:            "native_eth_deposit_scanner",
		LastScannedBlockNumber: 101,
		LastScannedBlockHash:   "0xblockhash101",
	}

	if err := repo.UpsertAfterBlockProcessed(context.Background(), second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var count int64
	if err := tx.Raw(
		"SELECT COUNT(*) FROM wallet_scanner_cursors WHERE chain_id = ? AND scanner_name = ?",
		11155111,
		"native_eth_deposit_scanner",
	).Scan(&count).Error; err != nil {
		t.Fatalf("count scanner cursors: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one cursor row, got %d", count)
	}

	got, found, err := repo.GetByChainIDAndScannerName(
		context.Background(),
		11155111,
		"native_eth_deposit_scanner",
	)
	if err != nil {
		t.Fatalf("get cursor: %v", err)
	}

	if !found {
		t.Fatal("expected found=true, got false")
	}

	if got.LastScannedBlockNumber != 101 {
		t.Fatalf("expected block number 101, got %d", got.LastScannedBlockNumber)
	}

	if got.LastScannedBlockHash != "0xblockhash101" {
		t.Fatalf("expected block hash 0xblockhash101, got %q", got.LastScannedBlockHash)
	}
}

func TestScannerCursorRepo_UpsertAfterBlockProcessed_RepeatedSameBlockDoesNotCreateDuplicate(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewScannerCursorRepo(tx)

	cursor := &model.WalletScannerCursor{
		ChainID:                11155111,
		ScannerName:            "native_eth_deposit_scanner",
		LastScannedBlockNumber: 100,
		LastScannedBlockHash:   "0xblockhash100",
	}

	if err := repo.UpsertAfterBlockProcessed(context.Background(), cursor); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	if err := repo.UpsertAfterBlockProcessed(context.Background(), cursor); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var count int64
	if err := tx.Raw(
		"SELECT COUNT(*) FROM wallet_scanner_cursors WHERE chain_id = ? AND scanner_name = ?",
		11155111,
		"native_eth_deposit_scanner",
	).Scan(&count).Error; err != nil {
		t.Fatalf("count scanner cursors: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one cursor row, got %d", count)
	}
}

func TestScannerCursorRepo_UpsertAfterBlockProcessed_InvalidCursorReturnsError(t *testing.T) {
	tx := beginTestTransaction(t)
	repo := NewScannerCursorRepo(tx)

	tests := []struct {
		name    string
		cursor  *model.WalletScannerCursor
		wantErr string
	}{
		{
			name:    "nil cursor",
			cursor:  nil,
			wantErr: "cursor is required",
		},
		{
			name: "non-positive chain id",
			cursor: &model.WalletScannerCursor{
				ChainID:                0,
				ScannerName:            "native_eth_deposit_scanner",
				LastScannedBlockNumber: 100,
				LastScannedBlockHash:   "0xblockhash100",
			},
			wantErr: "cursor.chain_id must be positive",
		},
		{
			name: "missing scanner name",
			cursor: &model.WalletScannerCursor{
				ChainID:                11155111,
				ScannerName:            "",
				LastScannedBlockNumber: 100,
				LastScannedBlockHash:   "0xblockhash100",
			},
			wantErr: "cursor.scanner_name is required",
		},
		{
			name: "negative block number",
			cursor: &model.WalletScannerCursor{
				ChainID:                11155111,
				ScannerName:            "native_eth_deposit_scanner",
				LastScannedBlockNumber: -1,
				LastScannedBlockHash:   "0xblockhash100",
			},
			wantErr: "cursor.last_scanned_block_number must be non-negative",
		},
		{
			name: "missing block hash",
			cursor: &model.WalletScannerCursor{
				ChainID:                11155111,
				ScannerName:            "native_eth_deposit_scanner",
				LastScannedBlockNumber: 100,
				LastScannedBlockHash:   "",
			},
			wantErr: "cursor.last_scanned_block_hash is required",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			err := repo.UpsertAfterBlockProcessed(context.Background(), tt.cursor)

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
