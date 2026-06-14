package repo

import (
	"context"

	"github.com/Yuilu1317/wallet-backend/internal/service"
	"gorm.io/gorm"
)

type DepositCreditTransactionRunner struct {
	db *gorm.DB
}

func NewDepositCreditTransactionRunner(db *gorm.DB) *DepositCreditTransactionRunner {
	return &DepositCreditTransactionRunner{db: db}
}

func (r *DepositCreditTransactionRunner) WithinTransaction(
	ctx context.Context,
	fn func(repos service.DepositCreditRepositories) error,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repos := service.DepositCreditRepositories{
			CreditDepositRepository:        NewDepositRepo(tx),
			CreditBalanceLedgerRepository:  NewBalanceLedgerRepository(tx),
			CreditBalanceAccountRepository: NewBalanceAccountRepository(tx),
		}

		return fn(repos)
	})
}
