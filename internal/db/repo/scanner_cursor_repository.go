package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Yuilu1317/wallet-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ScannerCursorRepo struct {
	db *gorm.DB
}

func NewScannerCursorRepo(db *gorm.DB) *ScannerCursorRepo {
	return &ScannerCursorRepo{db: db}
}

// GetByChainIDAndScannerName returns the scanner cursor for a chain and scanner.
//
// found=false is not an error. It means this scanner has not processed any block
// yet, so the caller should start from the configured scanner start block.
func (r *ScannerCursorRepo) GetByChainIDAndScannerName(
	ctx context.Context,
	chainID int64,
	scannerName string,
) (*model.WalletScannerCursor, bool, error) {
	var cursor model.WalletScannerCursor

	if err := r.db.WithContext(ctx).
		Where("chain_id = ? ", chainID).
		Where("scanner_name = ?", scannerName).
		Take(&cursor).
		Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get scanner cursor: %w", err)
	}
	return &cursor, true, nil
}

// UpsertAfterBlockProcessed records the last fully processed block for a scanner.
//
// The caller must only call this after the whole block has been processed
// successfully. Updating the cursor too early can cause missed deposits after a
// crash or restart.
func (r *ScannerCursorRepo) UpsertAfterBlockProcessed(
	ctx context.Context,
	cursor *model.WalletScannerCursor,
) error {
	if err := validateScannerCursorForUpsert(cursor); err != nil {
		return err
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "chain_id"},
				{Name: "scanner_name"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"last_scanned_block_number": cursor.LastScannedBlockNumber,
				"last_scanned_block_hash":   cursor.LastScannedBlockHash,
				"updated_at":                gorm.Expr("now()"),
			}),
		}).
		Create(cursor)

	if result.Error != nil {
		return fmt.Errorf("upsert scanner cursor after block processed: %w", result.Error)
	}
	return nil
}

func validateScannerCursorForUpsert(cursor *model.WalletScannerCursor) error {
	if cursor == nil {
		return fmt.Errorf("cursor is required")
	}

	if err := validatePositiveInt64("cursor.chain_id", cursor.ChainID); err != nil {
		return err
	}

	if err := validateRequiredString("cursor.scanner_name", cursor.ScannerName); err != nil {
		return err
	}

	if err := validateNonNegativeInt64("cursor.last_scanned_block_number", cursor.LastScannedBlockNumber); err != nil {
		return err
	}

	if err := validateRequiredString("cursor.last_scanned_block_hash", cursor.LastScannedBlockHash); err != nil {
		return err
	}

	return nil
}

func validateRequiredString(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func validatePositiveInt64(name string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func validateNonNegativeInt64(name string, value int64) error {
	if value < 0 {
		return fmt.Errorf("%s must be non-negative", name)
	}
	return nil
}
