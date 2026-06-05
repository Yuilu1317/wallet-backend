package db

import (
	"strings"
	"testing"
)

func TestOpenPostgres_WithEmptyDSN_ReturnsError(t *testing.T) {
	t.Parallel()

	got, err := OpenPostgres("")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != nil {
		t.Fatalf("expected nil db, got %v", got)
	}

	if !strings.Contains(err.Error(), "database dsn is required") {
		t.Fatalf("expected error to contain database dsn is required, got %q", err.Error())
	}
}

func TestClose_WithNilDB_ReturnsNil(t *testing.T) {
	t.Parallel()

	err := Close(nil)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
