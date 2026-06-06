package scanner

import (
	"context"
	"fmt"
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

type NativeETHDepositScanner struct {
	cfg Config

	explorerClient     explorer.Client
	depositAddressRepo DepositAddressRepository
	depositRepo        DepositRepository
	cursorRepo         CursorRepository
}

func NewNativeETHDepositScanner(
	cfg Config,
	explorerClient explorer.Client,
	depositAddressRepo DepositAddressRepository,
	depositRepo DepositRepository,
	cursorRepo CursorRepository,
) (*NativeETHDepositScanner, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	if explorerClient == nil {
		return nil, fmt.Errorf("explorer client is required")
	}

	if depositAddressRepo == nil {
		return nil, fmt.Errorf("deposit address repo is required")
	}

	if depositRepo == nil {
		return nil, fmt.Errorf("deposit repo is required")
	}

	if cursorRepo == nil {
		return nil, fmt.Errorf("scanner cursor repo is required")
	}

	return &NativeETHDepositScanner{
		cfg:                cfg,
		explorerClient:     explorerClient,
		depositAddressRepo: depositAddressRepo,
		depositRepo:        depositRepo,
		cursorRepo:         cursorRepo,
	}, nil
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
		if err := s.processBlock(ctx, block); err != nil {
			return fmt.Errorf("process block %d: %w", block.Number, err)
		}
		if err := s.cursorRepo.UpsertAfterBlockProcessed(ctx, &model.WalletScannerCursor{
			ChainID:                s.cfg.ChainID,
			ScannerName:            s.cfg.ScannerName,
			LastScannedBlockNumber: block.Number,
			LastScannedBlockHash:   block.Hash,
		}); err != nil {
			return fmt.Errorf("update scanner cursor after block %d: %w", block.Number, err)
		}
		previousHash = block.Hash
		expectedBlockNumber = block.Number + 1
	}
	return nil
}

func (s *NativeETHDepositScanner) processBlock(
	ctx context.Context,
	block explorer.CompletedBlock,
) error {
	for _, tx := range block.Transactions {
		if err := s.processTransaction(ctx, block, tx); err != nil {
			return fmt.Errorf("process transaction %s: %w", tx.TxHash, err)
		}
	}
	return nil
}

func (s *NativeETHDepositScanner) processTransaction(
	ctx context.Context,
	block explorer.CompletedBlock,
	tx explorer.CompletedTransaction,
) error {
	if tx.ReceiptStatus != 1 {
		return nil
	}

	ok, err := isAmountAtLeast(tx.AmountWei, s.cfg.MinDepositWei)
	if err != nil {
		return fmt.Errorf("compare amount wei: %w", err)
	}
	if !ok {
		return nil
	}

	toAddressLower := strings.ToLower(strings.TrimSpace(tx.ToAddress))
	if toAddressLower == "" {
		return nil
	}

	depositAddress, found, err := s.depositAddressRepo.FindActiveByChainIDAndAddressLower(
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
		TxHash:           tx.TxHash,
		BlockNumber:      block.Number,
		BlockHash:        block.Hash,
		FromAddress:      strings.ToLower(strings.TrimSpace(tx.FromAddress)),
		ToAddress:        toAddressLower,
		AmountWei:        tx.AmountWei,
		Status:           model.DepositStatusConfirming,
		ReceiptStatus:    tx.ReceiptStatus,
	}
	_, err = s.depositRepo.CreateConfirmingDepositIdempotently(ctx, deposit)
	if err != nil {
		return fmt.Errorf("create confirming deposit idempotently: %w", err)
	}

	return nil
}

func isAmountAtLeast(amountWei string, minWei string) (bool, error) {
	amount, ok := new(big.Int).SetString(strings.TrimSpace(amountWei), 10)
	if !ok {
		return false, fmt.Errorf("invalid amount_wei %q", amountWei)
	}
	if amount.Sign() < 0 {
		return false, fmt.Errorf("amount_wei must be non-negative")
	}

	minAmount, ok := new(big.Int).SetString(strings.TrimSpace(minWei), 10)
	if !ok {
		return false, fmt.Errorf("invalid min_wei %q", minWei)
	}
	if minAmount.Sign() <= 0 {
		return false, fmt.Errorf("min_wei must be positive")
	}
	return amount.Cmp(minAmount) >= 0, nil
}
