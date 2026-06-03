package main

import (
	"log"
	"os"

	"github.com/Yuilu1317/wallet-backend/internal/config"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("load .env skipped: %v", err)
	}
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "configs/config.local.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	log.Printf(
		"config loaded: env=%s http_port=%s chain_id=%d scanner=%s explorer=%s",
		cfg.App.Env,
		cfg.App.HTTPPort,
		cfg.Ethereum.ChainID,
		cfg.Scanner.Name,
		cfg.Explorer.BaseURL,
	)
}
