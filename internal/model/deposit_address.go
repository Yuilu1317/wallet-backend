package model

import "time"

type DepositAddress struct {
	ID           int64
	UserID       int64
	ChainID      int64
	Address      string
	AddressLower string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (DepositAddress) TableName() string {
	return "deposit_addresses"
}
