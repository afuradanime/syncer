package tests

import (
	"os"
	"path/filepath"
	"testing"

	"syncer/src/sync"
)

func TestCreateTempDatabase(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "anime.db")

	path, err := sync.CreateTempDatabase(destination)
	if err != nil {
		t.Fatalf("CreateTempDatabase() error = %v", err)
	}
	defer os.Remove(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("temporary database does not exist: %v", err)
	}

	if info.IsDir() {
		t.Fatalf("temporary database path is a directory")
	}

	if filepath.Dir(path) != dir {
		t.Fatalf(
			"temporary database directory = %q, want %q",
			filepath.Dir(path),
			dir,
		)
	}
}

func TestOpenDatabase(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "anime.db")

	db, err := sync.OpenDatabase(databasePath)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	defer sync.CloseDatabase(db)

	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		t.Fatalf("query database: %v", err)
	}

	if result != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", result)
	}
}

func TestCreateDatabaseSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anime.db")

	db, err := sync.OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	defer sync.CloseDatabase(db)

	if err := sync.CreateDatabaseSchema(path, db); err != nil {
		t.Fatalf("CreateDatabaseSchema() error = %v", err)
	}

	expectedTables := []string{
		"anime",
		"anime_type",
		"anime_status",
		"tags",
		"anime_tags",
	}

	for _, table := range expectedTables {
		t.Run(table, func(t *testing.T) {
			var exists bool

			err := db.QueryRow(`
				SELECT EXISTS (
					SELECT 1
					FROM sqlite_master
					WHERE type = 'table'
					  AND name = ?
				)
			`, table).Scan(&exists)

			if err != nil {
				t.Fatalf("query table: %v", err)
			}

			if !exists {
				t.Fatalf("table %q does not exist", table)
			}
		})
	}
}
