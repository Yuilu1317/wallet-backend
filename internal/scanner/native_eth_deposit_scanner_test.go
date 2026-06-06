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
	depositAddressRepo DepositAddressRepository,
	depositRepo DepositRepository,
	cursorRepo CursorRepository,
) *NativeETHDepositScanner {
	t.Helper()

	scanner, err := NewNativeETHDepositScanner(
		cfg,
		explorerClient,
		depositAddressRepo,
		depositRepo,
		cursorRepo,
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

	scanner := newScannerForTest(t, cfg, explorerClient, &fakeDepositAddressRepo{}, &fakeDepositRepo{}, cursorRepo)

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

	scanner := newScannerForTest(t, cfg, explorerClient, &fakeDepositAddressRepo{}, &fakeDepositRepo{}, cursorRepo)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	req := explorerClient.requests[0]
	if req.FromBlock != 101 {
		t.Fatalf("expected from block 101, got %d", req.FromBlock)
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

	scanner := newScannerForTest(t, cfg, explorerClient, &fakeDepositAddressRepo{}, &fakeDepositRepo{}, cursorRepo)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
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

	scanner := newScannerForTest(t, cfg, explorerClient, &fakeDepositAddressRepo{}, &fakeDepositRepo{}, cursorRepo)

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "unexpected block number") {
		t.Fatalf("expected unexpected block number error, got %q", err.Error())
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

	scanner := newScannerForTest(t, cfg, explorerClient, &fakeDepositAddressRepo{}, &fakeDepositRepo{}, cursorRepo)

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "block continuity check failed") {
		t.Fatalf("expected continuity error, got %q", err.Error())
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
							TxHash:        "0xtx100",
							FromAddress:   "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
							ToAddress:     toAddress,
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

	scanner := newScannerForTest(t, cfg, explorerClient, depositAddressRepo, depositRepo, cursorRepo)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
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

	if len(cursorRepo.upserts) != 1 {
		t.Fatalf("expected 1 cursor upsert, got %d", len(cursorRepo.upserts))
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

	scanner := newScannerForTest(t, cfg, explorerClient, depositAddressRepo, depositRepo, cursorRepo)

	err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
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

	scanner := newScannerForTest(t, cfg, explorerClient, &fakeDepositAddressRepo{}, &fakeDepositRepo{}, cursorRepo)

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "compare amount wei") {
		t.Fatalf("expected compare amount wei error, got %q", err.Error())
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

	scanner := newScannerForTest(t, cfg, explorerClient, &fakeDepositAddressRepo{}, &fakeDepositRepo{}, &fakeCursorRepo{})

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "list completed blocks returned nil response") {
		t.Fatalf("expected nil response error, got %q", err.Error())
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

	scanner := newScannerForTest(t, cfg, explorerClient, &fakeDepositAddressRepo{}, &fakeDepositRepo{}, &fakeCursorRepo{})

	err := scanner.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "unexpected response chain_id") {
		t.Fatalf("expected unexpected response chain_id error, got %q", err.Error())
	}
}

func TestIsAmountAtLeast(t *testing.T) {
	tests := []struct {
		name    string
		amount  string
		min     string
		want    bool
		wantErr string
	}{
		{
			name:   "amount greater than min",
			amount: "100",
			min:    "10",
			want:   true,
		},
		{
			name:   "amount equal min",
			amount: "10",
			min:    "10",
			want:   true,
		},
		{
			name:   "amount less than min",
			amount: "9",
			min:    "10",
			want:   false,
		},
		{
			name:    "invalid amount",
			amount:  "abc",
			min:     "10",
			wantErr: "invalid amount_wei",
		},
		{
			name:    "negative amount",
			amount:  "-1",
			min:     "10",
			wantErr: "amount_wei must be non-negative",
		},
		{
			name:    "invalid min",
			amount:  "10",
			min:     "abc",
			wantErr: "invalid min_wei",
		},
		{
			name:    "zero min",
			amount:  "10",
			min:     "0",
			wantErr: "min_wei must be positive",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			got, err := isAmountAtLeast(tt.amount, tt.min)

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

			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
