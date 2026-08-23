package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"syncer/src/config"
	"syncer/src/scrape"
	"time"

	jikan "github.com/afuradanime/tenrai-go"
)

type Persister struct {
	db      *sql.DB
	lookups *Lookups
	opts    config.Config
}

func NewPersister(db *sql.DB, lookups *Lookups, opts config.Config) *Persister {
	return &Persister{db: db, lookups: lookups, opts: opts}
}

func (p *Persister) InsertScrapedManga(ctx context.Context, item scrape.ScrapedManga) error {
	if item.Error != nil {
		return fmt.Errorf("skipping manga with scrape error: %w", item.Error)
	}

	manga := item.Manga
	if manga.MalId == 0 {
		return fmt.Errorf("skipping manga without mal_id")
	}
	if manga.Title == "" {
		return fmt.Errorf("skipping manga %d without title", manga.MalId)
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for manga %d: %w", manga.MalId, err)
	}
	defer tx.Rollback()

	if err := p.insertMangaRow(tx, manga); err != nil {
		return err
	}
	if err := p.insertMangaSynonyms(tx, manga); err != nil {
		return err
	}
	if err := p.insertMangaDescription(tx, manga); err != nil {
		return err
	}
	if err := p.insertMangaAuthors(tx, manga); err != nil {
		return err
	}
	if err := p.insertMangaTags(tx, manga); err != nil {
		return err
	}
	if item.Relations != nil {
		if err := p.insertMangaRelations(tx, manga.MalId, *item.Relations); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit manga %d: %w", manga.MalId, err)
	}
	return nil
}

func (p *Persister) insertMangaRow(tx *sql.Tx, manga jikan.MangaBase) error {
	_, err := tx.Exec(`
		INSERT OR REPLACE INTO mangas
			(id, title, title_english, title_japanese, kind, status,
			 num_chapters, num_volumes, start_date, end_date,
			 cover_url_small, cover_url_large)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, manga.MalId, manga.Title, nullIfEmpty(manga.TitleEnglish), nullIfEmpty(manga.TitleJapanese),
		manga.Type, manga.Status, nullIfZero(manga.Chapters), nullIfZero(manga.Volumes),
		nullDate(manga.Published.From.Time), nullDate(manga.Published.To.Time),
		nullIfEmpty(manga.Images.Webp.SmallImageUrl), nullIfEmpty(manga.Images.Webp.LargeImageUrl))
	if err != nil {
		return fmt.Errorf("insert manga %d: %w", manga.MalId, err)
	}
	return nil
}

func (p *Persister) insertMangaDescription(tx *sql.Tx, manga jikan.MangaBase) error {
	var b strings.Builder
	if manga.Synopsis != "" {
		b.WriteString(manga.Synopsis)
	}
	if manga.Background != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(manga.Background)
	}
	if b.Len() == 0 {
		return nil
	}

	_, err := tx.Exec(`
		INSERT OR REPLACE INTO manga_descriptions (manga_id, description)
		VALUES (?, ?)
	`, manga.MalId, b.String())
	if err != nil {
		return fmt.Errorf("insert manga description for %d: %w", manga.MalId, err)
	}
	return nil
}

func (p *Persister) insertMangaSynonyms(tx *sql.Tx, manga jikan.MangaBase) error {
	if _, err := tx.Exec(`DELETE FROM manga_synonyms WHERE manga_id = ?`, manga.MalId); err != nil {
		return fmt.Errorf("clear manga synonyms for %d: %w", manga.MalId, err)
	}

	for _, synonym := range append([]struct{ typ, title string }{
		{"English", manga.TitleEnglish},
		{"Japanese", manga.TitleJapanese},
	}, func() []struct{ typ, title string } {
		result := make([]struct{ typ, title string }, 0, len(manga.TitleSynonyms))
		for _, title := range manga.TitleSynonyms {
			result = append(result, struct{ typ, title string }{"Synonym", title})
		}
		return result
	}()...) {
		if synonym.title == "" || synonym.title == manga.Title {
			continue
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO manga_synonyms (manga_id, type, title)
			VALUES (?, ?, ?)
		`, manga.MalId, synonym.typ, synonym.title); err != nil {
			return fmt.Errorf("insert manga synonym for %d: %w", manga.MalId, err)
		}
	}
	return nil
}

func (p *Persister) insertMangaAuthors(tx *sql.Tx, manga jikan.MangaBase) error {
	for _, author := range manga.Authors {
		if author.MalId == 0 || author.Name == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO people (mal_id, name, mal_url)
			VALUES (?, ?, ?)
		`, author.MalId, author.Name, nullIfEmpty(author.Url)); err != nil {
			return fmt.Errorf("insert manga author %d: %w", author.MalId, err)
		}
		role := author.Type
		if role == "" {
			role = "Author"
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO manga_authors (manga_id, person_id, role)
			VALUES (?, ?, ?)
		`, manga.MalId, author.MalId, role); err != nil {
			return fmt.Errorf("link manga author %d to %d: %w", author.MalId, manga.MalId, err)
		}
	}
	return nil
}

func (p *Persister) insertMangaTags(tx *sql.Tx, manga jikan.MangaBase) error {
	groups := []struct {
		items   []jikan.MalItem
		tagType string
	}{
		{manga.Genres, "genre"},
		{manga.ExplicitGenres, "explicit_genre"},
		{manga.Themes, "theme"},
		{manga.Demographics, "demographic"},
	}

	for _, group := range groups {
		for _, item := range group.items {
			if item.MalId == 0 || item.Name == "" {
				continue
			}
			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO tags (id, name, type, url)
				VALUES (?, ?, ?, ?)
			`, item.MalId, item.Name, group.tagType, nullIfEmpty(item.Url)); err != nil {
				return fmt.Errorf("insert manga tag %d: %w", item.MalId, err)
			}
			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO manga_tags (manga_id, tag_id)
				VALUES (?, ?)
			`, manga.MalId, item.MalId); err != nil {
				return fmt.Errorf("link manga tag %d to %d: %w", item.MalId, manga.MalId, err)
			}
		}
	}
	return nil
}

func (p *Persister) insertMangaRelations(tx *sql.Tx, mangaID int, relations jikan.MangaRelations) error {
	if _, err := tx.Exec(`DELETE FROM manga_relations WHERE manga_id = ?`, mangaID); err != nil {
		return fmt.Errorf("clear manga relations for %d: %w", mangaID, err)
	}
	for _, relation := range relations.Data {
		for _, entry := range relation.Entry {
			if entry.Type != "manga" || entry.MalId == 0 || relation.Relation == "" {
				continue
			}
			if _, err := tx.Exec(`
				INSERT OR REPLACE INTO manga_relations (manga_id, related_manga_id, relation_type)
				VALUES (?, ?, ?)
			`, mangaID, entry.MalId, relation.Relation); err != nil {
				return fmt.Errorf("insert manga relation %d -> %d: %w", mangaID, entry.MalId, err)
			}
		}
	}
	return nil
}

func (p *Persister) InsertScrapedAnime(ctx context.Context, item scrape.ScrapedAnime) error {
	if item.Error != nil {
		return fmt.Errorf("skipping anime with scrape error: %w", item.Error)
	}

	anime := item.Anime

	if anime.MalId == 0 {
		return fmt.Errorf("skipping anime without mal_id")
	}
	if anime.Title == "" {
		return fmt.Errorf("skipping anime %d without title", anime.MalId)
	}
	if IsBlacklisted(anime, p.opts) {
		return nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for anime %d: %w", anime.MalId, err)
	}
	defer tx.Rollback()

	if err := p.insertAnimeRow(tx, anime); err != nil {
		return err
	}
	if err := p.insertDescription(tx, anime); err != nil {
		return err
	}
	if err := p.insertSynonyms(tx, anime); err != nil {
		return err
	}
	if err := p.insertCompanies(tx, anime); err != nil {
		return err
	}
	if err := p.insertTags(tx, anime); err != nil {
		return err
	}
	if item.Relations != nil {
		if err := p.insertRelations(tx, anime.MalId, *item.Relations); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit anime %d: %w", anime.MalId, err)
	}

	return nil
}

func (p *Persister) insertAnimeRow(tx *sql.Tx, anime jikan.AnimeBase) error {

	typeID := p.lookups.AnimeTypeID(anime.Type)
	statusID := p.lookups.AnimeStatusID(anime.Status)
	quality := CalculateQualityScore(anime.Score, p.opts)

	_, err := tx.Exec(`
		INSERT OR REPLACE INTO anime
			(id, url, title, type_id, source, episodes, status_id, airing,
			 duration, quality_score, start_date, end_date, season, year,
			 broadcast_day, broadcast_time, broadcast_timezone,
			 image_url, small_image_url, large_image_url, trailer_embed_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		anime.MalId, nullIfEmpty(anime.Url), anime.Title, typeID, nullIfEmpty(anime.Source),
		nullIfZero(anime.Episodes), statusID, anime.Airing,
		nullIfEmpty(anime.Duration), quality,
		nullDate(anime.Aired.From.Time), nullDate(anime.Aired.To.Time),
		nullIfEmpty(anime.Season), nullIfZero(anime.Year),
		nullIfEmpty(anime.Broadcast.Day), nullIfEmpty(anime.Broadcast.Time),
		nullIfEmpty(anime.Broadcast.Timezone),
		// TODO, properly use config
		nullIfEmpty(anime.Images.Webp.ImageUrl), nullIfEmpty(anime.Images.Webp.SmallImageUrl),
		nullIfEmpty(anime.Images.Webp.LargeImageUrl), nullIfEmpty(anime.Trailer.EmbedUrl),
	)
	if err != nil {
		return fmt.Errorf("insert anime %d: %w", anime.MalId, err)
	}
	return nil
}

func (p *Persister) insertDescription(tx *sql.Tx, anime jikan.AnimeBase) error {
	var b strings.Builder
	if anime.Synopsis != "" {
		b.WriteString(anime.Synopsis)
	}
	if anime.Background != "" {
		b.WriteString("\n\n")
		b.WriteString(anime.Background)
	}
	description := b.String()
	if description == "" {
		return nil
	}

	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO anime_descriptions (anime_id, description)
		VALUES (?, ?)
	`, anime.MalId, description); err != nil {
		return fmt.Errorf("insert description for anime %d: %w", anime.MalId, err)
	}
	return nil
}

func (p *Persister) insertSynonyms(tx *sql.Tx, anime jikan.AnimeBase) error {
	if _, err := tx.Exec(`DELETE FROM synonyms WHERE anime_id = ?`, anime.MalId); err != nil {
		return fmt.Errorf("clear synonyms for anime %d: %w", anime.MalId, err)
	}

	type synonym struct {
		typ   string
		title string
	}

	var synonyms []synonym
	if anime.TitleEnglish != "" && anime.TitleEnglish != anime.Title {
		synonyms = append(synonyms, synonym{"English", anime.TitleEnglish})
	}
	if anime.TitleJapanese != "" && anime.TitleJapanese != anime.Title {
		synonyms = append(synonyms, synonym{"Japanese", anime.TitleJapanese})
	}
	for _, s := range anime.TitleSynonyms {
		if s != "" && s != anime.Title {
			synonyms = append(synonyms, synonym{"Synonym", s})
		}
	}

	for _, s := range synonyms {
		if _, err := tx.Exec(`
			INSERT INTO synonyms (anime_id, type, title) VALUES (?, ?, ?)
		`, anime.MalId, s.typ, s.title); err != nil {
			return fmt.Errorf("insert synonym for anime %d: %w", anime.MalId, err)
		}
	}

	return nil
}

func (p *Persister) insertCompanies(tx *sql.Tx, anime jikan.AnimeBase) error {
	groups := []struct {
		items []jikan.MalItem
		role  string
	}{
		{anime.Producers, "Producer"},
		{anime.Licensors, "Licensor"},
		{anime.Studios, "Studio"},
	}

	for _, g := range groups {
		companyTypeID, ok := p.lookups.CompanyTypeID(g.role)
		if !ok {
			return fmt.Errorf("unknown company role %q (add it to company_type)", g.role)
		}

		for _, item := range g.items {
			if item.MalId == 0 || item.Name == "" {
				continue
			}

			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO companies (mal_id, name, mal_url, type_id)
				VALUES (?, ?, ?, ?)
			`, item.MalId, item.Name, nullIfEmpty(item.Url), companyTypeID); err != nil {
				return fmt.Errorf("insert company %d: %w", item.MalId, err)
			}

			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO anime_companies (anime_id, company_id, role)
				VALUES (?, ?, ?)
			`, anime.MalId, item.MalId, g.role); err != nil {
				return fmt.Errorf("link company %d to anime %d: %w", item.MalId, anime.MalId, err)
			}
		}
	}

	return nil
}

func (p *Persister) insertTags(tx *sql.Tx, anime jikan.AnimeBase) error {
	groups := []struct {
		items   []jikan.MalItem
		tagType string
	}{
		{anime.Genres, "genre"},
		{anime.ExplicitGenres, "explicit_genre"},
		{anime.Themes, "theme"},
		{anime.Demographics, "demographic"},
	}

	for _, g := range groups {
		for _, item := range g.items {
			if item.MalId == 0 || item.Name == "" {
				continue
			}

			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO tags (id, name, type, url)
				VALUES (?, ?, ?, ?)
			`, item.MalId, item.Name, g.tagType, nullIfEmpty(item.Url)); err != nil {
				return fmt.Errorf("insert tag %d: %w", item.MalId, err)
			}

			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO anime_tags (anime_id, tag_id)
				VALUES (?, ?)
			`, anime.MalId, item.MalId); err != nil {
				return fmt.Errorf("link tag %d to anime %d: %w", item.MalId, anime.MalId, err)
			}
		}
	}

	return nil
}

func (p *Persister) insertRelations(tx *sql.Tx, animeID int, relations jikan.AnimeRelations) error {
	if _, err := tx.Exec(`DELETE FROM anime_relations WHERE anime_id = ?`, animeID); err != nil {
		return fmt.Errorf("clear relations for anime %d: %w", animeID, err)
	}

	for _, rel := range relations.Data {
		relationTypeID, ok := p.lookups.RelationTypeID(rel.Relation)
		if !ok {
			fmt.Printf("[Persister] Unknown relation type %q on anime %d, skipping\n", rel.Relation, animeID)
			continue
		}

		for _, entry := range rel.Entry {
			// Manga-side relations (e.g. an "Adaptation" pointing at a
			// manga) aren't representable here, anime_relations only
			// links anime to anime FOR NOW
			if entry.Type != "anime" {
				continue
			}

			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO anime_relations (anime_id, relation_type_id, related_anime_id)
				VALUES (?, ?, ?)
			`, animeID, relationTypeID, entry.MalId); err != nil {
				return fmt.Errorf("insert relation %d -> %d: %w", animeID, entry.MalId, err)
			}
		}
	}

	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format("2006-01-02")
}
