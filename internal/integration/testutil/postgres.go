package testutil

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func replacePostgresDSNDatabase(t *testing.T, dsn string, dbName string) string {
	t.Helper()

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse url dsn: %s", err)
		}
		u.Path = "/" + dbName
		return u.String()
	}

	paths := strings.Fields(dsn)
	replace := false

	for i, path := range paths {
		if strings.HasPrefix(path, "dbname=") {
			paths[i] = "dbname=" + dbName
			replace = true
			break
		}
	}
	if !replace {
		paths = append(paths, "dbname="+dbName)
	}
	return strings.Join(paths, " ")
}

func migrateTestTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlByte, err := os.ReadFile("../../migrations/001_init_schema.sql")
	if err != nil {
		t.Fatalf("read migration schema: %v", err)
	}

	if err := db.Exec(string(sqlByte)).Error; err != nil {
		t.Fatalf("migration test tables: %v", err)
	}
}

func loadTestEnv(t *testing.T) {
	t.Helper()
	_ = godotenv.Load("../../.env.test")
}

func CreateTempPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	loadTestEnv(t)

	adminDSN := strings.TrimSpace(os.Getenv("TEST_ADMIN_DSN"))
	if adminDSN == "" {
		t.Skip("admin dsn is not set")
	}
	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	adminSQLDB, err := adminDB.DB()
	if err != nil {
		t.Fatalf("open admin sql db: %v", err)
	}

	t.Cleanup(func() {
		_ = adminSQLDB.Close()
	})

	testDBName := fmt.Sprintf(
		"wallet_backend_test_%d_%d",
		time.Now().UnixNano(),
		os.Getpid())

	if err := adminDB.Exec(`CREATE DATABASE "` + testDBName + `"`).Error; err != nil {
		t.Fatalf("create test db: %v", err)
	}

	testDSN := replacePostgresDSNDatabase(t, adminDSN, testDBName)

	testDB, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	testSQLDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("open test sql db: %v", err)
	}

	t.Cleanup(func() {
		_ = testSQLDB.Close()

		_ = adminDB.Exec(`
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = ?
AND pid <> pg_backend_pid()`,
			testDBName).Error

		if err := adminDB.Exec(`DROP DATABASE IF EXISTS "` + testDBName + `"`).Error; err != nil {
			t.Fatalf("drop test db%s: %v", testDBName, err)
		}
	})
	migrateTestTables(t, testDB)
	return testDB
}

func TestCreateTempPostgresTestDB_MigratesTable(t *testing.T) {
	db := CreateTempPostgresTestDB(t)
	var exists bool
	if err := db.Raw(`
SELECT EXISTS(
SELECT 1
FROM information_schema.tables
WHERE table_schema = 'public'
AND table_name = 'deposit_addresses'
)
`).Scan(&exists).Error; err != nil {
		t.Fatalf("check deposit_addresses table exists: %v", err)
	}
	if !exists {
		t.Fatal("expect deposit_addresses table to exists after migrating")
	}
}
