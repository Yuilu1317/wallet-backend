package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		App: AppConfig{
			Env:      "dev",
			HTTPPort: "8081",
		},
		Database: DatabaseConfig{
			DSN: "host=127.0.0.1 user=postgres password=test dbname=wallet_backend_dev port=5432 sslmode=disable",
		},
		Ethereum: EthereumConfig{
			ChainID:           11155111,
			ConfirmationDepth: 12,
			MinDepositWei:     "1",
		},
		Scanner: ScannerConfig{
			Name:                "native_eth_deposit_scanner",
			StartBlock:          0,
			BatchSize:           10,
			PollIntervalSeconds: 5,
		},
		Explorer: ExplorerConfig{
			BaseURL:        "http://localhost:8080",
			TimeoutSeconds: 5,
		},
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	return path
}

func TestConfigValidate_WithValidConfig_ReturnsNil(t *testing.T) {
	t.Parallel()

	cfg := validConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestConfigValidate_WithInvalidConfig_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "missing app env",
			mutate: func(cfg *Config) {
				cfg.App.Env = ""
			},
			wantErr: "app.env is required",
		},
		{
			name: "missing http port",
			mutate: func(cfg *Config) {
				cfg.App.HTTPPort = ""
			},
			wantErr: "app.http_port is required",
		},
		{
			name: "missing database dsn",
			mutate: func(cfg *Config) {
				cfg.Database.DSN = ""
			},
			wantErr: "database.dsn is required",
		},
		{
			name: "non-positive ethereum chain id",
			mutate: func(cfg *Config) {
				cfg.Ethereum.ChainID = 0
			},
			wantErr: "ethereum.chain_id must be positive",
		},
		{
			name: "non-positive confirmation depth",
			mutate: func(cfg *Config) {
				cfg.Ethereum.ConfirmationDepth = -1
			},
			wantErr: "ethereum.confirmation_depth must be non-negative",
		},
		{
			name: "missing min deposit wei",
			mutate: func(cfg *Config) {
				cfg.Ethereum.MinDepositWei = ""
			},
			wantErr: "ethereum.min_deposit_wei is required",
		},
		{
			name: "invalid min deposit wei",
			mutate: func(cfg *Config) {
				cfg.Ethereum.MinDepositWei = "abc"
			},
			wantErr: "ethereum.min_deposit_wei must be a positive integer string",
		},
		{
			name: "zero min deposit wei",
			mutate: func(cfg *Config) {
				cfg.Ethereum.MinDepositWei = "0"
			},
			wantErr: "ethereum.min_deposit_wei must be positive",
		},
		{
			name: "missing scanner name",
			mutate: func(cfg *Config) {
				cfg.Scanner.Name = ""
			},
			wantErr: "scanner.name is required",
		},
		{
			name: "negative scanner start block",
			mutate: func(cfg *Config) {
				cfg.Scanner.StartBlock = -1
			},
			wantErr: "scanner.start_block must be non-negative",
		},
		{
			name: "non-positive scanner batch size",
			mutate: func(cfg *Config) {
				cfg.Scanner.BatchSize = 0
			},
			wantErr: "scanner.batch_size must be positive",
		},
		{
			name: "non-positive scanner poll interval",
			mutate: func(cfg *Config) {
				cfg.Scanner.PollIntervalSeconds = 0
			},
			wantErr: "scanner.poll_interval_seconds must be positive",
		},
		{
			name: "missing explorer base url",
			mutate: func(cfg *Config) {
				cfg.Explorer.BaseURL = ""
			},
			wantErr: "explorer.base_url is required",
		},
		{
			name: "non-positive explorer timeout",
			mutate: func(cfg *Config) {
				cfg.Explorer.TimeoutSeconds = 0
			},
			wantErr: "explorer.timeout_seconds must be positive",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig()
			tt.mutate(&cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestLoad_WithValidYAMLAndEnvExpansion_ReturnsConfig(t *testing.T) {
	t.Setenv("DATABASE_DSN", "host=127.0.0.1 user=postgres password=test dbname=wallet_backend_dev port=5432 sslmode=disable")

	configPath := writeTempConfig(t, `
app:
  env: dev
  http_port: "8081"

database:
  dsn: "${DATABASE_DSN}"

ethereum:
  chain_id: 11155111
  confirmation_depth: 12
  min_deposit_wei: "1"

scanner:
  name: native_eth_deposit_scanner
  start_block: 0
  batch_size: 10
  poll_interval_seconds: 5

explorer:
  base_url: "http://localhost:8080"
  timeout_seconds: 5
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if cfg.Database.DSN == "" {
		t.Fatal("expected database dsn to be expanded, got empty")
	}

	if cfg.Ethereum.ChainID != 11155111 {
		t.Fatalf("expected chain id 11155111, got %d", cfg.Ethereum.ChainID)
	}

	if cfg.Scanner.Name != "native_eth_deposit_scanner" {
		t.Fatalf("expected scanner name native_eth_deposit_scanner, got %q", cfg.Scanner.Name)
	}
}

func TestLoad_WithEmptyPath_ReturnsError(t *testing.T) {
	t.Parallel()

	cfg, err := Load("")

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}

	if !strings.Contains(err.Error(), "config path is required") {
		t.Fatalf("expected config path error, got %q", err.Error())
	}
}

func TestLoad_WithMissingFile_ReturnsError(t *testing.T) {
	t.Parallel()

	cfg, err := Load("not-exist.yaml")

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}

	if !strings.Contains(err.Error(), "read config file") {
		t.Fatalf("expected read config file error, got %q", err.Error())
	}
}

func TestLoad_WithInvalidYAML_ReturnsError(t *testing.T) {
	t.Parallel()

	configPath := writeTempConfig(t, `
app:
  env: dev
  http_port: "8081"
database:
  dsn: [
`)

	cfg, err := Load(configPath)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}

	if !strings.Contains(err.Error(), "unmarshal yaml") {
		t.Fatalf("expected unmarshal yaml error, got %q", err.Error())
	}
}

func TestLoad_WithMissingDatabaseDSNEnv_ReturnsError(t *testing.T) {
	t.Setenv("DATABASE_DSN", "")

	configPath := writeTempConfig(t, `
app:
  env: dev
  http_port: "8081"

database:
  dsn: "${DATABASE_DSN}"

ethereum:
  chain_id: 11155111
  confirmation_depth: 12
  min_deposit_wei: "1"

scanner:
  name: native_eth_deposit_scanner
  start_block: 0
  batch_size: 10
  poll_interval_seconds: 5

explorer:
  base_url: "http://localhost:8080"
  timeout_seconds: 5
`)

	cfg, err := Load(configPath)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}

	if !strings.Contains(err.Error(), "database.dsn is required") {
		t.Fatalf("expected database.dsn error, got %q", err.Error())
	}
}
