package store

import "fmt"

/*
* MyAnimeList (through tenrai-go) sends these fields as display text ("TV",
* "Finished Airing", "4-koma manga"). They should be stored as kebab-case slugs
* instead, so the db, the backend and its API all use the same stable values, and
* the responsibility to turn them into display text is delegated to the frontend.
*
* A value that isn't listed here is stored as NULL. "Unknown" and empty values
* are also expected and anything else is logged so it can be added later.
 */

var animeTypeSlugs = map[string]string{
	"TV":      "tv",
	"OVA":     "ova",
	"Movie":   "movie",
	"Special": "special",
	"ONA":     "ona",
	"Music":   "music",
}

var animeStatusSlugs = map[string]string{
	"Finished Airing":  "finished-airing",
	"Currently Airing": "currently-airing",
	"Not yet aired":    "not-yet-aired",
}

var animeSourceSlugs = map[string]string{
	"Original":     "original",
	"Manga":        "manga",
	"4-koma manga": "4-koma-manga",
	"Web manga":    "web-manga",
	"Light novel":  "light-novel",
	"Novel":        "novel",
	"Web novel":    "web-novel",
	"Visual novel": "visual-novel",
	"Game":         "game",
	"Card game":    "card-game",
	"Picture book": "picture-book",
	"Book":         "book",
	"Mixed media":  "mixed-media",
	"Music":        "music",
	"Radio":        "radio",
	"Other":        "other",
}

var relationTypeSlugs = map[string]string{
	"Parent Story":        "parent-story",
	"Sequel":              "sequel",
	"Prequel":             "prequel",
	"Side Story":          "side-story",
	"Full Story":          "full-story",
	"Summary":             "summary",
	"Spin-Off":            "spin-off",
	"Adaptation":          "adaptation",
	"Character":           "character",
	"Alternative Version": "alternative-version",
	"Alternative Setting": "alternative-setting",
	"Other":               "other",
}

// Values the syncer assigns itself rather than reading from MyAnimeList.
const (
	companyRoleProducer = "producer"
	companyRoleLicensor = "licensor"
	companyRoleStudio   = "studio"

	synonymEnglish  = "english"
	synonymJapanese = "japanese"
	synonymOther    = "synonym"

	tagGenre         = "genre"
	tagExplicitGenre = "explicit-genre"
	tagTheme         = "theme"
	tagDemographic   = "demographic"
)

// toSlug returns the slug for a MyAnimeList value, or false if it has none.
// Unrecognised values are logged; "" and "Unknown" are not, since they're expected.
func toSlug(field string, animeID int, value string, slugs map[string]string) (string, bool) {
	if slug, ok := slugs[value]; ok {
		return slug, true
	}
	if value != "" && value != "Unknown" {
		fmt.Printf("[Persister] Unrecognised %s %q on anime %d, storing NULL (add it to src/store/slugs.go)\n", field, value, animeID)
	}
	return "", false
}

// slugOrNil is toSlug for text columns: the slug, or nil to store NULL.
func slugOrNil(field string, animeID int, value string, slugs map[string]string) any {
	if slug, ok := toSlug(field, animeID, value, slugs); ok {
		return slug
	}
	return nil
}

// slugIDOrNil is toSlug for lookup columns: the id of the slug's row, or nil to store NULL.
func slugIDOrNil(field string, animeID int, value string, slugs map[string]string, idOf func(string) (int, bool)) any {
	slug, ok := toSlug(field, animeID, value, slugs)
	if !ok {
		return nil
	}
	id, ok := idOf(slug)
	if !ok {
		fmt.Printf("[Persister] %s %q has no row in its lookup table, storing NULL\n", field, slug)
		return nil
	}
	return id
}
