package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App      AppConfig      `mapstructure:"app" yaml:"app"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database"`
	Ethereum EthereumConfig `mapstructure:"ethereum" yaml:"ethereum"`
	Scanner  ScannerConfig  `mapstructure:"scanner" yaml:"scanner"`
	Explorer ExplorerConfig `mapstructure:"explorer" yaml:"explorer"`
	Worker   WorkerConfig   `mapstructure:"worker" yaml:"worker"`
}

type AppConfig struct {
	Env      string `mapstructure:"env" yaml:"env"`
	HTTPPort string `mapstructure:"http_port" yaml:"http_port"`
}

type DatabaseConfig struct {
	DSN              string `mapstructure:"dsn" yaml:"dsn"`
	DBTimeoutSeconds int    `mapstructure:"db_timeout_seconds" yaml:"db_timeout_seconds"`
}

type EthereumConfig struct {
	ChainID           int64  `mapstructure:"chain_id" yaml:"chain_id"`
	ConfirmationDepth int64  `mapstructure:"confirmation_depth" yaml:"confirmation_depth"`
	MinDepositWei     string `mapstructure:"min_deposit_wei" yaml:"min_deposit_wei"`
}

type ScannerConfig struct {
	Name       string `mapstructure:"name" yaml:"name"`
	StartBlock int64  `mapstructure:"start_block" yaml:"start_block"`
	BatchSize  int    `mapstructure:"batch_size" yaml:"batch_size"`
}

type ExplorerConfig struct {
	BaseURL        string `mapstructure:"base_url" yaml:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds" yaml:"timeout_seconds"`
}

type WorkerConfig struct {
	IntervalSeconds              int `mapstructure:"interval_seconds" yaml:"interval_seconds"`
	ScannerRunOnceTimeoutSeconds int `mapstructure:"scanner_run_once_timeout_seconds" yaml:"scanner_run_once_timeout_seconds"`
	CreditRunOnceTimeoutSeconds  int `mapstructure:"credit_run_once_timeout_seconds" yaml:"credit_run_once_timeout_seconds"`
}

func Load(configPath string) (*Config, error) {
	if strings.TrimSpace(configPath) == "" {
		return nil, fmt.Errorf("config path is required")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}
func (c *Config) Validate() error {
	if err := validateRequiredString("app.env", c.App.Env); err != nil {
		return err
	}

	if err := validateRequiredString("app.http_port", c.App.HTTPPort); err != nil {
		return err
	}

	if err := validateRequiredString("database.dsn", c.Database.DSN); err != nil {
		return err
	}
	if err := validatePositiveInt("database.db_timeout_seconds", c.Database.DBTimeoutSeconds); err != nil {
		return err
	}

	if err := validatePositiveInt64("ethereum.chain_id", c.Ethereum.ChainID); err != nil {
		return err
	}

	if err := validateNonNegativeInt64("ethereum.confirmation_depth", c.Ethereum.ConfirmationDepth); err != nil {
		return err
	}

	if err := validatePositiveIntegerString("ethereum.min_deposit_wei", c.Ethereum.MinDepositWei); err != nil {
		return err
	}

	if err := validateRequiredString("scanner.name", c.Scanner.Name); err != nil {
		return err
	}

	if err := validateNonNegativeInt64("scanner.start_block", c.Scanner.StartBlock); err != nil {
		return err
	}

	if err := validatePositiveInt("scanner.batch_size", c.Scanner.BatchSize); err != nil {
		return err
	}

	if err := validateRequiredString("explorer.base_url", c.Explorer.BaseURL); err != nil {
		return err
	}

	if err := validatePositiveInt("explorer.timeout_seconds", c.Explorer.TimeoutSeconds); err != nil {
		return err
	}

	if err := validatePositiveInt("worker.interval_seconds", c.Worker.IntervalSeconds); err != nil {
		return err
	}

	if err := validatePositiveInt("worker.scanner_run_once_timeout_seconds", c.Worker.ScannerRunOnceTimeoutSeconds); err != nil {
		return err
	}

	if err := validatePositiveInt("worker.credit_run_once_timeout_seconds", c.Worker.CreditRunOnceTimeoutSeconds); err != nil {
		return err
	}

	return nil
}

func validateRequiredString(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func validatePositiveInt64(name string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func validateNonNegativeInt64(name string, value int64) error {
	if value < 0 {
		return fmt.Errorf("%s must be non-negative", name)
	}
	return nil
}

func validatePositiveInt(name string, value int) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func validatePositiveIntegerString(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}

	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return fmt.Errorf("%s must be a positive integer string", name)
		}
	}

	if value == "0" {
		return fmt.Errorf("%s must be positive", name)
	}

	return nil
}
