package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/config"
	"github.com/Yuilu1317/wallet-backend/internal/controller"
	walletdb "github.com/Yuilu1317/wallet-backend/internal/db"
	"github.com/Yuilu1317/wallet-backend/internal/db/repo"
	"github.com/Yuilu1317/wallet-backend/internal/explorer"
	"github.com/Yuilu1317/wallet-backend/internal/router"
	"github.com/Yuilu1317/wallet-backend/internal/scanner"
	"github.com/Yuilu1317/wallet-backend/internal/service"
	"github.com/Yuilu1317/wallet-backend/internal/worker"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type App struct {
	cfg      *config.Config
	walletDB *gorm.DB
	engine   *gin.Engine
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

	engine := gin.Default()

	a := &App{
		cfg:      cfg,
		walletDB: walletDB,
		engine:   engine,
	}

	if err := a.registerRoutes(); err != nil {
		_ = a.Close()
		return nil, err
	}

	return a, nil
}

func (a *App) registerRoutes() error {
	explorerClient, err := explorer.NewHTTPClient(
		a.cfg.Explorer.BaseURL,
		time.Duration(a.cfg.Explorer.TimeoutSeconds)*time.Second,
	)
	if err != nil {
		return fmt.Errorf("create explorer client: %w", err)
	}

	scannerCursorRepo := repo.NewScannerCursorRepo(a.walletDB)
	scannerTxRunner := repo.NewScannerTransactionRunner(a.walletDB)

	nativeETHDepositScanner, err := scanner.NewNativeETHDepositScanner(
		scanner.Config{
			ChainID:           a.cfg.Ethereum.ChainID,
			ScannerName:       a.cfg.Scanner.Name,
			StartBlock:        a.cfg.Scanner.StartBlock,
			BatchSize:         a.cfg.Scanner.BatchSize,
			MinDepositWei:     a.cfg.Ethereum.MinDepositWei,
			ConfirmationDepth: a.cfg.Ethereum.ConfirmationDepth,
			DBTimeout:         time.Duration(a.cfg.Database.DBTimeoutSeconds) * time.Second,
		},
		explorerClient,
		scannerCursorRepo,
		scannerTxRunner,
	)
	if err != nil {
		return fmt.Errorf("create native eth deposit scanner: %w", err)
	}

	scannerWorker, err := worker.NewNativeETHDepositScannerWorker(nativeETHDepositScanner)
	if err != nil {
		return fmt.Errorf("create scanner worker: %w", err)
	}

	depositCreditTxRunner := repo.NewDepositCreditTransactionRunner(a.walletDB)

	depositCreditService, err := service.NewDepositCreditService(
		depositCreditTxRunner,
		time.Duration(a.cfg.Database.DBTimeoutSeconds)*time.Second,
	)
	if err != nil {
		return fmt.Errorf("create deposit credit service: %w", err)
	}

	creditWorker, err := worker.NewCreditWorker(
		a.cfg.Ethereum.ChainID,
		depositCreditService,
	)
	if err != nil {
		return fmt.Errorf("create credit worker: %w", err)
	}

	workerController, err := controller.NewWorkerController(
		scannerWorker,
		creditWorker,
	)
	if err != nil {
		return fmt.Errorf("create worker controller: %w", err)
	}

	router.RegisterWorkerRoutes(a.engine, workerController)

	return nil
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

	addr := ":" + a.cfg.App.HTTPPort

	srv := &http.Server{
		Addr:    addr,
		Handler: a.engine,
	}

	errCh := make(chan error, 1)

	go func() {
		log.Printf("http server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Println("shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}

		log.Println("server exited")
		return nil

	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server error: %w", err)
		}
		return nil
	}
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
