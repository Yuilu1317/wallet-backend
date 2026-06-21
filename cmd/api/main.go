package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Yuilu1317/wallet-backend/internal/app"
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

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM)
	defer stop()

	application, err := app.New(configPath)
	if err != nil {
		log.Fatalf("initialize app: %v", err)
	}

	runErr := application.Run(ctx)

	closeErr := application.Close()
	if closeErr != nil {
		log.Printf("close app: %v", closeErr)
	}

	if runErr != nil {
		log.Fatalf("run app: %v", runErr)
	}
}
