package tests

import (
	"context"
	"testing"

	"syncer/src/config"
	"syncer/src/scrape"
	"syncer/src/store"
	"syncer/src/sync"

	jikan "github.com/afuradanime/tenrai-go"
)

func TestInsertScrapedManga(t *testing.T) {
	db, err := sync.OpenDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer sync.CloseDatabase(db)
	if err := sync.CreateDatabaseSchema(":memory:", db); err != nil {
		t.Fatal(err)
	}

	lookups, err := store.LoadLookups(db)
	if err != nil {
		t.Fatal(err)
	}
	persister := store.NewPersister(db, lookups, config.Config{})
	if _, err := db.Exec(`INSERT INTO mangas (id, title, kind, status) VALUES (4, 'Related', 'Manga', 'Finished')`); err != nil {
		t.Fatal(err)
	}

	manga := jikan.MangaBase{
		MalId:        1,
		Title:        "Manga",
		TitleEnglish: "Manga English",
		Type:         "Manga",
		Status:       "Finished",
		Synopsis:     "Synopsis",
		Background:   "Background",
		Authors:      []jikan.MalItem{{MalId: 2, Name: "Author", Type: "Story"}},
		Genres:       []jikan.MalItem{{MalId: 3, Name: "Action", Type: "genre"}},
	}
	relations := jikan.MangaRelations{}
	relations.Data = append(relations.Data, struct {
		Relation string          `json:"relation"`
		Entry    []jikan.MalItem `json:"entry"`
	}{Relation: "Sequel", Entry: []jikan.MalItem{{MalId: 4, Type: "manga"}}})

	if err := persister.InsertScrapedManga(context.Background(), scrape.ScrapedManga{
		Manga:     manga,
		Relations: &relations,
	}); err != nil {
		t.Fatal(err)
	}

	var title string
	if err := db.QueryRow("SELECT title FROM mangas WHERE id = 1").Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Manga" {
		t.Fatalf("title = %q, want Manga", title)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM manga_authors WHERE manga_id = 1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("authors = %d, want 1", count)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM manga_tags WHERE manga_id = 1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("tags = %d, want 1", count)
	}
}
