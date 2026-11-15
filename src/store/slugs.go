package store

import "fmt"

var AnimeTypeSlugs = map[string]string{
	"TV":      "tv",
	"OVA":     "ova",
	"Movie":   "movie",
	"Special": "special",
	"ONA":     "ona",
	"Music":   "music",
}

var AnimeStatusSlugs = map[string]string{
	"Finished Airing":  "finished-airing",
	"Currently Airing": "currently-airing",
	"Not yet aired":    "not-yet-aired",
}

var AnimeSourceSlugs = map[string]string{
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

var RelationTypeSlugs = map[string]string{
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

// Values the syncer assigns itself rather than reading from MyAnimeList
const (
	CompanyRoleProducer = "producer"
	CompanyRoleLicensor = "licensor"
	CompanyRoleStudio   = "studio"

	SynonymEnglish  = "english"
	SynonymJapanese = "japanese"
	SynonymOther    = "synonym"

	TagGenre         = "genre"
	TagExplicitGenre = "explicit-genre"
	TagTheme         = "theme"
	TagDemographic   = "demographic"
)

func toSlug(field string, animeID int, value string, slugs map[string]string) (string, bool) {
	if slug, ok := slugs[value]; ok {
		return slug, true
	}
	if value != "" && value != "Unknown" {
		fmt.Printf("[Persister] Unrecognised %s %q on anime %d, storing NULL (add it to src/store/slugs.go)\n", field, value, animeID)
	}
	return "", false
}

func slugOrNil(field string, animeID int, value string, slugs map[string]string) any {
	if slug, ok := toSlug(field, animeID, value, slugs); ok {
		return slug
	}
	return nil
}

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
