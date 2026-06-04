package app

import (
	"context"
	"fmt"
	"log"

	"github.com/Yuilu1317/wallet-backend/internal/config"
	walletdb "github.com/Yuilu1317/wallet-backend/internal/db"
	"gorm.io/gorm"
)

type App struct {
	cfg      *config.Config
	walletDB *gorm.DB
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

	return &App{
		cfg:      cfg,
		walletDB: walletDB,
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
