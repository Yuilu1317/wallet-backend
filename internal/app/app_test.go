package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/config"
)

type fakeDepositScanner struct {
	calls int
	err   error
}

func (f *fakeDepositScanner) ScanOnce(ctx context.Context) error {
	f.calls++
	return f.err
}

func validConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{
			Env:      "dev",
			HTTPPort: "8081",
		},
		Database: config.DatabaseConfig{
			DSN: "test-dsn",
		},
		Ethereum: config.EthereumConfig{
			ChainID:           11155111,
			ConfirmationDepth: 12,
			MinDepositWei:     "1",
		},
		Scanner: config.ScannerConfig{
			Name:                "native_eth_deposit_scanner",
			StartBlock:          0,
			BatchSize:           10,
			PollIntervalSeconds: 5,
		},
		Explorer: config.ExplorerConfig{
			BaseURL:        "http://localhost:8080",
			TimeoutSeconds: 5,
		},
	}
}

func TestNew_WithInvalidConfigPath_ReturnsError(t *testing.T) {
	t.Parallel()
	application, err := New("not-exist.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if application != nil {
		t.Fatalf("expected nil app, got %+v", application)
	}

	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("expected error to contain %q, got %q", "load config", err.Error())
	}
}

func TestRun_WithCanceledContext_ReturnsNil(t *testing.T) {
	t.Parallel()

	application := &App{
		cfg:                     validConfig(),
		nativeETHDepositScanner: &fakeDepositScanner{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := application.Run(ctx)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestClose_WithNilWalletDB_ReturnsNil(t *testing.T) {
	t.Parallel()

	application := &App{
		cfg:                     validConfig(),
		nativeETHDepositScanner: &fakeDepositScanner{},
	}

	err := application.Close()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
