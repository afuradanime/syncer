package tests

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"context"
	"path/filepath"
	"syncer/src/config"
	"syncer/src/publish"
	"syncer/src/scrape"
	"syncer/src/store"
	"syncer/src/sync"
	"syncer/tests/unit/mock"

	jikan "github.com/afuradanime/tenrai-go"
)

type DBFixture struct {
	Path string
	DB   *sql.DB
	t    *testing.T
}

// opens a database at a temp path and applies the schema
func NewDBFixture(t *testing.T) *DBFixture {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "anime.db")
	db, err := sync.OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := sync.CreateDatabaseSchema(path, db); err != nil {
		db.Close()
		t.Fatalf("create schema: %v", err)
	}
	f := &DBFixture{Path: path, DB: db, t: t}
	t.Cleanup(func() { _ = f.DB.Close() })
	return f
}

func (f *DBFixture) Publish(dest string) string {
	f.t.Helper()
	if err := f.DB.Close(); err != nil {
		f.t.Fatalf("close fixture db: %v", err)
	}
	if err := publish.SetupDatabasePragmas(f.DB); err != nil {
		// pragmas applied before close in real code; here it's a no-op
	}
	if err := publish.PublishDatabase(f.Path, dest, "test"); err != nil {
		f.t.Fatalf("publish: %v", err)
	}
	db, err := sync.OpenDatabase(f.Path)
	if err != nil {
		f.t.Fatalf("reopen fixture db: %v", err)
	}
	f.DB = db
	return dest
}

type SyncFixture struct {
	API    *mock.Jikan
	Sync   sync.Syncer
	DBPath string
	cfg    config.Config
}

// NewSyncFixture creates a fresh Jikan mock and a Syncer pointing at a
// fresh temp database path
func NewSyncFixture(t *testing.T) *SyncFixture {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "anime.db")
	cfg := config.Config{DbPath: dbPath, MaxAttempts: 1}
	return &SyncFixture{
		API:    mock.NewJikan(t),
		Sync:   sync.Syncer{Config: cfg},
		DBPath: dbPath,
		cfg:    cfg,
	}
}

func searchQuery(page int) url.Values {
	return url.Values{
		"page":     {strconv.Itoa(page)},
		"limit":    {"50"},
		"order_by": {"mal_id"},
		"sort":     {"asc"},
	}
}

func searchPageResponse(firstID, count, page, lastPage int) mock.JikanResponse {
	items := make([]map[string]any, count)
	for i := range items {
		items[i] = map[string]any{"mal_id": firstID + i}
	}
	return mock.JikanResponse{Body: map[string]any{
		"data": items,
		"pagination": map[string]any{
			"has_next_page":     page < lastPage,
			"last_visible_page": lastPage,
		},
	}}
}

// animeRecord and mangaRecord build typed API records for the mock.
func animeRecord(id int, title string) jikan.AnimeBase {
	return jikan.AnimeBase{MalId: id, Title: title, Type: "TV", Status: "Finished Airing"}
}

func mangaRecord(id int, title string) jikan.MangaBase {
	return jikan.MangaBase{MalId: id, Title: title, Type: "Manga", Status: "Finished"}
}

func requireTable(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?
		)`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %q: %v", table, err)
	}
	if !exists {
		t.Fatalf("table %q does not exist", table)
	}
}

func requireCount(t *testing.T, db *sql.DB, query string, want int, args ...any) {
	t.Helper()
	var got int
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("%q = %d, want %d", query, got, want)
	}
}

func requireTitle(t *testing.T, db *sql.DB, table string, id int, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow("SELECT title FROM "+table+" WHERE id = ?", id).Scan(&got); err != nil {
		t.Fatalf("query %s id=%d: %v", table, id, err)
	}
	if got != want {
		t.Fatalf("%s id=%d title = %q, want %q", table, id, got, want)
	}
}

// SeedAnime inserts anime rows through the real persister so the fixture
// matches what a prior sync would leave behind.
func SeedAnime(t *testing.T, db *sql.DB, cfg config.Config, rows ...jikan.AnimeBase) {
	t.Helper()
	lookups, err := store.LoadLookups(db)
	if err != nil {
		t.Fatalf("load lookups: %v", err)
	}
	p := store.NewPersister(db, lookups, cfg)
	for _, a := range rows {
		if err := p.InsertScrapedAnime(context.Background(), scrape.ScrapedAnime{Anime: a}); err != nil {
			t.Fatalf("seed anime %d: %v", a.MalId, err)
		}
	}
}

func SeedManga(t *testing.T, db *sql.DB, cfg config.Config, rows ...jikan.MangaBase) {
	t.Helper()
	lookups, err := store.LoadLookups(db)
	if err != nil {
		t.Fatalf("load lookups: %v", err)
	}
	p := store.NewPersister(db, lookups, cfg)
	for _, m := range rows {
		if err := p.InsertScrapedManga(context.Background(), scrape.ScrapedManga{Manga: m}); err != nil {
			t.Fatalf("seed manga %d: %v", m.MalId, err)
		}
	}
}

// scrapeAll runs the full-scrape entrypoint for the given kind and returns
// the MAL IDs that came through, failing on any unexpected result shape.
func scrapeAll(t *testing.T, kind string, ctx context.Context, cfg config.Config) ([]int, error) {
	t.Helper()
	var ids []int
	if kind == "anime" {
		results := make(chan scrape.ScrapedAnime, 100)
		err := scrape.ScrapeAllAnime(ctx, results, cfg)
		for item := range results {
			if item.Error != nil || item.Relations != nil {
				t.Fatalf("unexpected anime result: %+v", item)
			}
			ids = append(ids, item.Anime.MalId)
		}
		return ids, err
	}
	results := make(chan scrape.ScrapedManga, 100)
	err := scrape.ScrapeAllManga(ctx, results, cfg)
	for item := range results {
		if item.Error != nil || item.Relations != nil {
			t.Fatalf("unexpected manga result: %+v", item)
		}
		ids = append(ids, item.Manga.MalId)
	}
	return ids, err
}

// countRequestsByPath returns (pageCount, refreshCount) for the given base path.
func countRequestsByPath(requests []*http.Request, basePath string) (pages, refreshes int) {
	for _, r := range requests {
		if r.URL.Path == basePath {
			pages++
		} else {
			refreshes++
		}
	}
	return
}

// insertScrapedAnime stores items through the real persister.
func insertScrapedAnime(t *testing.T, db *sql.DB, items ...scrape.ScrapedAnime) {
	t.Helper()
	lookups, err := store.LoadLookups(db)
	if err != nil {
		t.Fatalf("load lookups: %v", err)
	}
	p := store.NewPersister(db, lookups, config.Config{})
	for _, item := range items {
		if err := p.InsertScrapedAnime(context.Background(), item); err != nil {
			t.Fatalf("insert anime %d: %v", item.Anime.MalId, err)
		}
	}
}

func requireString(t *testing.T, db *sql.DB, query string, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("%q = %q, want %q", query, got, want)
	}
}
