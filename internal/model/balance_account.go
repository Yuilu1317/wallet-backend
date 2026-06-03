package model

import "time"

type BalanceAccount struct {
	ID               int64
	UserID           int64
	ChainID          int64
	AssetSymbol      string
	AvailableBalance string
	FrozenBalance    string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (BalanceAccount) TableName() string {
	return "balance_accounts"
}
