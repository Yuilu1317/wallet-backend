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

	rootCtx    context.Context
	rootCancel context.CancelFunc

	workerRunners []*worker.Runner
}

const (
	WorkerNativeETHDepositScanner = "native_eth_deposit_scanner"
	WorkerNativeETHDepositCredit  = "native_eth_deposit_credit"
)

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

	rootCtx, rootCancel := context.WithCancel(context.Background())

	a := &App{
		cfg:        cfg,
		walletDB:   walletDB,
		engine:     engine,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
	}

	if err := a.buildDependencies(); err != nil {
		_ = a.Close()
		return nil, err
	}

	return a, nil
}

func (a *App) buildDependencies() error {
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

	nativeETHDepositScannerRunner, err := worker.NewRunner(
		WorkerNativeETHDepositScanner,
		scannerWorker,
		time.Duration(a.cfg.Worker.IntervalSeconds)*time.Second,
		time.Duration(a.cfg.Worker.ScannerRunOnceTimeoutSeconds)*time.Second,
	)
	if err != nil {
		return fmt.Errorf("create native eth deposit scanner runner: %w", err)
	}

	nativeETHDepositCreditRunner, err := worker.NewRunner(
		WorkerNativeETHDepositCredit,
		creditWorker,
		time.Duration(a.cfg.Worker.IntervalSeconds)*time.Second,
		time.Duration(a.cfg.Worker.CreditRunOnceTimeoutSeconds)*time.Second,
	)
	if err != nil {
		return fmt.Errorf("create native eth deposit credit runner: %w", err)
	}

	a.workerRunners = []*worker.Runner{
		nativeETHDepositScannerRunner,
		nativeETHDepositCreditRunner,
	}

	workerController, err := controller.NewWorkerController(
		a.rootCtx,
		map[string]controller.WorkerRunner{
			WorkerNativeETHDepositScanner: nativeETHDepositScannerRunner,
			WorkerNativeETHDepositCredit:  nativeETHDepositCreditRunner,
		},
	)
	if err != nil {
		return fmt.Errorf("create worker controller: %w", err)
	}

	if err := a.registerRoutes(workerController); err != nil {
		return fmt.Errorf("register routes: %w", err)
	}

	return nil
}

func (a *App) registerRoutes(workerController *controller.WorkerController) error {
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

		if a.rootCancel != nil {
			a.rootCancel()
		}

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
	if a.rootCancel != nil {
		a.rootCancel()
		a.rootCancel = nil
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer waitCancel()

	if err := a.waitWorkerRunners(waitCtx); err != nil {
		log.Printf("wait worker runners before close: %v", err)
	}

	if a.walletDB == nil {
		return nil
	}

	if err := walletdb.Close(a.walletDB); err != nil {
		return fmt.Errorf("close wallet database: %w", err)
	}

	a.walletDB = nil

	log.Println("wallet database closed")
	return nil
}

func (a *App) waitWorkerRunners(ctx context.Context) error {
	for _, runner := range a.workerRunners {
		if runner == nil {
			continue
		}

		if err := runner.Wait(ctx); err != nil {
			return err
		}
	}

	return nil
}
