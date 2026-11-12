package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"syncer/src/config"
	"syncer/src/scrape"
	"syncer/src/store"

	jikan "github.com/afuradanime/tenrai-go"
)

// MyAnimeList's display text is stored as kebab-case slugs.
func TestPersister_StoresSlugs(t *testing.T) {
	f := NewDBFixture(t)

	var relations jikan.AnimeRelations
	if err := json.Unmarshal([]byte(`{"data": [
		{"relation": "Spin-Off", "entry": [{"mal_id": 2, "type": "anime", "name": "Other anime"}]}
	]}`), &relations); err != nil {
		t.Fatal(err)
	}

	insertScrapedAnime(t, f.DB, scrape.ScrapedAnime{
		Anime: jikan.AnimeBase{
			MalId:          1,
			Title:          "Some anime",
			TitleEnglish:   "Some anime (EN)",
			TitleJapanese:  "なにか",
			TitleSynonyms:  []string{"Alias"},
			Type:           "TV",
			Status:         "Finished Airing",
			Source:         "4-koma manga",
			Producers:      []jikan.MalItem{{MalId: 10, Name: "A producer"}},
			Studios:        []jikan.MalItem{{MalId: 11, Name: "A studio"}},
			ExplicitGenres: []jikan.MalItem{{MalId: 12, Name: "An explicit genre"}},
		},
		Relations: &relations,
	})

	requireString(t, f.DB, `SELECT t.name FROM anime a JOIN anime_type t ON t.id = a.type_id WHERE a.id = 1`, "tv")
	requireString(t, f.DB, `SELECT s.name FROM anime a JOIN anime_status s ON s.id = a.status_id WHERE a.id = 1`, "finished-airing")
	requireString(t, f.DB, `SELECT source FROM anime WHERE id = 1`, "4-koma-manga")
	requireString(t, f.DB, `SELECT r.name FROM anime_relations ar JOIN relation_types r ON r.id = ar.relation_type_id WHERE ar.anime_id = 1`, "spin-off")
	requireString(t, f.DB, `SELECT role FROM anime_companies WHERE company_id = 10`, "producer")
	requireString(t, f.DB, `SELECT role FROM anime_companies WHERE company_id = 11`, "studio")
	requireString(t, f.DB, `SELECT type FROM tags WHERE id = 12`, "explicit-genre")
	requireString(t, f.DB, `SELECT type FROM anime_synonyms WHERE title = 'Some anime (EN)'`, "english")
	requireString(t, f.DB, `SELECT type FROM anime_synonyms WHERE title = 'なにか'`, "japanese")
	requireString(t, f.DB, `SELECT type FROM anime_synonyms WHERE title = 'Alias'`, "synonym")
}

// "Unknown", empty and unrecognised values are stored as NULL rather than failing the anime.
func TestPersister_StoresUnknownValuesAsNull(t *testing.T) {
	f := NewDBFixture(t)

	insertScrapedAnime(t, f.DB,
		scrape.ScrapedAnime{Anime: jikan.AnimeBase{MalId: 1, Title: "Unknown everything", Type: "Unknown", Status: "Unknown", Source: "Unknown"}},
		scrape.ScrapedAnime{Anime: jikan.AnimeBase{MalId: 2, Title: "Empty everything"}},
		scrape.ScrapedAnime{Anime: jikan.AnimeBase{MalId: 3, Title: "Unrecognised everything", Type: "Hologram", Status: "Paused", Source: "Fridge magnet"}},
	)

	requireCount(t, f.DB, `SELECT COUNT(*) FROM anime WHERE type_id IS NULL AND status_id IS NULL AND source IS NULL`, 3)
}

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
