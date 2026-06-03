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
	DSN string `yaml:"dsn"`
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

	if c.Ethereum.ChainID <= 0 {
		return fmt.Errorf("ethereum.chain_id must be positive")
	}

	if c.Ethereum.ConfirmationDepth <= 0 {
		return fmt.Errorf("ethereum.confirmation_depth must be positive")
	}

	if err := validateNonNegativeIntegerString("ethereum.min_deposit_wei", c.Ethereum.MinDepositWei); err != nil {
		return err
	}

	if err := validateRequiredString("scanner.name", c.Scanner.Name); err != nil {
		return err
	}

	if c.Scanner.StartBlock < 0 {
		return fmt.Errorf("scanner.start_block must be non-negative")
	}

	if c.Scanner.BatchSize <= 0 {
		return fmt.Errorf("scanner.batch_size must be positive")
	}

	if c.Scanner.PollIntervalSeconds <= 0 {
		return fmt.Errorf("scanner.poll_interval_seconds must be positive")
	}

	if err := validateRequiredString("explorer.base_url", c.Explorer.BaseURL); err != nil {
		return err
	}

	if c.Explorer.TimeoutSeconds <= 0 {
		return fmt.Errorf("explorer.timeout_seconds must be positive")
	}

	return nil
}

func validateRequiredString(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func validateNonNegativeIntegerString(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}

	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return fmt.Errorf("%s must be a non-negative integer string", name)
		}
	}

	return nil
}
