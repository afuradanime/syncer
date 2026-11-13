package tests

import (
	"encoding/json"
	"testing"

	"syncer/src/scrape"
	"syncer/src/store"

	jikan "github.com/afuradanime/tenrai-go"
)

// Tests whether the persister uses the correct map for each column and
// if the fields are correctly stored as their slug counterparts in DB.
func TestPersister_MapsFieldsThroughSlugs(t *testing.T) {
	f := NewDBFixture(t)

	var relations jikan.AnimeRelations
	jsonRelationFromTenrai := `{"data": [
		{
			"relation": "Spin-Off", 
			"entry": [{"mal_id": 2, "type": "anime", "name": "Other anime"}]}
	]}`

	if err := json.Unmarshal([]byte(jsonRelationFromTenrai), &relations); err != nil {
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

	requireString(t, f.DB, `SELECT t.name FROM anime a JOIN anime_type t ON t.id = a.type_id WHERE a.id = 1`,
		store.AnimeTypeSlugs["TV"])
	requireString(t, f.DB, `SELECT s.name FROM anime a JOIN anime_status s ON s.id = a.status_id WHERE a.id = 1`,
		store.AnimeStatusSlugs["Finished Airing"])
	requireString(t, f.DB, `SELECT source FROM anime WHERE id = 1`,
		store.AnimeSourceSlugs["4-koma manga"])
	requireString(t, f.DB, `SELECT r.name FROM anime_relations ar JOIN relation_types r ON r.id = ar.relation_type_id WHERE ar.anime_id = 1`,
		store.RelationTypeSlugs["Spin-Off"])
	requireString(t, f.DB, `SELECT role FROM anime_companies WHERE company_id = 10`,
		store.CompanyRoleProducer)
	requireString(t, f.DB, `SELECT role FROM anime_companies WHERE company_id = 11`,
		store.CompanyRoleStudio)
	requireString(t, f.DB, `SELECT type FROM tags WHERE id = 12`,
		store.TagExplicitGenre)
	requireString(t, f.DB, `SELECT type FROM anime_synonyms WHERE title = 'Some anime (EN)'`,
		store.SynonymEnglish)
	requireString(t, f.DB, `SELECT type FROM anime_synonyms WHERE title = 'なにか'`,
		store.SynonymJapanese)
	requireString(t, f.DB, `SELECT type FROM anime_synonyms WHERE title = 'Alias'`,
		store.SynonymOther)
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
