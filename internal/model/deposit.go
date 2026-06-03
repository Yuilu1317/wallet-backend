package model

import "time"

type Deposit struct {
	ID               int64
	UserID           int64
	ChainID          int64
	DepositAddressID int64
	TxHash           string
	BlockNumber      int64
	BlockHash        string
	FromAddress      string
	ToAddress        string
	AmountWei        string
	Status           string
	ReceiptStatus    int16
	CreditedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Deposit) TableName() string {
	return "deposits"
}
