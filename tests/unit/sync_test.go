package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"syncer/src/config"
	"syncer/src/publish"
	"syncer/src/scrape"
	"syncer/src/store"
	"syncer/src/sync"
	"syncer/tests/mock"
	"testing"

	jikan "github.com/afuradanime/tenrai-go"
)

func TestRunPartialPublishesDatabase(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		for _, refreshFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/refreshFails=%t", kind, refreshFails), func(t *testing.T) {
				dir := t.TempDir()
				tempPath := filepath.Join(dir, "initial.db")
				databasePath := filepath.Join(dir, "published.db")
				cfg := config.Config{DbPath: databasePath, MaxAttempts: 1}
				db, err := sync.OpenDatabase(tempPath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if err := sync.CreateDatabaseSchema(tempPath, db); err != nil {
					t.Fatal(err)
				}
				lookups, err := store.LoadLookups(db)
				if err != nil {
					t.Fatal(err)
				}
				persister := store.NewPersister(db, lookups, cfg)
				for _, seed := range []struct {
					id    int
					title string
				}{{1, "Untouched"}, {2, "Old active"}, {100, "Last known"}} {
					if kind == "anime" {
						anime := jikan.AnimeBase{MalId: seed.id, Title: seed.title, Type: "TV", Status: "Finished Airing"}
						if seed.id == 2 {
							anime.Airing, anime.Status = true, "Currently Airing"
						}
						err = persister.InsertScrapedAnime(context.Background(), scrape.ScrapedAnime{Anime: anime})
					} else {
						manga := jikan.MangaBase{MalId: seed.id, Title: seed.title, Type: "Manga", Status: "Finished"}
						if seed.id == 2 {
							manga.Status = "Publishing"
						}
						err = persister.InsertScrapedManga(context.Background(), scrape.ScrapedManga{Manga: manga})
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if err := publish.SetupDatabasePragmas(db); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				if err := publish.PublishDatabase(tempPath, databasePath, "initial"); err != nil {
					t.Fatal(err)
				}

				api := mock.NewJikan(t)
				path, otherPath, table := "/v1/anime", "/v1/manga", "anime"
				if kind == "manga" {
					path, otherPath, table = "/v1/manga", "/v1/anime", "mangas"
				}
				record := func(id int, title string) any {
					if kind == "anime" {
						return jikan.AnimeBase{MalId: id, Title: title, Type: "TV", Status: "Finished Airing"}
					}
					return jikan.MangaBase{MalId: id, Title: title, Type: "Manga", Status: "Finished"}
				}
				refresh := mock.JikanResponse{Body: map[string]any{"data": record(2, "Refreshed")}}
				if refreshFails {
					refresh = mock.JikanResponse{StatusCode: 404, Body: map[string]any{"status": 404}}
				}
				api.ExpectGET(path+"/2", nil, refresh)
				api.ExpectGET(path, searchQuery(1), searchPageResponse(1, 2, 1, 3))
				resumePage := mock.JikanResponse{Body: map[string]any{
					"data":       []any{record(100, "Updated existing"), record(101, "New on resume page")},
					"pagination": map[string]any{"has_next_page": true, "last_visible_page": 3},
				}}
				api.ExpectGET(path, searchQuery(2), resumePage, resumePage)
				api.ExpectGET(path, searchQuery(3), mock.JikanResponse{Body: map[string]any{
					"data":       []any{record(150, "New on next page")},
					"pagination": map[string]any{"has_next_page": false, "last_visible_page": 3},
				}})
				api.ExpectGET(otherPath, searchQuery(1), searchPageResponse(1, 0, 1, 1))
				syncer := sync.Syncer{Config: cfg}
				if err := syncer.RunPartialSync(); err != nil {
					t.Fatalf("RunPartialSync: %v", err)
				}

				published, err := sync.OpenDatabase(databasePath)
				if err != nil {
					t.Fatal(err)
				}
				defer published.Close()
				expected := map[int]string{1: "Untouched", 2: "Refreshed", 100: "Updated existing", 101: "New on resume page", 150: "New on next page"}
				if refreshFails {
					expected[2] = "Old active"
				}
				for id, want := range expected {
					var title string
					if err := published.QueryRow("SELECT title FROM "+table+" WHERE id = ?", id).Scan(&title); err != nil {
						t.Fatalf("query %s %d: %v", kind, id, err)
					}
					if title != want {
						t.Fatalf("%s %d title = %q, want %q", kind, id, title, want)
					}
				}
				var count int
				if err := published.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != len(expected) {
					t.Fatalf("%s rows = %d, want %d", kind, count, len(expected))
				}
				for _, relations := range []string{"anime_relations", "manga_relations"} {
					if err := published.QueryRow("SELECT COUNT(*) FROM " + relations).Scan(&count); err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Fatalf("%s contains %d unexpected relations", relations, count)
					}
				}
				manifest, err := publish.ReadManifest(databasePath)
				if err != nil {
					t.Fatal(err)
				}
				checksum, err := publish.CalculateChecksum(databasePath, manifest.Version)
				if err != nil {
					t.Fatal(err)
				}
				if manifest.Checksum != *checksum {
					t.Fatal("partial sync manifest does not match the published database")
				}
			})
		}
	}
}

func TestPartialSyncAcceptsRuntimeEdits(t *testing.T) {
	dir := t.TempDir()
	tempPath := filepath.Join(dir, "temp.db")
	databasePath := filepath.Join(dir, "published.db")
	db, err := sync.OpenDatabase(tempPath)

	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := sync.CreateDatabaseSchema(tempPath, db); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := publish.PublishDatabase(tempPath, databasePath, "initial"); err != nil {
		t.Fatal(err)
	}

	edited, err := sync.OpenDatabase(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer edited.Close()

	if _, err := edited.Exec(
		`CREATE TABLE runtime_edits (value TEXT);
		INSERT INTO runtime_edits VALUES ('edited')`,
	); err != nil {
		t.Fatal(err)
	}

	if err := edited.Close(); err != nil {
		t.Fatal(err)
	}

	api := mock.NewJikan(t)
	api.ExpectGET("/v1/anime", searchQuery(1), searchPageResponse(1, 0, 1, 1))
	api.ExpectGET("/v1/manga", searchQuery(1), searchPageResponse(1, 0, 1, 1))
	syncer := sync.Syncer{Config: config.Config{DbPath: databasePath, MaxAttempts: 1}}
	if err := syncer.RunPartialSync(); err != nil {
		t.Fatalf("partial sync after runtime edit: %v", err)
	}
	if requests := len(api.Requests()); requests != 2 {
		t.Fatalf("search requests = %d, want 2", requests)
	}
	published, err := sync.OpenDatabase(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer published.Close()
	var value string
	if err := published.QueryRow("SELECT value FROM runtime_edits").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "edited" {
		t.Fatalf("runtime value = %q, want edited", value)
	}
}

func TestRunFull(t *testing.T) {
	mockFullSync(t, "anime")
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "anime.db")

	cfg := config.Config{
		DbPath:      databasePath,
		MaxAttempts: 1,
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
	for _, kind := range []string{"anime", "mangas"} {
		t.Run(kind, func(t *testing.T) {
			mockFullSync(t, kind)
			dir := t.TempDir()
			databasePath := filepath.Join(dir, "anime.db")

			cfg := config.Config{
				DbPath:      databasePath,
				MaxAttempts: 1,
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
				"mangas",
				"manga_relations",
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
			var title string
			if err := db.QueryRow("SELECT title FROM " + kind + " WHERE id = 1").Scan(&title); err != nil {
				t.Fatalf("query persisted %s: %v", kind, err)
			}
			if title != "Test "+kind {
				t.Fatalf("%s title = %q, want %q", kind, title, "Test "+kind)
			}
			for _, table := range []string{"anime_relations", "manga_relations"} {
				var count int
				if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("%s contains %d unexpected relations", table, count)
				}
			}
		})
	}
}

func mockFullSync(t *testing.T, kind string) {
	t.Helper()
	anime := []jikan.AnimeBase{}
	manga := []jikan.MangaBase{}
	if kind == "anime" {
		anime = append(anime, jikan.AnimeBase{MalId: 1, Title: "Test anime", Type: "TV", Status: "Finished Airing"})
	} else {
		manga = append(manga, jikan.MangaBase{MalId: 1, Title: "Test mangas", Type: "Manga", Status: "Finished"})
	}
	api := mock.NewJikan(t)
	api.ExpectGET("/v1/anime", searchQuery(1), mock.JikanResponse{Body: map[string]any{
		"data":       anime,
		"pagination": map[string]any{"has_next_page": false, "last_visible_page": 1},
	}})
	api.ExpectGET("/v1/manga", searchQuery(1), mock.JikanResponse{Body: map[string]any{
		"data":       manga,
		"pagination": map[string]any{"has_next_page": false, "last_visible_page": 1},
	}})
}
