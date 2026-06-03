package model

import "time"

type WalletScannerCursor struct {
	ID                     int64
	ChainID                int64
	ScannerName            string
	LastScannedBlockNumber int64
	LastScannedBlockHash   string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (*WalletScannerCursor) TableName() string {
	return "wallet_scanner_cursors"
}
