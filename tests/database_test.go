package tests

import (
	"os"
	"path/filepath"
	"testing"

	"syncer/src/publish"
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

func TestDatabaseRemainsWritable(t *testing.T) {
	dir := t.TempDir()
	tempPath := filepath.Join(dir, "temp.db")
	destPath := filepath.Join(dir, "published.db")
	db, err := sync.OpenDatabase(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := publish.SetupDatabasePragmas(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE runtime_edits (value TEXT)"); err != nil {
		t.Fatalf("write after setting pragmas: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := publish.PublishDatabase(tempPath, destPath, "test"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0200 == 0 {
		t.Fatal("published database is not owner-writable")
	}
	published, err := sync.OpenDatabase(destPath)
	if err != nil {
		t.Fatal(err)
	}
	defer published.Close()
	if _, err := published.Exec("INSERT INTO runtime_edits VALUES ('edited')"); err != nil {
		t.Fatalf("write published database: %v", err)
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
