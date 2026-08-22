package tests

import (
	"path/filepath"
	"syncer/src/config"
	"syncer/src/sync"
	"testing"
)

func TestRunFull(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "anime.db")

	cfg := config.Config{
		DbPath: databasePath,
	}

	syncer := sync.Syncer{
		Config: cfg,
	}

	err := syncer.RunFullSync()
	if err != nil {
		t.Fatalf("RunFullSync() error = %v", err)
	}
}

func TestRunFullPublishesDatabase(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "anime.db")

	cfg := config.Config{
		DbPath: databasePath,
	}

	syncer := sync.Syncer{
		Config: cfg,
	}

	err := syncer.RunFullSync()
	if err != nil {
		t.Fatalf("RunFullSync() error = %v", err)
	}

	db, err := sync.OpenDatabase(databasePath)
	if err != nil {
		t.Fatalf("open published database: %v", err)
	}
	defer sync.CloseDatabase(db)

	expectedTables := []string{
		"anime",
		"anime_type",
		"anime_status",
		"tags",
		"anime_tags",
	}

	for _, table := range expectedTables {
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
			t.Fatalf("check table %q: %v", table, err)
		}

		if !exists {
			t.Fatalf("published database missing table %q", table)
		}
	}
}
