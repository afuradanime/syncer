package tests

import (
	"os"
	"path/filepath"
	"testing"

	"syncer/src/sync"
)

// NFR-02: temporary database is written alongside the destination.
func TestCreateTempDatabase_WritesBesideDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "anime.db")

	path, err := sync.CreateTempDatabase(destination)
	if err != nil {
		t.Fatalf("CreateTempDatabase: %v", err)
	}
	defer os.Remove(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat temp db: %v", err)
	}
	if info.IsDir() {
		t.Fatal("temp db path is a directory")
	}
	if got, want := filepath.Dir(path), dir; got != want {
		t.Fatalf("temp db dir = %q, want %q", got, want)
	}
}

// NFR-01: published database is owner-writable and supports runtime edits.
func TestPublishedDatabase_IsWritable(t *testing.T) {
	f := NewDBFixture(t)
	dest := filepath.Join(filepath.Dir(f.Path), "published.db")
	f.Publish(dest)

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0200 == 0 {
		t.Fatalf("published db mode = %v, want owner-writable", info.Mode())
	}
}

// NFR-01: runtime edits survive after pragmas are applied.
func TestPublishedDatabase_AcceptsRuntimeEdits(t *testing.T) {
	f := NewDBFixture(t)
	dest := filepath.Join(filepath.Dir(f.Path), "published.db")
	f.Publish(dest)

	edited, err := sync.OpenDatabase(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer edited.Close()

	if _, err := edited.Exec(
		`CREATE TABLE runtime_edits (value TEXT);
		 INSERT INTO runtime_edits VALUES ('edited')`,
	); err != nil {
		t.Fatalf("runtime edit: %v", err)
	}
}

// Sanity: OpenDatabase returns a usable connection.
func TestOpenDatabase_QueriesSucceed(t *testing.T) {
	f := NewDBFixture(t)
	defer sync.CloseDatabase(f.DB)

	var got int
	if err := f.DB.QueryRow("SELECT 1").Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", got)
	}
}

// FR-02: schema creates all the tables a sync relies on.
func TestCreateDatabaseSchema_CreatesExpectedTables(t *testing.T) {
	f := NewDBFixture(t)
	for _, table := range []string{
		"anime",
		"anime_type",
		"anime_status",
		"tags",
		"anime_tags",
		"mangas",
		"manga_tags",
		"manga_relations",
		"manga_synonyms",
		"manga_descriptions",
		"people",
		"companies",
	} {
		t.Run(table, func(t *testing.T) {
			requireTable(t, f.DB, table)
		})
	}
}
