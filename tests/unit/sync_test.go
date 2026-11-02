package tests

import (
	"testing"

	"syncer/src/config"
	"syncer/src/publish"
	"syncer/src/sync"
	"syncer/tests/unit/mock"

	jikan "github.com/afuradanime/tenrai-go"
)

// FR-01 / NFR-02: a partial sync refreshes seeded records, walks new pages,
// preserves untouched rows, and publishes a matching manifest.
func TestRunPartialSync_UpdatesAndPublishes(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		for _, refreshFails := range []bool{false, true} {
			t.Run(kind+"/refreshFails="+boolStr(refreshFails), func(t *testing.T) {
				testRunPartialSync(t, kind, refreshFails)
			})
		}
	}
}

func testRunPartialSync(t *testing.T, kind string, refreshFails bool) {
	t.Helper()
	sf := NewSyncFixture(t)
	dest := sf.DBPath

	seed := NewDBFixture(t)
	seed.Path = sf.DBPath

	seedDB, err := sync.OpenDatabase(sf.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sync.CreateDatabaseSchema(sf.DBPath, seedDB); err != nil {
		t.Fatal(err)
	}
	seed.DB = seedDB
	defer seed.DB.Close()

	seedCfg := config.Config{DbPath: dest, MaxAttempts: 1}
	if kind == "anime" {
		SeedAnime(t, seed.DB, seedCfg,
			animeRecord(1, "Untouched"),
			jikan.AnimeBase{MalId: 2, Title: "Old active", Type: "TV", Status: "Currently Airing", Airing: true},
			animeRecord(100, "Last known"),
		)
	} else {
		SeedManga(t, seed.DB, seedCfg,
			mangaRecord(1, "Untouched"),
			jikan.MangaBase{MalId: 2, Title: "Old active", Type: "Manga", Status: "Publishing"},
			mangaRecord(100, "Last known"),
		)
	}
	seed.Publish(dest)

	// Mock partial-sync traffic
	path, otherPath, table := "/v1/anime", "/v1/manga", "anime"
	record := func(id int, title string) any { return animeRecord(id, title) }
	if kind == "manga" {
		path, otherPath, table = "/v1/manga", "/v1/anime", "mangas"
		record = func(id int, title string) any { return mangaRecord(id, title) }
	}

	refresh := mock.JikanResponse{Body: map[string]any{"data": record(2, "Refreshed")}}
	if refreshFails {
		refresh = mock.JikanResponse{StatusCode: 404, Body: map[string]any{"status": 404}}
	}
	sf.API.ExpectGET(path+"/2", nil, refresh)
	sf.API.ExpectGET(path, searchQuery(1), searchPageResponse(1, 2, 1, 3))
	resumePage := mock.JikanResponse{Body: map[string]any{
		"data":       []any{record(100, "Updated existing"), record(101, "New on resume page")},
		"pagination": map[string]any{"has_next_page": true, "last_visible_page": 3},
	}}
	sf.API.ExpectGET(path, searchQuery(2), resumePage, resumePage)
	sf.API.ExpectGET(path, searchQuery(3), mock.JikanResponse{Body: map[string]any{
		"data":       []any{record(150, "New on next page")},
		"pagination": map[string]any{"has_next_page": false, "last_visible_page": 3},
	}})
	sf.API.ExpectGET(otherPath, searchQuery(1), searchPageResponse(1, 0, 1, 1))

	if err := sf.Sync.RunPartialSync(); err != nil {
		t.Fatalf("RunPartialSync: %v", err)
	}

	published, err := sync.OpenDatabase(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer published.Close()

	want := map[int]string{
		1:   "Untouched",
		2:   "Refreshed",
		100: "Updated existing",
		101: "New on resume page",
		150: "New on next page",
	}
	if refreshFails {
		want[2] = "Old active"
	}
	for id, title := range want {
		requireTitle(t, published, table, id, title)
	}
	requireCount(t, published, "SELECT COUNT(*) FROM "+table, len(want))
	requireCount(t, published, "SELECT COUNT(*) FROM anime_relations", 0)
	requireCount(t, published, "SELECT COUNT(*) FROM manga_relations", 0)

	manifest, err := publish.ReadManifest(dest)
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := publish.CalculateChecksum(dest, manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Checksum != *checksum {
		t.Fatal("manifest checksum does not match published database")
	}
}

// NFR-01: a partial sync preserves user-created tables / rows.
func TestRunPartialSync_PreservesRuntimeEdits(t *testing.T) {
	sf := NewSyncFixture(t)

	// Publish an empty database as the "previous" state.
	seed := NewDBFixture(t)
	seed.Publish(sf.DBPath)

	// Add a runtime table on the published file.
	edited, err := sync.OpenDatabase(sf.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edited.Exec(
		`CREATE TABLE runtime_edits (value TEXT);
		 INSERT INTO runtime_edits VALUES ('edited')`,
	); err != nil {
		t.Fatal(err)
	}
	if err := edited.Close(); err != nil {
		t.Fatal(err)
	}

	// No new data: both endpoints return empty pages.
	sf.API.ExpectGET("/v1/anime", searchQuery(1), searchPageResponse(1, 0, 1, 1))
	sf.API.ExpectGET("/v1/manga", searchQuery(1), searchPageResponse(1, 0, 1, 1))

	if err := sf.Sync.RunPartialSync(); err != nil {
		t.Fatalf("RunPartialSync: %v", err)
	}
	if got := len(sf.API.Requests()); got != 2 {
		t.Fatalf("search requests = %d, want 2", got)
	}

	published, err := sync.OpenDatabase(sf.DBPath)
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

// FR-02: a full sync publishes a database with all expected tables.
func TestRunFullSync_PublishesAllTables(t *testing.T) {
	sf := NewSyncFixture(t)
	mockFullSync(t, sf, "anime")
	if err := sf.Sync.RunFullSync(); err != nil {
		t.Fatalf("RunFullSync: %v", err)
	}

	db, err := sync.OpenDatabase(sf.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sync.CloseDatabase(db)

	for _, table := range []string{
		"anime", "anime_type", "anime_status", "tags", "anime_tags",
		"mangas", "manga_relations",
	} {
		t.Run(table, func(t *testing.T) { requireTable(t, db, table) })
	}
	requireTitle(t, db, "anime", 1, "Test anime")
}

// FR-02: a full sync overwrites an existing database file.
func TestRunFullSync_OverwritesExistingDatabase(t *testing.T) {
	sf := NewSyncFixture(t)

	seed := NewDBFixture(t)
	seed.Publish(sf.DBPath)

	// Leave a stale marker in the destination.
	seeded, err := sync.OpenDatabase(sf.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seeded.Exec("CREATE TABLE stale_marker (value TEXT); INSERT INTO stale_marker VALUES ('stale')"); err != nil {
		seeded.Close()
		t.Fatal(err)
	}
	if err := seeded.Close(); err != nil {
		t.Fatal(err)
	}

	// Now run a full sync with a real record.
	mockFullSync(t, sf, "anime")
	if err := sf.Sync.RunFullSync(); err != nil {
		t.Fatalf("RunFullSync: %v", err)
	}

	db, err := sync.OpenDatabase(sf.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireTitle(t, db, "anime", 1, "Test anime")
	requireCount(t, db, "SELECT COUNT(*) FROM anime", 1)
}

// mockFullSync wires the two endpoints a full sync always hits.
func mockFullSync(t *testing.T, sf *SyncFixture, kind string) {
	t.Helper()
	anime := []jikan.AnimeBase{}
	manga := []jikan.MangaBase{}
	if kind == "anime" {
		anime = append(anime, animeRecord(1, "Test anime"))
	} else {
		manga = append(manga, mangaRecord(1, "Test mangas"))
	}
	sf.API.ExpectGET("/v1/anime", searchQuery(1), mock.JikanResponse{Body: map[string]any{
		"data":       anime,
		"pagination": map[string]any{"has_next_page": false, "last_visible_page": 1},
	}})
	sf.API.ExpectGET("/v1/manga", searchQuery(1), mock.JikanResponse{Body: map[string]any{
		"data":       manga,
		"pagination": map[string]any{"has_next_page": false, "last_visible_page": 1},
	}})
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
