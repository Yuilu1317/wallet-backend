package service_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/db/repo"
	"github.com/Yuilu1317/wallet-backend/internal/model"
	"github.com/Yuilu1317/wallet-backend/internal/service"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func replacePostgresDSNDatabase(t *testing.T, dsn string, dbname string) string {
	t.Helper()

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse url dsn: %v", err)
		}
		u.Path = "/" + dbname
		return u.String()
	}

	parts := strings.Fields(dsn)
	replace := false

	for i, part := range parts {
		if strings.HasPrefix(part, "dbname=") {
			parts[i] = "dbname=" + dbname
			replace = true
			break
		}
	}
	if !replace {
		parts = append(parts, "dbname="+dbname)
	}
	return strings.Join(parts, " ")
}

func migrateTestTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlByte, err := os.ReadFile("../../migrations/001_init_schema.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	if err := db.Exec(string(sqlByte)).Error; err != nil {
		t.Fatalf("migration test tables: %v", err)
	}
}

func loadTestEnv(t *testing.T) {
	t.Helper()

	_ = godotenv.Load("../../.env.test")
}

func createTempPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	loadTestEnv(t)

	adminDSN := strings.TrimSpace(os.Getenv("TEST_ADMIN_DSN"))
	if adminDSN == "" {
		t.Skip("admin dsn is not set")
	}

	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}

	adminSQLDB, err := adminDB.DB()
	if err != nil {
		t.Fatalf("open admin sql db: %v", err)
	}

	t.Cleanup(func() {
		_ = adminSQLDB.Close()
	})

	testDBName := fmt.Sprintf(
		"wallet_backend_test_%d_%d",
		time.Now().UnixNano(),
		os.Getpid())

	if err := adminDB.Exec(`CREATE DATABASE "` + testDBName + `"`).Error; err != nil {
		t.Fatalf("create test db: %v", err)
	}

	testDSN := replacePostgresDSNDatabase(t, adminDSN, testDBName)

	testDB, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	testSQLDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("open test sql db: %v", err)
	}

	if err := testSQLDB.Ping(); err != nil {
		t.Fatalf("ping test sql db: %v", err)
	}

	t.Cleanup(func() {
		_ = testSQLDB.Close()

		_ = adminDB.Exec(`
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = ?
AND pid <> pg_backend_pid()
`, testDBName).Error

		if err := adminDB.Exec(`DROP DATABASE IF EXISTS "` + testDBName + `"`).Error; err != nil {
			t.Fatalf("drop temp test db %s: %v", testDBName, err)
		}
	})

	migrateTestTables(t, testDB)

	return testDB
}

func TestCreateTempPostgresTestDB_MigratesTables(t *testing.T) {
	db := createTempPostgresTestDB(t)
	var exists bool
	if err := db.Raw(`
SELECT EXISTS (
SELECT 1
FROM information_schema.tables
WHERE table_schema = 'public'
AND table_name = 'deposits'
)
`).Scan(&exists).Error; err != nil {
		t.Fatalf("check deposits table exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected deposits table to exist after migration")
	}
}

type creditNextSeedData struct {
	UserID           int64
	ChainID          int64
	DepositAddressID int64
	DepositID        int64
	InitialBalance   string
	DepositAmount    string
}

func seedCreditableDepositData(t *testing.T, db *gorm.DB) creditNextSeedData {
	t.Helper()

	user := &model.User{
		Status: model.UserStatusActive,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	const chainID int64 = 11155111

	depositAddress := &model.DepositAddress{
		UserID:       user.ID,
		ChainID:      chainID,
		Address:      "0x1111111111111111111111111111111111111111",
		AddressLower: "0x1111111111111111111111111111111111111111",
		Status:       model.DepositAddressStatusActive,
	}
	if err := db.Create(depositAddress).Error; err != nil {
		t.Fatalf("create deposit address: %v", err)
	}

	balanceAccount := &model.BalanceAccount{
		UserID:           user.ID,
		ChainID:          chainID,
		AssetSymbol:      model.AssetSymbolETH,
		AvailableBalance: "5000",
		FrozenBalance:    "0",
	}
	if err := db.Create(balanceAccount).Error; err != nil {
		t.Fatalf("create balance account: %v", err)
	}

	deposit := &model.Deposit{
		UserID:           user.ID,
		ChainID:          chainID,
		DepositAddressID: depositAddress.ID,
		TxHash:           "0xdeposit001",
		BlockNumber:      100,
		BlockHash:        "0xblockhash001",
		FromAddress:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ToAddress:        depositAddress.Address,
		AmountWei:        "1000",
		ReceiptStatus:    1,
		Status:           model.DepositStatusConfirming,
		CreditedAt:       nil,
	}
	if err := db.Create(deposit).Error; err != nil {
		t.Fatalf("create deposit: %v", err)
	}

	return creditNextSeedData{
		UserID:           user.ID,
		ChainID:          chainID,
		DepositAddressID: depositAddress.ID,
		DepositID:        deposit.ID,
		InitialBalance:   balanceAccount.AvailableBalance,
		DepositAmount:    deposit.AmountWei,
	}
}

func TestSeedCreditableDepositData(t *testing.T) {
	db := createTempPostgresTestDB(t)
	seed := seedCreditableDepositData(t, db)

	var user model.User
	if err := db.First(&user, seed.UserID).Error; err != nil {
		t.Fatalf("query seeded user: %v", err)
	}

	var depositAddress model.DepositAddress
	if err := db.First(&depositAddress, seed.DepositAddressID).Error; err != nil {
		t.Fatalf("query seeded deposit address: %v", err)
	}
	if depositAddress.Status != model.DepositAddressStatusActive {
		t.Fatalf("expected deposit address status=%s, got %s", model.DepositAddressStatusActive, depositAddress.Status)
	}

	var deposit model.Deposit
	if err := db.First(&deposit, seed.DepositID).Error; err != nil {
		t.Fatalf("query seeded deposit: %v", err)
	}
	if deposit.Status != model.DepositStatusConfirming {
		t.Fatalf("expect deposit status %s, got %s", model.DepositStatusConfirming, deposit.Status)
	}
	if deposit.CreditedAt != nil {
		t.Fatal("expect credited_at to be nil")
	}
	if deposit.AmountWei != seed.DepositAmount {
		t.Fatalf("expect deposit amount %s, got %s", seed.DepositAmount, deposit.AmountWei)
	}

	var account model.BalanceAccount
	if err := db.
		Where("user_id = ? AND chain_id = ? AND asset_symbol = ?", seed.UserID, seed.ChainID, model.AssetSymbolETH).
		First(&account).Error; err != nil {
		t.Fatalf("query seeded balance account: %v", err)
	}
	if account.AvailableBalance != seed.InitialBalance {
		t.Fatalf("expect available balance %s, got %s", seed.InitialBalance, account.AvailableBalance)
	}
}

func newTestDepositCreditService(t *testing.T, db *gorm.DB) *service.DepositCreditService {
	t.Helper()
	txRunner := repo.NewDepositCreditTransactionRunner(db)
	svc, err := service.NewDepositCreditService(txRunner, time.Second)
	if err != nil {
		t.Fatalf("create deposit credit service: %v", err)
	}
	return svc
}

func queryDepositCreditLedgers(t *testing.T, db *gorm.DB, depositID int64) []model.BalanceLedger {
	t.Helper()
	var ledgers []model.BalanceLedger
	if err := db.
		Where("source_type = ? AND source_id = ?", model.LedgerSourceTypeDeposit, depositID).
		Find(&ledgers).Error; err != nil {
		t.Fatalf("query deposit credit ledgers: %v", err)
	}
	return ledgers
}

func queryBalanceAccount(t *testing.T, db *gorm.DB, userID int64, chainID int64) model.BalanceAccount {
	t.Helper()
	var account model.BalanceAccount
	if err := db.
		Where("user_id = ? AND chain_id = ? AND asset_symbol = ?", userID, chainID, model.AssetSymbolETH).
		First(&account).Error; err != nil {
		t.Fatalf("query credited balance account: %v", err)
	}
	return account
}

func addWeiStrings(t *testing.T, a string, b string) string {
	t.Helper()
	x, ok := new(big.Int).SetString(a, 10)
	if !ok {
		t.Fatalf("invalid wei amount: %s", a)
	}
	y, ok := new(big.Int).SetString(b, 10)
	if !ok {
		t.Fatalf("invalid wei amount: %s", b)
	}
	z := new(big.Int).Add(x, y)
	return z.String()
}

func queryDeposit(t *testing.T, db *gorm.DB, depositID int64) model.Deposit {
	t.Helper()
	var deposit model.Deposit
	if err := db.First(&deposit, depositID).Error; err != nil {
		t.Fatalf("query deposit: %v", err)
	}
	return deposit
}

func TestDepositCreditService_CreditNext_WithTestDB_CreditDeposit(t *testing.T) {
	ctx := context.Background()
	db := createTempPostgresTestDB(t)
	seed := seedCreditableDepositData(t, db)
	svc := newTestDepositCreditService(t, db)
	credited, err := svc.CreditNext(ctx, seed.ChainID)
	if err != nil {
		t.Fatalf("credit next deposit: %v", err)
	}
	if !credited {
		t.Fatalf("expected credited=true, got false")
	}

	deposit := queryDeposit(t, db, seed.DepositID)
	if deposit.Status != model.DepositStatusCredited {
		t.Fatalf("expect deposit status=%s, got %s", model.DepositStatusCredited, deposit.Status)
	}
	if deposit.CreditedAt == nil {
		t.Fatalf("expect deposit credited_at to be not nil")
	}

	ledgers := queryDepositCreditLedgers(t, db, seed.DepositID)
	if len(ledgers) != 1 {
		t.Fatalf("expect 1 deposit credit ledger, got %d", len(ledgers))
	}
	ledger := ledgers[0]
	if ledger.AmountWei != seed.DepositAmount {
		t.Fatalf("expected ledger amount_wei=%s, got %s", seed.DepositAmount, ledger.AmountWei)
	}
	if ledger.Direction != model.LedgerDirectionCredit {
		t.Fatalf("expected ledger direction=%s, got %s", model.LedgerDirectionCredit, ledger.Direction)
	}
	if ledger.Reason != model.LedgerReasonDepositCredit {
		t.Fatalf("expected ledger reason=%s, got %s", model.LedgerReasonDepositCredit, ledger.Reason)
	}
	if ledger.UserID != seed.UserID {
		t.Fatalf("expected ledger user_id=%d, got %d", seed.UserID, ledger.UserID)
	}
	if ledger.ChainID != seed.ChainID {
		t.Fatalf("expected ledger chain_id=%d, got %d", seed.ChainID, ledger.ChainID)
	}

	account := queryBalanceAccount(t, db, seed.UserID, seed.ChainID)

	expectedBalance := addWeiStrings(t, seed.InitialBalance, seed.DepositAmount)

	if account.AvailableBalance != expectedBalance {
		t.Fatalf("expected available_balance=%s, got %s", expectedBalance, account.AvailableBalance)
	}
}

func TestDepositCreditService_CreditNext_WithTestDB_SecondCallDoesNotCreditAgain(t *testing.T) {
	ctx := context.Background()
	db := createTempPostgresTestDB(t)
	seed := seedCreditableDepositData(t, db)
	svc := newTestDepositCreditService(t, db)
	credited, err := svc.CreditNext(ctx, seed.ChainID)
	if err != nil {
		t.Fatalf("credit next deposit: %v", err)
	}
	if !credited {
		t.Fatalf("expected credited=true, got false")
	}
	secondCredited, err := svc.CreditNext(ctx, seed.ChainID)
	if err != nil {
		t.Fatalf("credit next deposit: %v", err)
	}
	if secondCredited {
		t.Fatal("expected second credited=false, got true")
	}

	ledgers := queryDepositCreditLedgers(t, db, seed.DepositID)
	if len(ledgers) != 1 {
		t.Fatalf("expect 1 deposit credit ledger, got %d", len(ledgers))
	}

	account := queryBalanceAccount(t, db, seed.UserID, seed.ChainID)

	expectedBalance := addWeiStrings(t, seed.InitialBalance, seed.DepositAmount)
	if account.AvailableBalance != expectedBalance {
		t.Fatalf("expected available_balance=%s, got %s", expectedBalance, account.AvailableBalance)
	}

	deposit := queryDeposit(t, db, seed.DepositID)
	if deposit.Status != model.DepositStatusCredited {
		t.Fatalf("expected deposit status=%s, got %s", model.DepositStatusCredited, deposit.Status)
	}
	if deposit.CreditedAt == nil {
		t.Fatal("expected deposit credited_at to be not nil")
	}
}

func TestDepositCreditService_CreditNext_WithTestDB_WhenNoCreditableDeposit_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	db := createTempPostgresTestDB(t)
	svc := newTestDepositCreditService(t, db)
	const chainID int64 = 11155111
	credited, err := svc.CreditNext(ctx, chainID)
	if err != nil {
		t.Fatalf("credit next deposit: %v", err)
	}
	if credited {
		t.Fatal("expected credited=false, got true")
	}
}

func TestDepositCreditService_CreditNext_WithTestDB_WhenLedgerAlreadyExists_ReturnsErrorAndDoesNotChangeBalance(t *testing.T) {
	ctx := context.Background()
	db := createTempPostgresTestDB(t)
	seed := seedCreditableDepositData(t, db)
	existingLedger := &model.BalanceLedger{
		UserID:      seed.UserID,
		ChainID:     seed.ChainID,
		AssetSymbol: model.AssetSymbolETH,
		AmountWei:   seed.DepositAmount,
		Direction:   model.LedgerDirectionCredit,
		Reason:      model.LedgerReasonDepositCredit,
		SourceType:  model.LedgerSourceTypeDeposit,
		SourceID:    seed.DepositID,
	}
	if err := db.Create(existingLedger).Error; err != nil {
		t.Fatalf("create exists ledger: %v", err)
	}
	svc := newTestDepositCreditService(t, db)
	credited, err := svc.CreditNext(ctx, seed.ChainID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if credited {
		t.Fatal("expected credited=false, got true")
	}
	if !strings.Contains(err.Error(), "deposit credit ledger already exists") {
		t.Fatalf("expected ledger already exists error, got %q", err.Error())
	}

	ledgers := queryDepositCreditLedgers(t, db, seed.DepositID)

	if len(ledgers) != 1 {
		t.Fatalf("expected 1 deposit credit ledger, got %d", len(ledgers))
	}

	account := queryBalanceAccount(t, db, seed.UserID, seed.ChainID)

	if account.AvailableBalance != seed.InitialBalance {
		t.Fatalf("expected available_balance=%s, got %s", seed.InitialBalance, account.AvailableBalance)
	}

	deposit := queryDeposit(t, db, seed.DepositID)
	if deposit.Status != model.DepositStatusConfirming {
		t.Fatalf("expected deposit status=%s, got %s", model.DepositStatusConfirming, deposit.Status)
	}

	if deposit.CreditedAt != nil {
		t.Fatal("expected deposit credited_at to still be nil")
	}
}

func TestDepositCreditService_CreditNext_WithTestDB_WhenBalanceAccountMissing_CreatesAccountAndCreditsDeposit(t *testing.T) {
	ctx := context.Background()

	db := createTempPostgresTestDB(t)
	seed := seedCreditableDepositData(t, db)

	result := db.
		Where("user_id = ? AND chain_id = ? AND asset_symbol = ?", seed.UserID, seed.ChainID, model.AssetSymbolETH).
		Delete(&model.BalanceAccount{})
	if result.Error != nil {
		t.Fatalf("delete seeded balance account: %v", result.Error)
	}
	if result.RowsAffected != 1 {
		t.Fatalf("expected to delete 1 balance account, got %d", result.RowsAffected)
	}

	svc := newTestDepositCreditService(t, db)

	credited, err := svc.CreditNext(ctx, seed.ChainID)
	if err != nil {
		t.Fatalf("credit next deposit: %v", err)
	}
	if !credited {
		t.Fatal("expected credited=true, got false")
	}

	deposit := queryDeposit(t, db, seed.DepositID)
	if deposit.Status != model.DepositStatusCredited {
		t.Fatalf("expected deposit status=%s, got %s", model.DepositStatusCredited, deposit.Status)
	}
	if deposit.CreditedAt == nil {
		t.Fatal("expected deposit credited_at to be not nil")
	}

	ledgers := queryDepositCreditLedgers(t, db, seed.DepositID)
	if len(ledgers) != 1 {
		t.Fatalf("expected 1 deposit credit ledger, got %d", len(ledgers))
	}

	account := queryBalanceAccount(t, db, seed.UserID, seed.ChainID)
	if account.AvailableBalance != seed.DepositAmount {
		t.Fatalf("expected available_balance=%s, got %s", seed.DepositAmount, account.AvailableBalance)
	}
}

type failingCreditDepositRepository struct {
	real service.CreditDepositRepository
	err  error
}

func (r *failingCreditDepositRepository) LockNextCreditableDeposit(
	ctx context.Context,
	chainID int64,
) (*model.Deposit, bool, error) {
	return r.real.LockNextCreditableDeposit(ctx, chainID)
}

func (r *failingCreditDepositRepository) MarkDepositCredited(
	ctx context.Context,
	depositID int64,
) error {
	return r.err
}

type failingDepositCreditTransactionRunner struct {
	db  *gorm.DB
	err error
}

func (r *failingDepositCreditTransactionRunner) WithinTransaction(
	ctx context.Context,
	fn func(repos service.DepositCreditRepositories) error,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		realDepositRepo := repo.NewDepositRepo(tx)

		repos := service.DepositCreditRepositories{
			CreditDepositRepository: &failingCreditDepositRepository{
				real: realDepositRepo,
				err:  r.err,
			},
			CreditBalanceLedgerRepository:  repo.NewBalanceLedgerRepository(tx),
			CreditBalanceAccountRepository: repo.NewBalanceAccountRepository(tx),
		}

		return fn(repos)
	})
}

func TestDepositCreditService_CreditNext_WithTestDB_WhenMarkDepositCreditedFails_RollsBackLedgerAndBalance(t *testing.T) {
	ctx := context.Background()

	db := createTempPostgresTestDB(t)
	seed := seedCreditableDepositData(t, db)

	markErr := errors.New("mark deposit credited failed")
	txRunner := &failingDepositCreditTransactionRunner{
		db:  db,
		err: markErr,
	}

	svc, err := service.NewDepositCreditService(txRunner, time.Second)
	if err != nil {
		t.Fatalf("create deposit credit service: %v", err)
	}

	credited, err := svc.CreditNext(ctx, seed.ChainID)
	if err == nil {
		t.Fatalf("expect error,got nil")
	}
	if credited {
		t.Fatal("expected credited=false, got true")
	}
	if !strings.Contains(err.Error(), "mark deposit credited") {
		t.Fatalf("expected mark deposit credited error, got %q", err.Error())
	}
	ledgers := queryDepositCreditLedgers(t, db, seed.DepositID)
	if len(ledgers) != 0 {
		t.Fatalf("expected 0 deposit credit ledgers after rollback, got %d", len(ledgers))
	}
	account := queryBalanceAccount(t, db, seed.UserID, seed.ChainID)
	if account.AvailableBalance != seed.InitialBalance {
		t.Fatalf("expected available_balance=%s after rollback, got %s", seed.InitialBalance, account.AvailableBalance)
	}
	deposit := queryDeposit(t, db, seed.DepositID)
	if deposit.Status != model.DepositStatusConfirming {
		t.Fatalf("expected deposit status=%s after rollback, got %s", model.DepositStatusConfirming, deposit.Status)
	}
	if deposit.CreditedAt != nil {
		t.Fatal("expected deposit credited_at to still be nil after rollback")
	}
}
