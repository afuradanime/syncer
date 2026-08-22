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
