package repo

import (
	"context"

	"github.com/Yuilu1317/wallet-backend/internal/scanner"
	"gorm.io/gorm"
)

type ScannerTransactionRunner struct {
	db *gorm.DB
}

func NewScannerTransactionRunner(db *gorm.DB) *ScannerTransactionRunner {
	return &ScannerTransactionRunner{db: db}
}

func (r *ScannerTransactionRunner) WithinTransaction(
	ctx context.Context,
	fn func(repos scanner.Repositories) error,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repos := scanner.Repositories{
			DepositAddressRepo: NewDepositAddressRepo(tx),
			DepositRepo:        NewDepositRepo(tx),
			CursorRepo:         NewScannerCursorRepo(tx),
		}

		return fn(repos)
	})
}
