package model

import "time"

type BalanceLedger struct {
	ID          int64
	UserID      int64
	ChainID     int64
	AssetSymbol string
	AmountWei   string
	Direction   string
	Reason      string
	SourceType  string
	SourceID    int64
	CreatedAt   time.Time
}

func (BalanceLedger) TableName() string {
	return "balance_ledgers"
}
