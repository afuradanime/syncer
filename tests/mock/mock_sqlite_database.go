package mock

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = database.Close()
	})

	if _, err := database.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	migrationPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", "0001_init.up.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	if _, err := database.Exec(string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	return database
}
