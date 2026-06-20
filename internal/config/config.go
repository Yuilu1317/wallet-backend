package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App      AppConfig      `yaml:"app"`
	Database DatabaseConfig `yaml:"database"`
	Ethereum EthereumConfig `yaml:"ethereum"`
	Scanner  ScannerConfig  `yaml:"scanner"`
	Explorer ExplorerConfig `yaml:"explorer"`
}

type AppConfig struct {
	Env      string `yaml:"env"`
	HTTPPort string `yaml:"http_port"`
}

type DatabaseConfig struct {
	DSN              string `yaml:"dsn"`
	DBTimeoutSeconds int    `yaml:"db_timeout_seconds"`
}

type EthereumConfig struct {
	ChainID           int64  `yaml:"chain_id"`
	ConfirmationDepth int64  `yaml:"confirmation_depth"`
	MinDepositWei     string `yaml:"min_deposit_wei"`
}

type ScannerConfig struct {
	Name                string `yaml:"name"`
	StartBlock          int64  `yaml:"start_block"`
	BatchSize           int    `yaml:"batch_size"`
	PollIntervalSeconds int    `yaml:"poll_interval_seconds"`
}

type ExplorerConfig struct {
	BaseURL        string `yaml:"base_url"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

func Load(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("config path is required")
	}

	data, err := os.ReadFile(path)
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

	if err := validatePositiveInt("scanner.poll_interval_seconds", c.Scanner.PollIntervalSeconds); err != nil {
		return err
	}

	if err := validateRequiredString("explorer.base_url", c.Explorer.BaseURL); err != nil {
		return err
	}

	if err := validatePositiveInt("explorer.timeout_seconds", c.Explorer.TimeoutSeconds); err != nil {
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
