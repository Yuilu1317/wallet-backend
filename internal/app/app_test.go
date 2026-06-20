package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Yuilu1317/wallet-backend/internal/config"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func validConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{
			Env:      "test",
			HTTPPort: "0",
		},
		Database: config.DatabaseConfig{
			DSN:              "test-dsn",
			DBTimeoutSeconds: 1,
		},
		Ethereum: config.EthereumConfig{
			ChainID:           11155111,
			ConfirmationDepth: 12,
			MinDepositWei:     "1",
		},
		Scanner: config.ScannerConfig{
			Name:       "native_eth_deposit_scanner",
			StartBlock: 0,
			BatchSize:  10,
		},
		Explorer: config.ExplorerConfig{
			BaseURL:        "http://localhost:8080",
			TimeoutSeconds: 5,
		},
	}
}

func TestNew_WithInvalidConfigPath_ReturnsError(t *testing.T) {
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

func TestRun_WithCanceledContext_ShutsDownAndReturnsNil(t *testing.T) {
	application := &App{
		cfg:    validConfig(),
		engine: gin.New(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := application.Run(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRun_WhenHTTPServerFails_ReturnsError(t *testing.T) {
	cfg := validConfig()
	cfg.App.HTTPPort = "invalid-port"

	application := &App{
		cfg:    cfg,
		engine: gin.New(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := application.Run(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "http server error") {
		t.Fatalf("expected http server error, got %q", err.Error())
	}
}

func TestClose_WithNilWalletDB_ReturnsNil(t *testing.T) {
	application := &App{
		cfg: validConfig(),
	}

	err := application.Close()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
