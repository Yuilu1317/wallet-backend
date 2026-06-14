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
	"github.com/Yuilu1317/wallet-backend/internal/service"
	"gorm.io/gorm"
)

type DepositScanner interface {
	ScanOnce(ctx context.Context) error
}

type DepositCreditService interface {
	CreditNext(ctx context.Context, chainID int64) (bool, error)
}
type App struct {
	cfg                     *config.Config
	walletDB                *gorm.DB
	nativeETHDepositScanner DepositScanner
	depositCreditService    DepositCreditService
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

	depositCreditTxRunner := repo.NewDepositCreditTransactionRunner(walletDB)
	depositCreditService := service.NewDepositCreditService(
		depositCreditTxRunner,
	)
	return &App{
		cfg:                     cfg,
		walletDB:                walletDB,
		nativeETHDepositScanner: nativeETHDepositScanner,
		depositCreditService:    depositCreditService,
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
	if a.depositCreditService == nil {
		return fmt.Errorf("deposit credit service is nil")
	}
	credited, err := a.depositCreditService.CreditNext(ctx, a.cfg.Ethereum.ChainID)
	if err != nil {
		return fmt.Errorf("credit native eth deposit once: %w", err)
	}

	if credited {
		log.Println("native eth deposit credit once completed")
	} else {
		log.Println("no creditable native eth deposit found")
	}
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
