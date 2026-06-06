package repo

import (
	"os"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func beginTestTransaction(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}

	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}

	t.Cleanup(func() {
		_ = tx.Rollback()
	})

	return tx
}

func insertTestUser(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var id int64
	if err := db.Raw("INSERT INTO users DEFAULT VALUES RETURNING id").Scan(&id).Error; err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	return id
}

func insertTestDepositAddress(t *testing.T, db *gorm.DB, userID int64, chainID int64, address string) int64 {
	t.Helper()

	var id int64
	if err := db.Raw(
		`
INSERT INTO deposit_addresses (user_id, chain_id, address, address_lower, status)
VALUES (?, ?, ?, lower(?), ?)
RETURNING id
`,
		userID,
		chainID,
		address,
		address,
		model.DepositAddressStatusActive,
	).Scan(&id).Error; err != nil {
		t.Fatalf("insert test deposit address: %v", err)
	}

	return id
}
