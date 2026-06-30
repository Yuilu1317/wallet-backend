package scanner_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/db/repo"
	"github.com/Yuilu1317/wallet-backend/internal/explorer"
	"github.com/Yuilu1317/wallet-backend/internal/model"
	"github.com/Yuilu1317/wallet-backend/internal/scanner"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func replacePostgresDSNDatabase(t *testing.T, dsn string, dbName string) string {
	t.Helper()

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse url dsn: %s", err)
		}
		u.Path = "/" + dbName
		return u.String()
	}

	paths := strings.Fields(dsn)
	replace := false

	for i, path := range paths {
		if strings.HasPrefix(path, "dbname=") {
			paths[i] = "dbname=" + dbName
			replace = true
			break
		}
	}
	if !replace {
		paths = append(paths, "dbname="+dbName)
	}
	return strings.Join(paths, " ")
}

func migrateTestTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlByte, err := os.ReadFile("../../migrations/001_init_schema.sql")
	if err != nil {
		t.Fatalf("read migration schema: %v", err)
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

	t.Cleanup(func() {
		_ = testSQLDB.Close()

		_ = adminDB.Exec(`
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = ?
AND pid <> pg_backend_pid()`,
			testDBName).Error

		if err := adminDB.Exec(`DROP DATABASE IF EXISTS "` + testDBName + `"`).Error; err != nil {
			t.Fatalf("drop test db%s: %v", testDBName, err)
		}
	})
	migrateTestTables(t, testDB)
	return testDB
}

func TestCreateTempPostgresTestDB_MigratesTable(t *testing.T) {
	db := createTempPostgresTestDB(t)
	var exists bool
	if err := db.Raw(`
SELECT EXISTS(
SELECT 1
FROM information_schema.tables
WHERE table_schema = 'public'
AND table_name = 'deposit_addresses'
)
`).Scan(&exists).Error; err != nil {
		t.Fatalf("check deposit_addresses table exists: %v", err)
	}
	if !exists {
		t.Fatal("expect deposit_addresses table to exists after migrating")
	}
}

type scanOnceSeedData struct {
	UserID           int64
	ChainID          int64
	DepositAddressID int64
	DepositAddress   string
}

const chainID int64 = 11155111

func seedScanOnceTestData(t *testing.T, db *gorm.DB) scanOnceSeedData {
	t.Helper()

	user := &model.User{
		Status: model.UserStatusActive,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

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
	return scanOnceSeedData{
		UserID:           user.ID,
		ChainID:          chainID,
		DepositAddressID: depositAddress.ID,
		DepositAddress:   depositAddress.Address,
	}
}

func TestSeeScanOnceTestData(t *testing.T) {
	db := createTempPostgresTestDB(t)
	seed := seedScanOnceTestData(t, db)

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
}

type fakeExplorerClient struct {
	SyncStatusResponse          *explorer.SyncStatusResponse
	ListCompletedBlocksResponse *explorer.ListCompletedBlocksResponse
	err                         error

	GetSyncStatusCalls       int
	ListCompletedBlocksCalls int
}

func (c *fakeExplorerClient) GetSyncStatus(ctx context.Context, chainID int64) (*explorer.SyncStatusResponse, error) {
	c.GetSyncStatusCalls++
	if c.err != nil {
		return nil, c.err
	}
	return c.SyncStatusResponse, nil
}

func (c *fakeExplorerClient) ListCompletedBlocks(
	ctx context.Context,
	req explorer.ListCompletedBlocksRequest,
) (*explorer.ListCompletedBlocksResponse, error) {
	c.ListCompletedBlocksCalls++
	if c.err != nil {
		return nil, c.err
	}
	return c.ListCompletedBlocksResponse, nil
}

type nativeETHDepositScannerTestOptions struct {
	StartBlock        int64
	BatchSize         int
	MinDepositWei     string
	ConfirmationDepth int64

	LatestCompletedBlock int64

	BlockNumber int64
	BlockHash   string
	ParentHash  string

	TxHash        string
	FromAddress   string
	ToAddress     string
	AmountWei     string
	ReceiptStatus int16
}

const scannerName = "native_eth_deposit_scanner"

func defaultNativeETHDepositScannerTestOptions(seed scanOnceSeedData) nativeETHDepositScannerTestOptions {
	return nativeETHDepositScannerTestOptions{
		StartBlock:        100,
		BatchSize:         1,
		MinDepositWei:     "1",
		ConfirmationDepth: 2,

		LatestCompletedBlock: 102,

		BlockNumber: 100,
		BlockHash:   "0xblock100",
		ParentHash:  "0xblock99",

		TxHash:        "0xtx100",
		FromAddress:   "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ToAddress:     seed.DepositAddress,
		AmountWei:     "1000",
		ReceiptStatus: 1,
	}
}

func newTestNativeETHDepositScanner(
	t *testing.T,
	db *gorm.DB,
	seed scanOnceSeedData,
	opts nativeETHDepositScannerTestOptions,
) (*scanner.NativeETHDepositScanner, *fakeExplorerClient) {
	t.Helper()

	client := &fakeExplorerClient{
		SyncStatusResponse: &explorer.SyncStatusResponse{
			ChainID:    seed.ChainID,
			SyncTarget: "native_eth_deposit",
			LatestCompletedBlock: &explorer.CompletedBlockSummary{
				Number: opts.LatestCompletedBlock,
				Hash:   "0xblock102",
			},
		},
		ListCompletedBlocksResponse: &explorer.ListCompletedBlocksResponse{
			ChainID: seed.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     opts.BlockNumber,
					Hash:       opts.BlockHash,
					ParentHash: opts.ParentHash,
					Transactions: []explorer.CompletedTransaction{
						{
							TxHash:        opts.TxHash,
							FromAddress:   opts.FromAddress,
							ToAddress:     opts.ToAddress,
							AmountWei:     opts.AmountWei,
							ReceiptStatus: opts.ReceiptStatus,
						},
					},
				},
			},
		},
	}

	scannerCursorRepo := repo.NewScannerCursorRepo(db)
	scannerTxRunner := repo.NewScannerTransactionRunner(db)
	nativeETHDepositScanner, err := scanner.NewNativeETHDepositScanner(
		scanner.Config{
			ChainID:           seed.ChainID,
			ScannerName:       scannerName,
			StartBlock:        opts.StartBlock,
			BatchSize:         opts.BatchSize,
			MinDepositWei:     opts.MinDepositWei,
			ConfirmationDepth: opts.ConfirmationDepth,
			DBTimeout:         time.Second,
		},
		client,
		scannerCursorRepo,
		scannerTxRunner,
	)
	if err != nil {
		t.Fatalf("create Test native eth deposit scanner: %v", err)
	}
	return nativeETHDepositScanner, client
}

func countDepositsByTxHash(t *testing.T, db *gorm.DB, chainID int64, txHash string) int64 {
	t.Helper()

	var count int64
	if err := db.
		Model(&model.Deposit{}).
		Where("chain_id = ? AND tx_hash = ?", chainID, txHash).
		Count(&count).Error; err != nil {
		t.Fatalf("count deposits: %v", err)
	}

	return count
}

func queryScannerCursorCount(t *testing.T, db *gorm.DB, chainID int64) int64 {
	t.Helper()

	var count int64
	if err := db.
		Model(&model.WalletScannerCursor{}).
		Where("chain_id = ? AND scanner_name = ?", chainID, scannerName).
		Count(&count).Error; err != nil {
		t.Fatalf("count scanner cursor: %v", err)
	}

	return count
}

func queryScannerCursor(t *testing.T, db *gorm.DB, chainID int64) model.WalletScannerCursor {
	t.Helper()

	var cursor model.WalletScannerCursor
	if err := db.
		Where("chain_id = ? AND scanner_name = ?", chainID, scannerName).
		First(&cursor).Error; err != nil {
		t.Fatalf("query scanner cursor: %v", err)
	}

	return cursor
}

func TestDepositScanner_ScanOnce_WithTestDB_ScanOnce(t *testing.T) {
	ctx := context.Background()
	db := createTempPostgresTestDB(t)
	seed := seedScanOnceTestData(t, db)

	opts := defaultNativeETHDepositScannerTestOptions(seed)
	svc, _ := newTestNativeETHDepositScanner(t, db, seed, opts)

	if err := svc.ScanOnce(ctx); err != nil {
		t.Fatalf("scan once: %v", err)
	}

	cursor := queryScannerCursor(t, db, seed.ChainID)
	if cursor.LastScannedBlockNumber != 100 {
		t.Fatalf("expect last scanned block number=100, got %d", cursor.LastScannedBlockNumber)
	}

	var deposits []model.Deposit
	if err := db.
		Where("chain_id = ? AND tx_hash = ?", seed.ChainID, "0xtx100").
		Find(&deposits).Error; err != nil {
		t.Fatalf("query deposit: %s", err)
	}
	if len(deposits) != 1 {
		t.Fatalf("expect 1 scanned deposit, got %d", len(deposits))
	}

	deposit := deposits[0]

	if deposit.DepositAddressID != seed.DepositAddressID {
		t.Fatalf("expect deposit address id=%d, got %d", seed.DepositAddressID, deposit.DepositAddressID)
	}
	if deposit.ToAddress != seed.DepositAddress {
		t.Fatalf("expect deposit address=%s, got %s", seed.DepositAddress, deposit.ToAddress)
	}
	if deposit.ChainID != seed.ChainID {
		t.Fatalf("expect deposit chain id=%d, got %d", seed.ChainID, deposit.ChainID)
	}
	if deposit.UserID != seed.UserID {
		t.Fatalf("expect deposit user id=%d, got %d", seed.UserID, deposit.UserID)
	}
	if deposit.TxHash != "0xtx100" {
		t.Fatalf("expect tx_hash=0xtx100, got %s", deposit.TxHash)
	}
	if deposit.BlockNumber != 100 {
		t.Fatalf("expect block_number=100, got %d", deposit.BlockNumber)
	}
	if deposit.BlockHash != "0xblock100" {
		t.Fatalf("expect block_hash=0xblock100, got %s", deposit.BlockHash)
	}
	if deposit.FromAddress != "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expect from_address=0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa, got %s", deposit.FromAddress)
	}
	if deposit.AmountWei != "1000" {
		t.Fatalf("expect amount_wei=1000, got %s", deposit.AmountWei)
	}
	if deposit.ReceiptStatus != 1 {
		t.Fatalf("expect receipt_status=1, got %d", deposit.ReceiptStatus)
	}
	if deposit.Status != model.DepositStatusConfirming {
		t.Fatalf("expect deposit status=%s, got %s", model.DepositStatusConfirming, deposit.Status)
	}
	if deposit.CreditedAt != nil {
		t.Fatal("expect deposit credited_at to be nil")
	}
}

func TestNativeETHDepositScanner_ScanOnce_WithTestDB_DoesNotCreateDuplicateDeposit(t *testing.T) {
	ctx := context.Background()

	db := createTempPostgresTestDB(t)
	seed := seedScanOnceTestData(t, db)

	opts := defaultNativeETHDepositScannerTestOptions(seed)

	existingDeposit := &model.Deposit{
		UserID:           seed.UserID,
		ChainID:          seed.ChainID,
		DepositAddressID: seed.DepositAddressID,
		TxHash:           opts.TxHash,
		BlockNumber:      opts.BlockNumber,
		BlockHash:        opts.BlockHash,
		FromAddress:      opts.FromAddress,
		ToAddress:        opts.ToAddress,
		AmountWei:        opts.AmountWei,
		Status:           model.DepositStatusConfirming,
		ReceiptStatus:    opts.ReceiptStatus,
	}
	if err := db.Create(existingDeposit).Error; err != nil {
		t.Fatalf("create existing deposit: %v", err)
	}

	s, _ := newTestNativeETHDepositScanner(t, db, seed, opts)

	if err := s.ScanOnce(ctx); err != nil {
		t.Fatalf("scan once: %v", err)
	}

	gotDepositCount := countDepositsByTxHash(t, db, seed.ChainID, opts.TxHash)
	if gotDepositCount != 1 {
		t.Fatalf("expect deposit count=1, got %d", gotDepositCount)
	}

	cursor := queryScannerCursor(t, db, seed.ChainID)
	if cursor.LastScannedBlockNumber != 100 {
		t.Fatalf("expect last_scanned_block_number=100, got %d", cursor.LastScannedBlockNumber)
	}
}

func TestNativeETHDepositScanner_ScanOnce_WithTestDB_DoesNotCreateDepositCases(t *testing.T) {
	tests := []struct {
		name                 string
		mutate               func(seed scanOnceSeedData, opts *nativeETHDepositScannerTestOptions)
		wantDepositCount     int64
		wantCursorExists     bool
		wantLastScannedBlock int64
		wantListBlocksCalls  int
	}{
		{
			name: "non platform address",
			mutate: func(seed scanOnceSeedData, opts *nativeETHDepositScannerTestOptions) {
				opts.ToAddress = "0x2222222222222222222222222222222222222222"
			},
			wantDepositCount:     0,
			wantCursorExists:     true,
			wantLastScannedBlock: 100,
			wantListBlocksCalls:  1,
		},
		{
			name: "failed receipt status",
			mutate: func(seed scanOnceSeedData, opts *nativeETHDepositScannerTestOptions) {
				opts.ReceiptStatus = 0
			},
			wantDepositCount:     0,
			wantCursorExists:     true,
			wantLastScannedBlock: 100,
			wantListBlocksCalls:  1,
		},
		{
			name: "amount below min deposit wei",
			mutate: func(seed scanOnceSeedData, opts *nativeETHDepositScannerTestOptions) {
				opts.MinDepositWei = "1000"
				opts.AmountWei = "999"
			},
			wantDepositCount:     0,
			wantCursorExists:     true,
			wantLastScannedBlock: 100,
			wantListBlocksCalls:  1,
		},
		{
			name: "not enough confirmations",
			mutate: func(seed scanOnceSeedData, opts *nativeETHDepositScannerTestOptions) {
				opts.LatestCompletedBlock = 101
				opts.ConfirmationDepth = 2
				opts.StartBlock = 100
			},
			wantDepositCount:    0,
			wantCursorExists:    false,
			wantListBlocksCalls: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := createTempPostgresTestDB(t)
			seed := seedScanOnceTestData(t, db)
			opts := defaultNativeETHDepositScannerTestOptions(seed)
			tt.mutate(seed, &opts)
			s, client := newTestNativeETHDepositScanner(t, db, seed, opts)
			if err := s.ScanOnce(ctx); err != nil {
				t.Fatalf("scan once: %v", err)
			}
			gotDepositCount := countDepositsByTxHash(t, db, seed.ChainID, opts.TxHash)
			if gotDepositCount != tt.wantDepositCount {
				t.Fatalf("expect deposit count=%d, got %d", tt.wantDepositCount, gotDepositCount)
			}

			if client.ListCompletedBlocksCalls != tt.wantListBlocksCalls {
				t.Fatalf("expect ListCompletedBlocks calls=%d, got %d", tt.wantListBlocksCalls, client.ListCompletedBlocksCalls)
			}

			cursorCount := queryScannerCursorCount(t, db, seed.ChainID)

			if !tt.wantCursorExists {
				if cursorCount != 0 {
					t.Fatalf("expect scanner cursor not exists, got count=%d", cursorCount)
				}
				return
			}

			if cursorCount != 1 {
				t.Fatalf("expect scanner cursor count=1, got %d", cursorCount)
			}

			cursor := queryScannerCursor(t, db, seed.ChainID)
			if cursor.LastScannedBlockNumber != tt.wantLastScannedBlock {
				t.Fatalf("expect last_scanned_block_number=%d, got %d",
					tt.wantLastScannedBlock,
					cursor.LastScannedBlockNumber,
				)
			}
		})
	}
}

type fakeCursorRepository struct {
	real scanner.CursorRepository
	err  error
}

func (r *fakeCursorRepository) GetByChainIDAndScannerName(
	ctx context.Context,
	chainID int64,
	scannerName string,
) (*model.WalletScannerCursor, bool, error) {
	return r.real.GetByChainIDAndScannerName(ctx, chainID, scannerName)
}

func (r *fakeCursorRepository) UpsertAfterBlockProcessed(
	ctx context.Context,
	cursor *model.WalletScannerCursor,
) error {
	return r.err
}

type fakeScannerTransactionRunner struct {
	db  *gorm.DB
	err error
}

func (r *fakeScannerTransactionRunner) WithinTransaction(
	ctx context.Context,
	fn func(repos scanner.Repositories) error,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		realCursorRepository := repo.NewScannerCursorRepo(tx)
		repos := scanner.Repositories{
			DepositAddressRepo: repo.NewDepositAddressRepo(tx),
			DepositRepo:        repo.NewDepositRepo(tx),
			CursorRepo: &fakeCursorRepository{
				real: realCursorRepository,
				err:  r.err,
			},
		}
		return fn(repos)
	})
}

func newTestNativeETHDepositScannerWithTxError(
	t *testing.T,
	db *gorm.DB,
	seed scanOnceSeedData,
	opts nativeETHDepositScannerTestOptions,
) (*scanner.NativeETHDepositScanner, *fakeExplorerClient) {
	t.Helper()

	client := &fakeExplorerClient{
		SyncStatusResponse: &explorer.SyncStatusResponse{
			ChainID:    seed.ChainID,
			SyncTarget: "native_eth_deposit",
			LatestCompletedBlock: &explorer.CompletedBlockSummary{
				Number: opts.LatestCompletedBlock,
				Hash:   "0xblock102",
			},
		},
		ListCompletedBlocksResponse: &explorer.ListCompletedBlocksResponse{
			ChainID: seed.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     opts.BlockNumber,
					Hash:       opts.BlockHash,
					ParentHash: opts.ParentHash,
					Transactions: []explorer.CompletedTransaction{
						{
							TxHash:        opts.TxHash,
							FromAddress:   opts.FromAddress,
							ToAddress:     opts.ToAddress,
							AmountWei:     opts.AmountWei,
							ReceiptStatus: opts.ReceiptStatus,
						},
					},
				},
			},
		},
	}

	scannerCursorRepo := repo.NewScannerCursorRepo(db)
	upsertErr := errors.New("upsert after block processed failed")
	txRunner := &fakeScannerTransactionRunner{
		db:  db,
		err: upsertErr,
	}
	nativeETHDepositScanner, err := scanner.NewNativeETHDepositScanner(
		scanner.Config{
			ChainID:           seed.ChainID,
			ScannerName:       scannerName,
			StartBlock:        opts.StartBlock,
			BatchSize:         opts.BatchSize,
			MinDepositWei:     opts.MinDepositWei,
			ConfirmationDepth: opts.ConfirmationDepth,
			DBTimeout:         time.Second,
		},
		client,
		scannerCursorRepo,
		txRunner,
	)
	if err != nil {
		t.Fatalf("create Test native eth deposit scanner: %v", err)
	}
	return nativeETHDepositScanner, client
}

func TestNativeETHDepositScanner_ScanOnce_WithTestDB_WhenUpdateCursorFails_RollsBackDeposit(t *testing.T) {
	ctx := context.Background()
	db := createTempPostgresTestDB(t)
	seed := seedScanOnceTestData(t, db)
	opts := defaultNativeETHDepositScannerTestOptions(seed)
	s, client := newTestNativeETHDepositScannerWithTxError(t, db, seed, opts)
	err := s.ScanOnce(ctx)
	if err == nil {
		t.Fatal("expected scan once error, got nil")
	}
	if !strings.Contains(err.Error(), "upsert after block processed failed") {
		t.Fatalf("expected upsert cursor error, got %q", err.Error())
	}

	gotDepositCount := countDepositsByTxHash(t, db, seed.ChainID, opts.TxHash)
	if gotDepositCount != 0 {
		t.Fatalf("expect deposit count=0 after rollback, got %d", gotDepositCount)
	}
	cursorCount := queryScannerCursorCount(t, db, seed.ChainID)
	if cursorCount != 0 {
		t.Fatalf("expect scanner cursor count=0 after rollback, got %d", cursorCount)
	}
	if client.ListCompletedBlocksCalls != 1 {
		t.Fatalf("expect ListCompletedBlocks calls=1, got %d", client.ListCompletedBlocksCalls)
	}
}
