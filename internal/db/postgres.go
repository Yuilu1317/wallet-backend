package db

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenPostgres opens a PostgreSQL connection through GORM, configures the
// underlying database/sql connection pool, and verifies connectivity with Ping.
//
// It does not run migrations or create tables. Database schema changes are
// managed by SQL migration files under the migrations directory.
func OpenPostgres(dsn string) (*gorm.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("database dsn is required")
	}

	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get postgres sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return gormDB, nil
}

// Close closes the underlying database/sql connection pool used by GORM.
//
// It only releases the current application's database connections. It does not
// stop the PostgreSQL server itself. Passing nil is treated as a no-op so that
// shutdown code can call Close safely even when the database was not initialized.
func Close(gormDB *gorm.DB) error {
	if gormDB == nil {
		return nil
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("get postgres sql db: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close postgres: %w", err)
	}
	return nil
}
