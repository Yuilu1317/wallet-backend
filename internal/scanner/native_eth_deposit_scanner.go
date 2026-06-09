package scanner

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"

	"github.com/Yuilu1317/wallet-backend/internal/explorer"
	"github.com/Yuilu1317/wallet-backend/internal/model"
)

type DepositAddressRepository interface {
	FindActiveByChainIDAndAddressLower(
		ctx context.Context,
		chainID int64,
		addressLower string,
	) (*model.DepositAddress, bool, error)
}

type DepositRepository interface {
	CreateConfirmingDepositIdempotently(
		ctx context.Context,
		deposit *model.Deposit,
	) (bool, error)
}

type CursorRepository interface {
	GetByChainIDAndScannerName(
		ctx context.Context,
		chainID int64,
		scannerName string,
	) (*model.WalletScannerCursor, bool, error)

	UpsertAfterBlockProcessed(
		ctx context.Context,
		cursor *model.WalletScannerCursor,
	) error
}

type Config struct {
	ChainID       int64
	ScannerName   string
	StartBlock    int64
	BatchSize     int
	MinDepositWei string
}

type Repositories struct {
	DepositAddressRepo DepositAddressRepository
	DepositRepo        DepositRepository
	CursorRepo         CursorRepository
}

type TransactionRunner interface {
	WithinTransaction(
		ctx context.Context,
		fn func(repos Repositories) error,
	) error
}

type NativeETHDepositScanner struct {
	cfg           Config
	minDepositWei *big.Int

	explorerClient explorer.Client
	cursorRepo     CursorRepository
	txRunner       TransactionRunner
}

func NewNativeETHDepositScanner(
	cfg Config,
	explorerClient explorer.Client,
	cursorRepo CursorRepository,
	txRunner TransactionRunner,
) (*NativeETHDepositScanner, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	minDepositWei, err := parsePositiveWei(cfg.MinDepositWei)
	if err != nil {
		return nil, fmt.Errorf("parse scanner.min_deposit_wei: %w", err)
	}

	if explorerClient == nil {
		return nil, fmt.Errorf("explorer client is required")
	}

	if cursorRepo == nil {
		return nil, fmt.Errorf("scanner cursor repo is required")
	}

	if txRunner == nil {
		return nil, fmt.Errorf("transaction runner is required")
	}

	return &NativeETHDepositScanner{
		cfg:            cfg,
		minDepositWei:  minDepositWei,
		explorerClient: explorerClient,
		cursorRepo:     cursorRepo,
		txRunner:       txRunner,
	}, nil
}

func parseWei(value string) (*big.Int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("value is required")
	}

	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("value must be a base-10 integer")
	}

	return amount, nil
}

func parsePositiveWei(value string) (*big.Int, error) {
	amount, err := parseWei(value)
	if err != nil {
		return nil, err
	}

	if amount.Sign() <= 0 {
		return nil, fmt.Errorf("value must be positive")
	}

	return amount, nil
}

func validateConfig(cfg Config) error {
	if cfg.ChainID <= 0 {
		return fmt.Errorf("scanner.chain_id must be positive")
	}

	if strings.TrimSpace(cfg.ScannerName) == "" {
		return fmt.Errorf("scanner.name is required")
	}

	if cfg.StartBlock < 0 {
		return fmt.Errorf("scanner.start_block must be non-negative")
	}

	if cfg.BatchSize <= 0 {
		return fmt.Errorf("scanner.batch_size must be positive")
	}

	if strings.TrimSpace(cfg.MinDepositWei) == "" {
		return fmt.Errorf("scanner.min_deposit_wei is required")
	}

	return nil
}

func (s *NativeETHDepositScanner) ScanOnce(ctx context.Context) error {
	cursor, found, err := s.cursorRepo.GetByChainIDAndScannerName(ctx, s.cfg.ChainID, s.cfg.ScannerName)
	if err != nil {
		return fmt.Errorf("get scanner cursor: %w", err)
	}
	fromBlock := s.cfg.StartBlock
	previousHash := ""
	if found {
		fromBlock = cursor.LastScannedBlockNumber + 1
		previousHash = cursor.LastScannedBlockHash
	}

	resp, err := s.explorerClient.ListCompletedBlocks(ctx, explorer.ListCompletedBlocksRequest{
		ChainID:   s.cfg.ChainID,
		FromBlock: fromBlock,
		Limit:     s.cfg.BatchSize,
	})
	if err != nil {
		return fmt.Errorf("list completed blocks: %w", err)
	}

	if resp == nil {
		return fmt.Errorf("list completed blocks returned nil response")
	}
	if resp.ChainID != s.cfg.ChainID {
		return fmt.Errorf("unexpected response chain_id: got=%d expected=%d", resp.ChainID, s.cfg.ChainID)
	}

	return s.processBlocks(ctx, fromBlock, previousHash, resp.Blocks)
}

func (s *NativeETHDepositScanner) processBlocks(
	ctx context.Context,
	expectedBlockNumber int64,
	previousHash string,
	blocks []explorer.CompletedBlock,
) error {
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Number < blocks[j].Number
	})

	for _, block := range blocks {
		if block.Number != expectedBlockNumber {
			return fmt.Errorf(
				"unexpected block number: got=%d expected=%d",
				block.Number,
				expectedBlockNumber,
			)
		}
		if previousHash != "" && block.ParentHash != previousHash {
			return fmt.Errorf(
				"block continuity check failed: block=%d parent_hash=%s expected_parent_hash=%s",
				block.Number,
				block.ParentHash,
				previousHash,
			)
		}
		if err := s.processBlockInTransaction(ctx, block); err != nil {
			return fmt.Errorf("process block in transaction %d: %w", block.Number, err)
		}
		previousHash = block.Hash
		expectedBlockNumber = block.Number + 1
	}
	return nil
}

func (s *NativeETHDepositScanner) processBlockInTransaction(
	ctx context.Context,
	block explorer.CompletedBlock,
) error {
	return s.txRunner.WithinTransaction(ctx, func(repos Repositories) error {
		if err := s.processBlock(ctx, repos, block); err != nil {
			return fmt.Errorf("process block: %w", err)
		}

		if err := repos.CursorRepo.UpsertAfterBlockProcessed(ctx, &model.WalletScannerCursor{
			ChainID:                s.cfg.ChainID,
			ScannerName:            s.cfg.ScannerName,
			LastScannedBlockNumber: block.Number,
			LastScannedBlockHash:   block.Hash,
		}); err != nil {
			return fmt.Errorf("update scanner cursor: %w", err)
		}

		return nil
	})
}

func (s *NativeETHDepositScanner) processBlock(
	ctx context.Context,
	repos Repositories,
	block explorer.CompletedBlock,
) error {
	for _, tx := range block.Transactions {
		if err := s.processTransaction(ctx, repos, block, tx); err != nil {
			return fmt.Errorf("process transaction %s: %w", tx.TxHash, err)
		}
	}
	return nil
}

func (s *NativeETHDepositScanner) processTransaction(
	ctx context.Context,
	repos Repositories,
	block explorer.CompletedBlock,
	tx explorer.CompletedTransaction,
) error {
	if tx.ReceiptStatus != 1 {
		return nil
	}
	txHash := strings.ToLower(strings.TrimSpace(tx.TxHash))
	if txHash == "" {
		return fmt.Errorf("tx_hash is required")
	}

	amountWei, err := parseNonNegativeWei(tx.AmountWei)
	if err != nil {
		return fmt.Errorf("parse amount_wei: tx_hash=%s amount_wei=%q: %w",
			tx.TxHash,
			tx.AmountWei,
			err,
		)
	}

	if amountWei.Cmp(s.minDepositWei) < 0 {
		return nil
	}

	toAddressLower := strings.ToLower(strings.TrimSpace(tx.ToAddress))
	if toAddressLower == "" {
		return nil
	}

	depositAddress, found, err := repos.DepositAddressRepo.FindActiveByChainIDAndAddressLower(
		ctx,
		s.cfg.ChainID,
		toAddressLower,
	)
	if err != nil {
		return fmt.Errorf("find active deposit address: %w", err)
	}
	if !found {
		return nil
	}

	deposit := &model.Deposit{
		UserID:           depositAddress.UserID,
		ChainID:          s.cfg.ChainID,
		DepositAddressID: depositAddress.ID,
		TxHash:           txHash,
		BlockNumber:      block.Number,
		BlockHash:        block.Hash,
		FromAddress:      strings.ToLower(strings.TrimSpace(tx.FromAddress)),
		ToAddress:        toAddressLower,
		AmountWei:        amountWei.String(),
		Status:           model.DepositStatusConfirming,
		ReceiptStatus:    tx.ReceiptStatus,
	}
	created, err := repos.DepositRepo.CreateConfirmingDepositIdempotently(ctx, deposit)
	if err != nil {
		return fmt.Errorf("create confirming deposit idempotently: %w", err)
	}
	if created {
		log.Printf(
			"native eth deposit created: chain_id=%d user_id=%d deposit_address_id=%d tx_hash=%s block_number=%d amount_wei=%s",
			deposit.ChainID,
			deposit.UserID,
			deposit.DepositAddressID,
			deposit.TxHash,
			deposit.BlockNumber,
			deposit.AmountWei,
		)
	}

	return nil
}

func parseNonNegativeWei(value string) (*big.Int, error) {
	amount, err := parseWei(value)
	if err != nil {
		return nil, err
	}

	if amount.Sign() < 0 {
		return nil, fmt.Errorf("value must be non-negative")
	}

	return amount, nil
}
