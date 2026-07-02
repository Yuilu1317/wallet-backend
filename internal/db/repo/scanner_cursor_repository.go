package repo

import (
	"context"
	"errors"
	"fmt"

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

func (r *ScannerCursorRepo) GetByChainIDAndScannerName(
	ctx context.Context,
	chainID int64,
	scannerName string,
) (*model.WalletScannerCursor, bool, error) {
	if err := validatePositiveInt64("chain_id", chainID); err != nil {
		return nil, false, err
	}

	if err := validateRequiredString("scanner_name", scannerName); err != nil {
		return nil, false, err
	}
	var cursor model.WalletScannerCursor
	if err := r.db.WithContext(ctx).
		Where("chain_id = ? AND scanner_name = ?", chainID, scannerName).
		Take(&cursor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		if mapped := mapDBError(err); mapped != nil {
			return nil, false, fmt.Errorf("get scanner cursor by chain_id and scanner_name: %w", mapped)
		}
		return nil, false, fmt.Errorf("get scanner cursor by chain_id and scanner_name: %w", err)
	}
	return &cursor, true, nil
}

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
		}).Create(cursor)
	if result.Error != nil {
		if mapped := mapDBError(result.Error); mapped != nil {
			return fmt.Errorf("upsert scanner cursor after block processed: %w", mapped)
		}
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
