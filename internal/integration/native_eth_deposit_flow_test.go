package integration_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/db/repo"
	"github.com/Yuilu1317/wallet-backend/internal/explorer"
	"github.com/Yuilu1317/wallet-backend/internal/integration/testutil"
	"github.com/Yuilu1317/wallet-backend/internal/model"
	"github.com/Yuilu1317/wallet-backend/internal/scanner"
	"github.com/Yuilu1317/wallet-backend/internal/service"
	"gorm.io/gorm"
)

type depositFlowData struct {
	UserID           int64
	ChainID          int64
	DepositAddressID int64
	DepositAddress   string
	InitialBalance   string
}

func seedDepositFlowData(t *testing.T, db *gorm.DB) depositFlowData {
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
	return depositFlowData{
		UserID:           user.ID,
		ChainID:          chainID,
		DepositAddressID: depositAddress.ID,
		DepositAddress:   depositAddress.Address,
		InitialBalance:   balanceAccount.AvailableBalance,
	}
}

func TestSeedDepositFlowData(t *testing.T) {
	db := testutil.CreateTempPostgresTestDB(t)
	seed := seedDepositFlowData(t, db)

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

type fakeExplorerClient struct {
	SyncStatusResponse          *explorer.SyncStatusResponse
	ListCompletedBlocksResponse *explorer.ListCompletedBlocksResponse
	Err                         error
	GetSyncStatusCalls          int
	ListCompletedBlocksCalls    int
}

func (c *fakeExplorerClient) GetSyncStatus(ctx context.Context, chainID int64) (*explorer.SyncStatusResponse, error) {
	c.GetSyncStatusCalls++
	if c.Err != nil {
		return nil, c.Err
	}
	return c.SyncStatusResponse, nil
}

func (c *fakeExplorerClient) ListCompletedBlocks(
	ctx context.Context,
	req explorer.ListCompletedBlocksRequest,
) (*explorer.ListCompletedBlocksResponse, error) {
	c.ListCompletedBlocksCalls++
	if c.Err != nil {
		return nil, c.Err
	}
	return c.ListCompletedBlocksResponse, nil
}

type depositFlowTestOptions struct {
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

func defaultDepositFlowTestOptions(seed depositFlowData) depositFlowTestOptions {
	return depositFlowTestOptions{
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

func newTestDepositFlow(
	t *testing.T,
	seed depositFlowData,
	db *gorm.DB,
	opts depositFlowTestOptions,
) (*scanner.NativeETHDepositScanner,
	*fakeExplorerClient,
	*service.DepositCreditService,
) {
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

	txRunner := repo.NewDepositCreditTransactionRunner(db)
	depositCreditService, err := service.NewDepositCreditService(txRunner, time.Second)
	if err != nil {
		t.Fatalf("create deposit credit service: %v", err)
	}

	return nativeETHDepositScanner, client, depositCreditService
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

func queryDepositsByTxHash(t *testing.T, seed depositFlowData, db *gorm.DB, txHash string) []model.Deposit {
	t.Helper()
	var deposits []model.Deposit
	if err := db.
		Where("chain_id = ? AND tx_hash = ?", seed.ChainID, txHash).
		Find(&deposits).Error; err != nil {
		t.Fatalf("query deposit: %v", err)
	}
	return deposits
}

func TestNativeETHDepositFlow_WithTestDB_ScanThenCredit(t *testing.T) {
	ctx := context.Background()
	db := testutil.CreateTempPostgresTestDB(t)
	seed := seedDepositFlowData(t, db)
	opts := defaultDepositFlowTestOptions(seed)
	nativeETHDepositScanner, client, depositCreditService := newTestDepositFlow(t, seed, db, opts)

	if err := nativeETHDepositScanner.ScanOnce(ctx); err != nil {
		t.Fatalf("scan once: %v", err)
	}

	if client.GetSyncStatusCalls != 1 {
		t.Fatalf("expect GetSyncStatus calls=1, got %d", client.GetSyncStatusCalls)
	}
	if client.ListCompletedBlocksCalls != 1 {
		t.Fatalf("expect ListCompletedBlocks calls=1, got %d", client.ListCompletedBlocksCalls)
	}

	deposits := queryDepositsByTxHash(t, seed, db, opts.TxHash)
	if len(deposits) != 1 {
		t.Fatalf("expect 1 scanned deposit, got %d", len(deposits))
	}
	scannedDeposit := deposits[0]
	if scannedDeposit.Status != model.DepositStatusConfirming {
		t.Fatalf("expect scanned deposit status=%s, got %s",
			model.DepositStatusConfirming,
			scannedDeposit.Status,
		)
	}
	if scannedDeposit.CreditedAt != nil {
		t.Fatal("expect scanned deposit credited_at to be nil before credit")
	}

	var cursor model.WalletScannerCursor
	if err := db.
		Where("chain_id = ? AND scanner_name = ?", seed.ChainID, scannerName).
		First(&cursor).Error; err != nil {
		t.Fatalf("query scanner cursor: %v", err)
	}
	if cursor.LastScannedBlockNumber != opts.BlockNumber {
		t.Fatalf("expect last_scanned_block_number=%d, got %d",
			opts.BlockNumber,
			cursor.LastScannedBlockNumber,
		)
	}

	credited, err := depositCreditService.CreditNext(ctx, seed.ChainID)
	if err != nil {
		t.Fatalf("credit next deposit: %v", err)
	}
	if !credited {
		t.Fatalf("expected credited=true, got false")
	}

	deposits = queryDepositsByTxHash(t, seed, db, opts.TxHash)
	if len(deposits) != 1 {
		t.Fatalf("expect 1 scanned deposit, got %d", len(deposits))
	}
	if deposits[0].Status != model.DepositStatusCredited {
		t.Fatalf("expect deposit status=%s, got %s", model.DepositStatusCredited, deposits[0].Status)
	}
	if deposits[0].CreditedAt == nil {
		t.Fatalf("expect deposit credited_at to be not nil")
	}

	ledgers := queryDepositCreditLedgers(t, db, deposits[0].ID)
	if len(ledgers) != 1 {
		t.Fatalf("expect 1 deposit credit ledger, got %d", len(ledgers))
	}
	ledger := ledgers[0]
	if ledger.AmountWei != deposits[0].AmountWei {
		t.Fatalf("expected ledger amount_wei=%s, got %s", deposits[0].AmountWei, ledger.AmountWei)
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

	expectedBalance := addWeiStrings(t, seed.InitialBalance, deposits[0].AmountWei)

	if account.AvailableBalance != expectedBalance {
		t.Fatalf("expected available_balance=%s, got %s", expectedBalance, account.AvailableBalance)
	}
}
