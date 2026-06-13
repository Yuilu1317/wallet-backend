package scanner

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/explorer"
	"github.com/Yuilu1317/wallet-backend/internal/model"
)

func newScannerForTest(
	t *testing.T,
	cfg Config,
	explorerClient ExplorerClient,
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
		ChainID:           11155111,
		ScannerName:       "native_eth_deposit_scanner",
		StartBlock:        100,
		BatchSize:         10,
		ConfirmationDepth: 12,
		MinDepositWei:     "10",
	}
}

func validSyncStatus(chainID int64, latestCompletedBlock int64) *explorer.SyncStatusResponse {
	return &explorer.SyncStatusResponse{
		ChainID:    chainID,
		SyncTarget: "safe",
		LatestCompletedBlock: &explorer.CompletedBlockSummary{
			Number: latestCompletedBlock,
			Hash:   "0xlatest",
		},
	}
}

type fakeExplorerClient struct {
	syncStatusResp     *explorer.SyncStatusResponse
	syncStatusErr      error
	syncStatusChainIDs []int64

	resp     *explorer.ListCompletedBlocksResponse
	err      error
	requests []explorer.ListCompletedBlocksRequest
}

func (f *fakeExplorerClient) GetSyncStatus(
	ctx context.Context,
	chainID int64,
) (*explorer.SyncStatusResponse, error) {
	f.syncStatusChainIDs = append(f.syncStatusChainIDs, chainID)

	if f.syncStatusErr != nil {
		return nil, f.syncStatusErr
	}

	return f.syncStatusResp, nil
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

	getChainIDs     []int64
	getScannerNames []string

	upserts   []*model.WalletScannerCursor
	upsertErr error
}

func (f *fakeCursorRepo) GetByChainIDAndScannerName(
	ctx context.Context,
	chainID int64,
	scannerName string,
) (*model.WalletScannerCursor, bool, error) {
	f.getChainIDs = append(f.getChainIDs, chainID)
	f.getScannerNames = append(f.getScannerNames, scannerName)

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

func completedBlock(number int64) explorer.CompletedBlock {
	return explorer.CompletedBlock{
		Number:     number,
		Hash:       fmt.Sprintf("0xblock%d", number),
		ParentHash: fmt.Sprintf("0xblock%d", number-1),
	}
}

func validScanRange() *ScanRange {
	return &ScanRange{
		FromBlock:                  100,
		Limit:                      3,
		ConfirmedTargetBlockNumber: 102,
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to contain %q, got %q", want, err.Error())
	}
}

func assertNoProcessing(t *testing.T, txRunner *fakeTransactionRunner, cursorRepo *fakeCursorRepo) {
	t.Helper()

	if txRunner.calls != 0 {
		t.Fatalf("expected no transaction call, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 0 {
		t.Fatalf("expected no cursor upsert, got %d", len(cursorRepo.upserts))
	}
}

func assertCompletedBlocksRequest(
	t *testing.T,
	explorerClient *fakeExplorerClient,
	wantChainID int64,
	wantFromBlock int64,
	wantLimit int,
) {
	t.Helper()

	if len(explorerClient.requests) != 1 {
		t.Fatalf("expected 1 completed-blocks request, got %d", len(explorerClient.requests))
	}

	req := explorerClient.requests[0]
	if req.ChainID != wantChainID {
		t.Fatalf("expected request chain id %d, got %d", wantChainID, req.ChainID)
	}

	if req.FromBlock != wantFromBlock {
		t.Fatalf("expected from block %d, got %d", wantFromBlock, req.FromBlock)
	}

	if req.Limit != wantLimit {
		t.Fatalf("expected limit %d, got %d", wantLimit, req.Limit)
	}
}

func assertCursorUpsert(
	t *testing.T,
	cursorRepo *fakeCursorRepo,
	index int,
	wantBlockNumber int64,
	wantBlockHash string,
) {
	t.Helper()

	if len(cursorRepo.upserts) <= index {
		t.Fatalf("expected cursor upsert at index %d, got %d upserts", index, len(cursorRepo.upserts))
	}

	got := cursorRepo.upserts[index]
	if got.LastScannedBlockNumber != wantBlockNumber {
		t.Fatalf("expected cursor block %d, got %d", wantBlockNumber, got.LastScannedBlockNumber)
	}

	if got.LastScannedBlockHash != wantBlockHash {
		t.Fatalf("expected cursor hash %q, got %q", wantBlockHash, got.LastScannedBlockHash)
	}
}

func TestNativeETHDepositScanner_ValidateCompletedBlocksResponse(t *testing.T) {
	cfg := validScannerConfig()

	scanner := &NativeETHDepositScanner{
		cfg: cfg,
	}

	tests := []struct {
		name      string
		resp      *explorer.ListCompletedBlocksResponse
		scanRange *ScanRange
		wantErr   string
	}{
		{
			name:      "nil response",
			resp:      nil,
			scanRange: validScanRange(),
			wantErr:   "list completed blocks returned nil response",
		},
		{
			name: "nil scan range",
			resp: &explorer.ListCompletedBlocksResponse{
				ChainID: cfg.ChainID,
				Blocks:  []explorer.CompletedBlock{},
			},
			scanRange: nil,
			wantErr:   "scan range is nil",
		},
		{
			name: "unexpected chain id",
			resp: &explorer.ListCompletedBlocksResponse{
				ChainID: 1,
				Blocks:  []explorer.CompletedBlock{},
			},
			scanRange: validScanRange(),
			wantErr:   "unexpected response chain_id",
		},
		{
			name: "response exceeds requested limit",
			resp: &explorer.ListCompletedBlocksResponse{
				ChainID: cfg.ChainID,
				Blocks: []explorer.CompletedBlock{
					completedBlock(100),
					completedBlock(101),
					completedBlock(102),
					completedBlock(103),
				},
			},
			scanRange: validScanRange(),
			wantErr:   "completed blocks response exceeds requested limit",
		},
		{
			name: "block exceeds confirmed target",
			resp: &explorer.ListCompletedBlocksResponse{
				ChainID: cfg.ChainID,
				Blocks: []explorer.CompletedBlock{
					completedBlock(103),
				},
			},
			scanRange: validScanRange(),
			wantErr:   "completed block exceeds confirmed target",
		},
		{
			name: "empty blocks is valid",
			resp: &explorer.ListCompletedBlocksResponse{
				ChainID: cfg.ChainID,
				Blocks:  []explorer.CompletedBlock{},
			},
			scanRange: validScanRange(),
		},
		{
			name: "valid response",
			resp: &explorer.ListCompletedBlocksResponse{
				ChainID: cfg.ChainID,
				Blocks: []explorer.CompletedBlock{
					completedBlock(100),
					completedBlock(101),
					completedBlock(102),
				},
			},
			scanRange: validScanRange(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			err := scanner.validateCompletedBlocksResponse(tt.resp, tt.scanRange)

			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}

			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestNativeETHDepositScanner_ScanOnce_WithoutCursorRequestsStartBlockAndUpdatesCursor(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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

	if len(explorerClient.syncStatusChainIDs) != 1 {
		t.Fatalf("expected 1 sync status request, got %d", len(explorerClient.syncStatusChainIDs))
	}

	if explorerClient.syncStatusChainIDs[0] != cfg.ChainID {
		t.Fatalf("expected sync status chain id %d, got %d", cfg.ChainID, explorerClient.syncStatusChainIDs[0])
	}

	if len(cursorRepo.getChainIDs) != 1 {
		t.Fatalf("expected 1 cursor lookup, got %d", len(cursorRepo.getChainIDs))
	}

	if cursorRepo.getChainIDs[0] != cfg.ChainID {
		t.Fatalf("expected cursor lookup chain id %d, got %d", cfg.ChainID, cursorRepo.getChainIDs[0])
	}

	if cursorRepo.getScannerNames[0] != cfg.ScannerName {
		t.Fatalf("expected scanner name %q, got %q", cfg.ScannerName, cursorRepo.getScannerNames[0])
	}

	if len(explorerClient.requests) != 1 {
		t.Fatalf("expected 1 explorer completed-blocks request, got %d", len(explorerClient.requests))
	}

	req := explorerClient.requests[0]
	if req.ChainID != cfg.ChainID {
		t.Fatalf("expected request chain id %d, got %d", cfg.ChainID, req.ChainID)
	}

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

	if cursorRepo.upserts[0].LastScannedBlockHash != "0xblock100" {
		t.Fatalf("expected cursor hash 0xblock100, got %q", cursorRepo.upserts[0].LastScannedBlockHash)
	}
}

func TestNativeETHDepositScanner_ScanOnce_WithCursorRequestsNextBlock(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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

	if len(explorerClient.requests) != 1 {
		t.Fatalf("expected 1 completed-blocks request, got %d", len(explorerClient.requests))
	}

	req := explorerClient.requests[0]
	if req.FromBlock != 101 {
		t.Fatalf("expected from block 101, got %d", req.FromBlock)
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

	if cursorRepo.upserts[0].LastScannedBlockNumber != 101 {
		t.Fatalf("expected cursor block 101, got %d", cursorRepo.upserts[0].LastScannedBlockNumber)
	}
}

func TestNativeETHDepositScanner_ScanOnce_UsesConfirmedTargetToCapLimit(t *testing.T) {
	cfg := validScannerConfig()
	cfg.ConfirmationDepth = 3

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 105), // confirmed target = 102
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				{
					Number:     100,
					Hash:       "0xblock100",
					ParentHash: "0xblock99",
				},
				{
					Number:     101,
					Hash:       "0xblock101",
					ParentHash: "0xblock100",
				},
				{
					Number:     102,
					Hash:       "0xblock102",
					ParentHash: "0xblock101",
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
		t.Fatalf("expected 1 completed-blocks request, got %d", len(explorerClient.requests))
	}

	req := explorerClient.requests[0]
	if req.FromBlock != 100 {
		t.Fatalf("expected from block 100, got %d", req.FromBlock)
	}

	if req.Limit != 3 {
		t.Fatalf("expected limit 3, got %d", req.Limit)
	}

	if txRunner.calls != 3 {
		t.Fatalf("expected 3 transaction calls, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 3 {
		t.Fatalf("expected 3 cursor upserts, got %d", len(cursorRepo.upserts))
	}

	if cursorRepo.upserts[2].LastScannedBlockNumber != 102 {
		t.Fatalf("expected final cursor block 102, got %d", cursorRepo.upserts[2].LastScannedBlockNumber)
	}
}

func TestNativeETHDepositScanner_ScanOnce_NoConfirmedBlocksToScan(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 111), // confirmed target = 99
		resp:           nil,
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

	if len(explorerClient.syncStatusChainIDs) != 1 {
		t.Fatalf("expected 1 sync status request, got %d", len(explorerClient.syncStatusChainIDs))
	}

	if len(cursorRepo.getChainIDs) != 1 {
		t.Fatalf("expected 1 cursor lookup, got %d", len(cursorRepo.getChainIDs))
	}

	if len(explorerClient.requests) != 0 {
		t.Fatalf("expected no completed-blocks request, got %d", len(explorerClient.requests))
	}

	if txRunner.calls != 0 {
		t.Fatalf("expected no transaction call, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 0 {
		t.Fatalf("expected no cursor upsert, got %d", len(cursorRepo.upserts))
	}
}

func TestNativeETHDepositScanner_ScanOnce_GetSyncStatusErrorReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusErr: errTest("explorer unavailable"),
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

	if !strings.Contains(err.Error(), "get explorer sync status") {
		t.Fatalf("expected get sync status error, got %q", err.Error())
	}

	if len(cursorRepo.getChainIDs) != 0 {
		t.Fatalf("expected no cursor lookup, got %d", len(cursorRepo.getChainIDs))
	}

	if len(explorerClient.requests) != 0 {
		t.Fatalf("expected no completed-blocks request, got %d", len(explorerClient.requests))
	}
}

func TestNativeETHDepositScanner_ScanOnce_NilSyncStatusReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: nil,
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

	if !strings.Contains(err.Error(), "get explorer sync status returned nil response") {
		t.Fatalf("expected nil sync status error, got %q", err.Error())
	}

	if len(cursorRepo.getChainIDs) != 0 {
		t.Fatalf("expected no cursor lookup, got %d", len(cursorRepo.getChainIDs))
	}
}

func TestNativeETHDepositScanner_ScanOnce_UnexpectedSyncStatusChainIDReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(1, 200),
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

	if !strings.Contains(err.Error(), "unexpected sync status chain_id") {
		t.Fatalf("expected unexpected sync status chain id error, got %q", err.Error())
	}

	if len(cursorRepo.getChainIDs) != 0 {
		t.Fatalf("expected no cursor lookup, got %d", len(cursorRepo.getChainIDs))
	}
}

func TestNativeETHDepositScanner_ScanOnce_NilLatestCompletedBlockReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: &explorer.SyncStatusResponse{
			ChainID:              cfg.ChainID,
			SyncTarget:           "safe",
			LatestCompletedBlock: nil,
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

	if !strings.Contains(err.Error(), "sync status latest_completed_block is nil") {
		t.Fatalf("expected nil latest completed block error, got %q", err.Error())
	}

	if len(cursorRepo.getChainIDs) != 0 {
		t.Fatalf("expected no cursor lookup, got %d", len(cursorRepo.getChainIDs))
	}
}

func TestNativeETHDepositScanner_ScanOnce_CursorErrorReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
	}

	cursorRepo := &fakeCursorRepo{
		err: errTest("db error"),
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

	if !strings.Contains(err.Error(), "get scanner cursor") {
		t.Fatalf("expected get scanner cursor error, got %q", err.Error())
	}

	if len(explorerClient.requests) != 0 {
		t.Fatalf("expected no completed-blocks request, got %d", len(explorerClient.requests))
	}
}

func TestNativeETHDepositScanner_ScanOnce_SortsBlocksAndProcessesSequentially(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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

	if got.BlockNumber != 100 {
		t.Fatalf("expected block number 100, got %d", got.BlockNumber)
	}

	if got.BlockHash != "0xblock100" {
		t.Fatalf("expected block hash 0xblock100, got %q", got.BlockHash)
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
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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

func TestNativeETHDepositScanner_ScanOnce_ListCompletedBlocksErrorReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
		err:            errTest("explorer list failed"),
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

	if !strings.Contains(err.Error(), "list completed blocks") {
		t.Fatalf("expected list completed blocks error, got %q", err.Error())
	}

	if txRunner.calls != 0 {
		t.Fatalf("expected no transaction call, got %d", txRunner.calls)
	}
}

func TestNativeETHDepositScanner_ScanOnce_NilListCompletedBlocksResponseReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
		resp:           nil,
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

func TestNativeETHDepositScanner_ScanOnce_UnexpectedCompletedBlocksChainIDReturnsError(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
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

func TestNativeETHDepositScanner_ScanOnce_CompletedBlocksResponseExceedsRequestedLimitReturnsError(t *testing.T) {
	cfg := validScannerConfig()
	cfg.ConfirmationDepth = 3

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 105), // confirmed target = 102, from = 100, limit = 3
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				completedBlock(100),
				completedBlock(101),
				completedBlock(102),
				completedBlock(103),
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
	assertErrorContains(t, err, "completed blocks response exceeds requested limit")
	assertCompletedBlocksRequest(t, explorerClient, cfg.ChainID, 100, 3)
	assertNoProcessing(t, txRunner, cursorRepo)
}

func TestNativeETHDepositScanner_ScanOnce_CompletedBlockExceedsConfirmedTargetReturnsError(t *testing.T) {
	cfg := validScannerConfig()
	cfg.ConfirmationDepth = 3

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 105), // confirmed target = 102, from = 100, limit = 3
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks: []explorer.CompletedBlock{
				completedBlock(103),
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
	assertErrorContains(t, err, "completed block exceeds confirmed target")
	assertCompletedBlocksRequest(t, explorerClient, cfg.ChainID, 100, 3)
	assertNoProcessing(t, txRunner, cursorRepo)
}

func TestNativeETHDepositScanner_ScanOnce_EmptyCompletedBlocksResponseReturnsNil(t *testing.T) {
	cfg := validScannerConfig()

	explorerClient := &fakeExplorerClient{
		syncStatusResp: validSyncStatus(cfg.ChainID, 200),
		resp: &explorer.ListCompletedBlocksResponse{
			ChainID: cfg.ChainID,
			Blocks:  []explorer.CompletedBlock{},
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

	if len(explorerClient.requests) != 1 {
		t.Fatalf("expected 1 completed-blocks request, got %d", len(explorerClient.requests))
	}

	if txRunner.calls != 0 {
		t.Fatalf("expected no transaction call, got %d", txRunner.calls)
	}

	if len(cursorRepo.upserts) != 0 {
		t.Fatalf("expected no cursor upsert, got %d", len(cursorRepo.upserts))
	}
}

func TestNewNativeETHDepositScanner_InvalidConfigReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr string
	}{
		{
			name: "invalid chain id",
			mutate: func(cfg *Config) {
				cfg.ChainID = 0
			},
			wantErr: "scanner.chain_id must be positive",
		},
		{
			name: "empty scanner name",
			mutate: func(cfg *Config) {
				cfg.ScannerName = " "
			},
			wantErr: "scanner.name is required",
		},
		{
			name: "negative start block",
			mutate: func(cfg *Config) {
				cfg.StartBlock = -1
			},
			wantErr: "scanner.start_block must be non-negative",
		},
		{
			name: "invalid batch size",
			mutate: func(cfg *Config) {
				cfg.BatchSize = 0
			},
			wantErr: "scanner.batch_size must be positive",
		},
		{
			name: "empty min deposit wei",
			mutate: func(cfg *Config) {
				cfg.MinDepositWei = ""
			},
			wantErr: "scanner.min_deposit_wei is required",
		},
		{
			name: "negative confirmation depth",
			mutate: func(cfg *Config) {
				cfg.ConfirmationDepth = -1
			},
			wantErr: "scanner.confirmation_depth must be non-negative",
		},
		{
			name: "invalid min deposit wei",
			mutate: func(cfg *Config) {
				cfg.MinDepositWei = "abc"
			},
			wantErr: "parse scanner.min_deposit_wei",
		},
		{
			name: "zero min deposit wei",
			mutate: func(cfg *Config) {
				cfg.MinDepositWei = "0"
			},
			wantErr: "value must be positive",
		},
		{
			name: "negative min deposit wei",
			mutate: func(cfg *Config) {
				cfg.MinDepositWei = "-1"
			},
			wantErr: "value must be positive",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			cfg := validScannerConfig()
			tt.mutate(&cfg)

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
		tt := tt

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
		tt := tt

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

type errTest string

func (e errTest) Error() string {
	return string(e)
}
