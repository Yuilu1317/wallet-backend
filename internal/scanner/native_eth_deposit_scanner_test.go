package scanner

import (
	"context"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/explorer"
	"github.com/Yuilu1317/wallet-backend/internal/model"
)

func newScannerForTest(
	t *testing.T,
	cfg Config,
	explorerClient explorer.Client,
	cursorRepo CursorRepository,
	txRunner TransactionRunner,
) *NativeETHDepositScanner {
	t.Helper()

	scanner, err := NewNativeETHDepositScanner(
		cfg,
		explorerClient,
		cursorRepo,
		txRunner,
	)
	if err != nil {
		t.Fatalf("new native eth deposit scanner: %v", err)
	}

	return scanner
}

func validScannerConfig() Config {
	return Config{
		ChainID:       11155111,
		ScannerName:   "native_eth_deposit_scanner",
		StartBlock:    100,
		BatchSize:     10,
		MinDepositWei: "10",
	}
}

type fakeExplorerClient struct {
	resp     *explorer.ListCompletedBlocksResponse
	err      error
	requests []explorer.ListCompletedBlocksRequest
}

func (f *fakeExplorerClient) ListCompletedBlocks(
	ctx context.Context,
	req explorer.ListCompletedBlocksRequest,
) (*explorer.ListCompletedBlocksResponse, error) {
	f.requests = append(f.requests, req)

	if f.err != nil {
		return nil, f.err
	}

	return f.resp, nil
}

type fakeDepositAddressRepo struct {
	byAddress map[string]*model.DepositAddress
	err       error
	calls     []string
}

func (f *fakeDepositAddressRepo) FindActiveByChainIDAndAddressLower(
	ctx context.Context,
	chainID int64,
	addressLower string,
) (*model.DepositAddress, bool, error) {
	f.calls = append(f.calls, addressLower)

	if f.err != nil {
		return nil, false, f.err
	}

	if f.byAddress == nil {
		return nil, false, nil
	}

	address, ok := f.byAddress[addressLower]
	if !ok {
		return nil, false, nil
	}

	return address, true, nil
}

type fakeDepositRepo struct {
	deposits []*model.Deposit
	created  bool
	err      error
}

func (f *fakeDepositRepo) CreateConfirmingDepositIdempotently(
	ctx context.Context,
	deposit *model.Deposit,
) (bool, error) {
	if f.err != nil {
		return false, f.err
	}

	copied := *deposit
	f.deposits = append(f.deposits, &copied)

	return f.created, nil
}

type fakeCursorRepo struct {
	cursor *model.WalletScannerCursor
	found  bool
	err    error

	upserts   []*model.WalletScannerCursor
	upsertErr error
}

func (f *fakeCursorRepo) GetByChainIDAndScannerName(
	ctx context.Context,
	chainID int64,
	scannerName string,
) (*model.WalletScannerCursor, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}

	return f.cursor, f.found, nil
}

func (f *fakeCursorRepo) UpsertAfterBlockProcessed(
	ctx context.Context,
	cursor *model.WalletScannerCursor,
) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}

	copied := *cursor
	f.upserts = append(f.upserts, &copied)

	return nil
}

type fakeTransactionRunner struct {
	repos Repositories
	err   error
	calls int
}

func newFakeTransactionRunner(
	depositAddressRepo DepositAddressRepository,
	depositRepo DepositRepository,
	cursorRepo CursorRepository,
) *fakeTransactionRunner {
	return &fakeTransactionRunner{
		repos: Repositories{
			DepositAddressRepo: depositAddressRepo,
			DepositRepo:        depositRepo,
			CursorRepo:         cursorRepo,
		},
	}
}

func (f *fakeTransactionRunner) WithinTransaction(
	ctx context.Context,
	fn func(repos Repositories) error,
) error {
	f.calls++

	if f.err != nil {
		return f.err
	}

	return fn(f.repos)
}

func TestNativeETHDepositScanner_ScanOnce_WithoutCursorRequestsStartBlockAndUpdatesCursor(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     100,
					Hash:       "0xblock100",
					ParentHash: "0xblock99",
				},
			},
		},
	}

	cursorRepo := &fakeCursorRepo{
		found: false,
	}

	txRunner := newFakeTransactionRunner(
		&fakeDepositAddressRepo{},
		&fakeDepositRepo{},
		cursorRepo,
	)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(explorerClient.requests) != 1 {
		t.Fatalf("expected 1 explorer request, got %d", len(explorerClient.requests))
	}

	req := explorerClient.requests[0]
	if req.FromBlock != cfg.StartBlock {
		t.Fatalf("expected from block %d, got %d", cfg.StartBlock, req.FromBlock)
	}

	if req.Limit != cfg.BatchSize {
		t.Fatalf("expected limit %d, got %d", cfg.BatchSize, req.Limit)
	}

	if txRunner.calls != 1 {
		t.Fatalf("expected 1 transaction call, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 1 {
		t.Fatalf("expected 1 cursor upsert, got %d", len(cursorRepo.upserts))
	}

	if cursorRepo.upserts[0].LastScannedBlockNumber != 100 {
		t.Fatalf("expected cursor block 100, got %d", cursorRepo.upserts[0].LastScannedBlockNumber)
	}
}

func TestNativeETHDepositScanner_ScanOnce_WithCursorRequestsNextBlock(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     101,
					Hash:       "0xblock101",
					ParentHash: "0xblock100",
				},
			},
		},
	}

	cursorRepo := &fakeCursorRepo{
		cursor: &model.WalletScannerCursor{
			ChainID:                cfg.ChainID,
			ScannerName:            cfg.ScannerName,
			LastScannedBlockNumber: 100,
			LastScannedBlockHash:   "0xblock100",
		},
		found: true,
	}

	txRunner := newFakeTransactionRunner(
		&fakeDepositAddressRepo{},
		&fakeDepositRepo{},
		cursorRepo,
	)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	req := explorerClient.requests[0]
	if req.FromBlock != 101 {
		t.Fatalf("expected from block 101, got %d", req.FromBlock)
	}

	if txRunner.calls != 1 {
		t.Fatalf("expected 1 transaction call, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 1 {
		t.Fatalf("expected 1 cursor upsert, got %d", len(cursorRepo.upserts))
	}

	if cursorRepo.upserts[0].LastScannedBlockNumber != 101 {
		t.Fatalf("expected cursor block 101, got %d", cursorRepo.upserts[0].LastScannedBlockNumber)
	}
}

func TestNativeETHDepositScanner_ScanOnce_SortsBlocksAndProcessesSequentially(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     101,
					Hash:       "0xblock101",
					ParentHash: "0xblock100",
				},
				{
					Number:     100,
					Hash:       "0xblock100",
					ParentHash: "0xblock99",
				},
			},
		},
	}

	cursorRepo := &fakeCursorRepo{}
	txRunner := newFakeTransactionRunner(
		&fakeDepositAddressRepo{},
		&fakeDepositRepo{},
		cursorRepo,
	)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if txRunner.calls != 2 {
		t.Fatalf("expected 2 transaction calls, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 2 {
		t.Fatalf("expected 2 cursor upserts, got %d", len(cursorRepo.upserts))
	}

	if cursorRepo.upserts[0].LastScannedBlockNumber != 100 {
		t.Fatalf("expected first upsert block 100, got %d", cursorRepo.upserts[0].LastScannedBlockNumber)
	}

	if cursorRepo.upserts[1].LastScannedBlockNumber != 101 {
		t.Fatalf("expected second upsert block 101, got %d", cursorRepo.upserts[1].LastScannedBlockNumber)
	}
}

func TestNativeETHDepositScanner_ScanOnce_MissingBlockReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     100,
					Hash:       "0xblock100",
					ParentHash: "0xblock99",
				},
				{
					Number:     102,
					Hash:       "0xblock102",
					ParentHash: "0xblock101",
				},
			},
		},
	}

	cursorRepo := &fakeCursorRepo{}
	txRunner := newFakeTransactionRunner(
		&fakeDepositAddressRepo{},
		&fakeDepositRepo{},
		cursorRepo,
	)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "unexpected block number") {
		t.Fatalf("expected unexpected block number error, got %q", err.Error())
	}

	if txRunner.calls != 1 {
		t.Fatalf("expected 1 transaction call before missing block, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 1 {
		t.Fatalf("expected cursor upsert for processed block 100, got %d", len(cursorRepo.upserts))
	}

	if cursorRepo.upserts[0].LastScannedBlockNumber != 100 {
		t.Fatalf("expected cursor block 100, got %d", cursorRepo.upserts[0].LastScannedBlockNumber)
	}
}

func TestNativeETHDepositScanner_ScanOnce_ParentHashMismatchReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     101,
					Hash:       "0xblock101",
					ParentHash: "0xwrong",
				},
			},
		},
	}

	cursorRepo := &fakeCursorRepo{
		cursor: &model.WalletScannerCursor{
			ChainID:                cfg.ChainID,
			ScannerName:            cfg.ScannerName,
			LastScannedBlockNumber: 100,
			LastScannedBlockHash:   "0xblock100",
		},
		found: true,
	}

	txRunner := newFakeTransactionRunner(
		&fakeDepositAddressRepo{},
		&fakeDepositRepo{},
		cursorRepo,
	)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "block continuity check failed") {
		t.Fatalf("expected continuity error, got %q", err.Error())
	}

	if txRunner.calls != 0 {
		t.Fatalf("expected no transaction call, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 0 {
		t.Fatalf("expected no cursor upsert, got %d", len(cursorRepo.upserts))
	}
}

func TestNativeETHDepositScanner_ScanOnce_CreatesDepositForMatchingTransaction(t *testing.T) {
	cfg := validScannerConfig()

	toAddress := "0x1111111111111111111111111111111111111111"
	toAddressLower := strings.ToLower(toAddress)

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     100,
					Hash:       "0xblock100",
					ParentHash: "0xblock99",
					Transactions: []explorer.CompletedTransaction{
						{
							TxHash:        " 0xTX100 ",
							FromAddress:   "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
							ToAddress:     toAddress,
							AmountWei:     " 100 ",
							ReceiptStatus: 1,
						},
					},
				},
			},
		},
	}

	depositAddressRepo := &fakeDepositAddressRepo{
		byAddress: map[string]*model.DepositAddress{
			toAddressLower: {
				ID:           7,
				UserID:       42,
				ChainID:      cfg.ChainID,
				Address:      toAddress,
				AddressLower: toAddressLower,
				Status:       model.DepositAddressStatusActive,
			},
		},
	}

	depositRepo := &fakeDepositRepo{
		created: true,
	}

	cursorRepo := &fakeCursorRepo{}
	txRunner := newFakeTransactionRunner(depositAddressRepo, depositRepo, cursorRepo)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if txRunner.calls != 1 {
		t.Fatalf("expected 1 transaction call, got %d", txRunner.calls)
	}

	if len(depositRepo.deposits) != 1 {
		t.Fatalf("expected 1 deposit, got %d", len(depositRepo.deposits))
	}

	got := depositRepo.deposits[0]

	if got.UserID != 42 {
		t.Fatalf("expected user id 42, got %d", got.UserID)
	}

	if got.DepositAddressID != 7 {
		t.Fatalf("expected deposit address id 7, got %d", got.DepositAddressID)
	}

	if got.TxHash != "0xtx100" {
		t.Fatalf("expected tx hash 0xtx100, got %q", got.TxHash)
	}

	if got.Status != model.DepositStatusConfirming {
		t.Fatalf("expected status %q, got %q", model.DepositStatusConfirming, got.Status)
	}

	if got.ReceiptStatus != 1 {
		t.Fatalf("expected receipt status 1, got %d", got.ReceiptStatus)
	}

	if got.ToAddress != toAddressLower {
		t.Fatalf("expected to address %q, got %q", toAddressLower, got.ToAddress)
	}

	if got.AmountWei != "100" {
		t.Fatalf("expected amount wei 100, got %q", got.AmountWei)
	}

	if got.FromAddress != "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected normalized from address, got %q", got.FromAddress)
	}

	if len(cursorRepo.upserts) != 1 {
		t.Fatalf("expected 1 cursor upsert, got %d", len(cursorRepo.upserts))
	}

	if cursorRepo.upserts[0].LastScannedBlockNumber != 100 {
		t.Fatalf("expected cursor block 100, got %d", cursorRepo.upserts[0].LastScannedBlockNumber)
	}
}

func TestNativeETHDepositScanner_ScanOnce_SkipsNonDepositTransactions(t *testing.T) {
	cfg := validScannerConfig()

	toAddress := "0x1111111111111111111111111111111111111111"
	toAddressLower := strings.ToLower(toAddress)

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     100,
					Hash:       "0xblock100",
					ParentHash: "0xblock99",
					Transactions: []explorer.CompletedTransaction{
						{
							TxHash:        "0xfailed",
							FromAddress:   "0xaaa",
							ToAddress:     toAddress,
							AmountWei:     "100",
							ReceiptStatus: 0,
						},
						{
							TxHash:        "0xsmall",
							FromAddress:   "0xaaa",
							ToAddress:     toAddress,
							AmountWei:     "9",
							ReceiptStatus: 1,
						},
						{
							TxHash:        "0xnot-platform-address",
							FromAddress:   "0xaaa",
							ToAddress:     "0x2222222222222222222222222222222222222222",
							AmountWei:     "100",
							ReceiptStatus: 1,
						},
					},
				},
			},
		},
	}

	depositAddressRepo := &fakeDepositAddressRepo{
		byAddress: map[string]*model.DepositAddress{
			toAddressLower: {
				ID:           7,
				UserID:       42,
				ChainID:      cfg.ChainID,
				AddressLower: toAddressLower,
				Status:       model.DepositAddressStatusActive,
			},
		},
	}

	depositRepo := &fakeDepositRepo{}
	cursorRepo := &fakeCursorRepo{}
	txRunner := newFakeTransactionRunner(depositAddressRepo, depositRepo, cursorRepo)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if txRunner.calls != 1 {
		t.Fatalf("expected 1 transaction call, got %d", txRunner.calls)
	}

	if len(depositRepo.deposits) != 0 {
		t.Fatalf("expected no deposits, got %d", len(depositRepo.deposits))
	}

	if len(cursorRepo.upserts) != 1 {
		t.Fatalf("expected cursor update after block processed, got %d", len(cursorRepo.upserts))
	}
}

func TestNativeETHDepositScanner_ScanOnce_InvalidAmountReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     100,
					Hash:       "0xblock100",
					ParentHash: "0xblock99",
					Transactions: []explorer.CompletedTransaction{
						{
							TxHash:        "0xinvalid",
							FromAddress:   "0xaaa",
							ToAddress:     "0x1111111111111111111111111111111111111111",
							AmountWei:     "abc",
							ReceiptStatus: 1,
						},
					},
				},
			},
		},
	}

	cursorRepo := &fakeCursorRepo{}
	txRunner := newFakeTransactionRunner(
		&fakeDepositAddressRepo{},
		&fakeDepositRepo{},
		cursorRepo,
	)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "parse amount_wei") {
		t.Fatalf("expected parse amount_wei error, got %q", err.Error())
	}

	if txRunner.calls != 1 {
		t.Fatalf("expected 1 transaction call, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 0 {
		t.Fatalf("expected no cursor update after failed block processing, got %d", len(cursorRepo.upserts))
	}
}

func TestNativeETHDepositScanner_ScanOnce_NilResponseReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		resp: nil,
	}

	cursorRepo := &fakeCursorRepo{}
	txRunner := newFakeTransactionRunner(
		&fakeDepositAddressRepo{},
		&fakeDepositRepo{},
		cursorRepo,
	)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "list completed blocks returned nil response") {
		t.Fatalf("expected nil response error, got %q", err.Error())
	}

	if txRunner.calls != 0 {
		t.Fatalf("expected no transaction call, got %d", txRunner.calls)
	}
}

func TestNativeETHDepositScanner_ScanOnce_UnexpectedChainIDReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: 1,
			Blocks:  nil,
		},
	}

	cursorRepo := &fakeCursorRepo{}
	txRunner := newFakeTransactionRunner(
		&fakeDepositAddressRepo{},
		&fakeDepositRepo{},
		cursorRepo,
	)

	scanner := newScannerForTest(t, cfg, explorerClient, cursorRepo, txRunner)

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "unexpected response chain_id") {
		t.Fatalf("expected unexpected response chain_id error, got %q", err.Error())
	}

	if txRunner.calls != 0 {
		t.Fatalf("expected no transaction call, got %d", txRunner.calls)
	}
}

func TestNewNativeETHDepositScanner_InvalidMinDepositWeiReturnsError(t *testing.T) {
	tests := []struct {
		name          string
		minDepositWei string
		wantErr       string
	}{
		{
			name:          "empty",
			minDepositWei: "",
			wantErr:       "scanner.min_deposit_wei is required",
		},
		{
			name:          "invalid integer",
			minDepositWei: "abc",
			wantErr:       "parse scanner.min_deposit_wei",
		},
		{
			name:          "zero",
			minDepositWei: "0",
			wantErr:       "value must be positive",
		},
		{
			name:          "negative",
			minDepositWei: "-1",
			wantErr:       "value must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validScannerConfig()
			cfg.MinDepositWei = tt.minDepositWei

			cursorRepo := &fakeCursorRepo{}
			txRunner := newFakeTransactionRunner(
				&fakeDepositAddressRepo{},
				&fakeDepositRepo{},
				cursorRepo,
			)

			_, err := NewNativeETHDepositScanner(
				cfg,
				&fakeExplorerClient{},
				cursorRepo,
				txRunner,
			)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestParsePositiveWei(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr string
	}{
		{
			name:  "positive",
			value: "10",
			want:  "10",
		},
		{
			name:  "trim spaces",
			value: " 10 ",
			want:  "10",
		},
		{
			name:    "empty",
			value:   "",
			wantErr: "value is required",
		},
		{
			name:    "invalid",
			value:   "abc",
			wantErr: "value must be a base-10 integer",
		},
		{
			name:    "zero",
			value:   "0",
			wantErr: "value must be positive",
		},
		{
			name:    "negative",
			value:   "-1",
			wantErr: "value must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePositiveWei(tt.value)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
				}

				return
			}

			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}

			if got.String() != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got.String())
			}
		})
	}
}

func TestParseNonNegativeWei(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr string
	}{
		{
			name:  "positive",
			value: "10",
			want:  "10",
		},
		{
			name:  "zero",
			value: "0",
			want:  "0",
		},
		{
			name:  "trim spaces",
			value: " 100 ",
			want:  "100",
		},
		{
			name:    "empty",
			value:   "",
			wantErr: "value is required",
		},
		{
			name:    "invalid",
			value:   "abc",
			wantErr: "value must be a base-10 integer",
		},
		{
			name:    "negative",
			value:   "-1",
			wantErr: "value must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNonNegativeWei(tt.value)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
				}

				return
			}

			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}

			if got.String() != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got.String())
			}
		})
	}
}
