package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Yuilu1317/wallet-backend/internal/config"
)

type fakeDepositScanner struct {
	calls int
	err   error
}

type fakeDepositCreditService struct {
	calls      int
	credited   bool
	err        error
	gotChainID int64
}

func (f *fakeDepositScanner) ScanOnce(ctx context.Context) error {
	f.calls++
	return f.err
}

func (f *fakeDepositCreditService) CreditNext(ctx context.Context, chainID int64) (bool, error) {
	f.calls++
	f.gotChainID = chainID
	return f.credited, f.err
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

	scanner := &fakeDepositScanner{}
	creditService := &fakeDepositCreditService{
		credited: true,
	}

	application := &App{
		cfg:                     validConfig(),
		nativeETHDepositScanner: scanner,
		depositCreditService:    creditService,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := application.Run(ctx)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if scanner.calls != 1 {
		t.Fatalf("expected scanner calls=1, got %d", scanner.calls)
	}

	if creditService.calls != 1 {
		t.Fatalf("expected credit service calls=1, got %d", creditService.calls)
	}

	if creditService.gotChainID != 11155111 {
		t.Fatalf("expected chain_id=11155111, got %d", creditService.gotChainID)
	}
}

func TestRun_WhenScannerFails_ReturnsErrorAndDoesNotCredit(t *testing.T) {
	t.Parallel()

	scanner := &fakeDepositScanner{
		err: errors.New("scanner failed"),
	}
	creditService := &fakeDepositCreditService{}

	application := &App{
		cfg:                     validConfig(),
		nativeETHDepositScanner: scanner,
		depositCreditService:    creditService,
	}

	err := application.Run(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "scan native eth deposits once") {
		t.Fatalf("expected scan error, got %q", err.Error())
	}

	if scanner.calls != 1 {
		t.Fatalf("expected scanner calls=1, got %d", scanner.calls)
	}

	if creditService.calls != 0 {
		t.Fatalf("expected credit service calls=0, got %d", creditService.calls)
	}
}

func TestRun_WhenCreditFails_ReturnsError(t *testing.T) {
	t.Parallel()

	scanner := &fakeDepositScanner{}
	creditService := &fakeDepositCreditService{
		err: errors.New("credit failed"),
	}

	application := &App{
		cfg:                     validConfig(),
		nativeETHDepositScanner: scanner,
		depositCreditService:    creditService,
	}

	err := application.Run(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "credit native eth deposit once") {
		t.Fatalf("expected credit error, got %q", err.Error())
	}

	if scanner.calls != 1 {
		t.Fatalf("expected scanner calls=1, got %d", scanner.calls)
	}

	if creditService.calls != 1 {
		t.Fatalf("expected credit service calls=1, got %d", creditService.calls)
	}
}

func TestClose_WithNilWalletDB_ReturnsNil(t *testing.T) {
	t.Parallel()

	application := &App{
		cfg:                     validConfig(),
		nativeETHDepositScanner: &fakeDepositScanner{},
		depositCreditService:    &fakeDepositCreditService{},
	}

	err := application.Close()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
