package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/config"
	walletdb "github.com/Yuilu1317/wallet-backend/internal/db"
	"github.com/Yuilu1317/wallet-backend/internal/db/repo"
	"github.com/Yuilu1317/wallet-backend/internal/explorer"
	"github.com/Yuilu1317/wallet-backend/internal/scanner"
	"gorm.io/gorm"
)

type DepositScanner interface {
	ScanOnce(ctx context.Context) error
}
type App struct {
	cfg                     *config.Config
	walletDB                *gorm.DB
	nativeETHDepositScanner DepositScanner
}

func New(configPath string) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	walletDB, err := walletdb.OpenPostgres(cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("open wallet database: %w", err)
	}

	explorerClient, err := explorer.NewHTTPClient(
		cfg.Explorer.BaseURL,
		time.Duration(cfg.Explorer.TimeoutSeconds)*time.Second,
	)
	if err != nil {
		return nil, fmt.Errorf("create explorer client: %w", err)
	}

	scannerCursorRepo := repo.NewScannerCursorRepo(walletDB)
	txRunner := repo.NewScannerTransactionRunner(walletDB)

	nativeETHDepositScanner, err := scanner.NewNativeETHDepositScanner(
		scanner.Config{
			ChainID:       cfg.Ethereum.ChainID,
			ScannerName:   cfg.Scanner.Name,
			StartBlock:    cfg.Scanner.StartBlock,
			BatchSize:     cfg.Scanner.BatchSize,
			MinDepositWei: cfg.Ethereum.MinDepositWei,
		},
		explorerClient,
		scannerCursorRepo,
		txRunner,
	)
	if err != nil {
		return nil, fmt.Errorf("create native eth deposit scanner: %w", err)

	}
	return &App{
		cfg:                     cfg,
		walletDB:                walletDB,
		nativeETHDepositScanner: nativeETHDepositScanner,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	log.Printf(
		"app started: env=%s http_port=%s chain_id=%d scanner=%s explorer=%s",
		a.cfg.App.Env,
		a.cfg.App.HTTPPort,
		a.cfg.Ethereum.ChainID,
		a.cfg.Scanner.Name,
		a.cfg.Explorer.BaseURL,
	)

	log.Println("wallet database connected")
	if err := a.nativeETHDepositScanner.ScanOnce(ctx); err != nil {
		return fmt.Errorf("scan native eth deposits once: %w", err)
	}
	log.Println("native eth deposit scan once completed")
	log.Println("app is running, press Ctrl+C to stop")

	<-ctx.Done()

	log.Println("shutdown signal received")
	return nil
}

func (a *App) Close() error {
	if a.walletDB == nil {
		return nil
	}

	if err := walletdb.Close(a.walletDB); err != nil {
		return fmt.Errorf("close wallet database: %w", err)
	}

	log.Println("wallet database closed")
	return nil
}
