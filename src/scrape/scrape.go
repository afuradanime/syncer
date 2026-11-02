package scrape

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"syncer/src/config"
	"time"

	jikan "github.com/afuradanime/tenrai-go"
)

const LIMIT = 50
const REPORT_FREQUENCY = 100

type ScrapedAnime struct {
	Anime     jikan.AnimeBase
	Relations *jikan.AnimeRelations
	Error     error
}

type ScrapedManga struct {
	Manga     jikan.MangaBase
	Relations *jikan.MangaRelations
	Error     error
}

type searchPage[T any] struct {
	items    []T
	hasNext  bool
	lastPage int
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func fetchWithRetry[T any](ctx context.Context, cfg config.Config, fetch func() (T, error)) (T, error) {
	var zero T
	backoff := cfg.BaseBackoff
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		result, err := fetch()
		if err == nil {
			return result, nil
		}
		if !isRetryable(err) || attempt == cfg.MaxAttempts {
			return zero, err
		}
		fmt.Printf("[Scraper Retry] Request failed (%v). Retrying (%d/%d) in %v...\n", err, attempt, cfg.MaxAttempts, backoff)
		if err := wait(ctx, backoff); err != nil {
			return zero, err
		}
		backoff *= 2
	}
	return zero, fmt.Errorf("exceeded max retries fetching data")
}

func isRetryable(err error) bool {
	errString := err.Error()
	return strings.Contains(errString, "429") || strings.Contains(errString, "500") ||
		strings.Contains(errString, "502") || strings.Contains(errString, "504")
}

func pageQuery(page int) url.Values {
	return url.Values{
		"page":     {strconv.Itoa(page)},
		"limit":    {strconv.Itoa(LIMIT)},
		"order_by": {"mal_id"},
		"sort":     {"asc"},
	}
}

func scrapePagesFrom[T, Result any](ctx context.Context, results chan<- Result, cfg config.Config, startPage int, adapter fetchAdapter[T, Result]) (int, error) {
	page, totalProcessed, consecutiveErrors := startPage, 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return totalProcessed, err
		}
		fmt.Printf("[%s Scraper] Fetching search page %d...\n", adapter.name, page)
		response, err := fetchWithRetry(ctx, cfg, func() (searchPage[T], error) {
			return adapter.search(pageQuery(page))
		})
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= cfg.MaxAttempts {
				return totalProcessed, fmt.Errorf("too many consecutive %s search errors: %w", adapter.name, err)
			}
			if err := wait(ctx, cfg.SleepAmount); err != nil {
				return totalProcessed, err
			}
			continue
		}
		consecutiveErrors = 0
		for _, item := range response.items {
			select {
			case <-ctx.Done():
				return totalProcessed, ctx.Err()
			case results <- adapter.result(item, adapter.id(item), nil):
				totalProcessed++
				if totalProcessed%REPORT_FREQUENCY == 0 {
					fmt.Printf("[Report] Queue pushed: %d total %s queued so far\n", totalProcessed, adapter.name)
				}
			}
		}
		if len(response.items) == 0 || !response.hasNext {
			return totalProcessed, nil
		}
		page++
		if err := wait(ctx, cfg.SleepAmount); err != nil {
			return totalProcessed, err
		}
	}
}

func findPage[T, Result any](ctx context.Context, malID int, cfg config.Config, adapter fetchAdapter[T, Result]) (int, error) {
	response, err := fetchWithRetry(ctx, cfg, func() (searchPage[T], error) {
		return adapter.search(pageQuery(1))
	})
	if err != nil {
		return 0, fmt.Errorf("fetch first %s page: %w", adapter.name, err)
	}
	left, right := 1, response.lastPage
	for left <= right {
		middle := (left + right) / 2
		if err := wait(ctx, cfg.SleepAmount); err != nil {
			return 0, err
		}
		response, err = fetchWithRetry(ctx, cfg, func() (searchPage[T], error) {
			return adapter.search(pageQuery(middle))
		})
		if err != nil {
			return 0, fmt.Errorf("fetch %s page %d: %w", adapter.name, middle, err)
		}
		if len(response.items) == 0 {
			return 0, fmt.Errorf("no %s data found on page %d", adapter.name, middle)
		}
		firstID, lastID := adapter.id(response.items[0]), adapter.id(response.items[len(response.items)-1])
		if malID >= firstID && malID <= lastID {
			return middle, nil
		} else if malID < firstID {
			right = middle - 1
		} else {
			left = middle + 1
		}
	}
	return 0, fmt.Errorf("%s id %d not found in any page", adapter.name, malID)
}

func scrapePartial[T, Result any](ctx context.Context, results chan<- Result, cfg config.Config, db *sql.DB, adapter fetchAdapter[T, Result]) error {
	defer close(results)
	var lastMalID int
	if err := db.QueryRowContext(ctx, adapter.lastIDQuery).Scan(&lastMalID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("query last %s mal_id: %w", adapter.name, err)
	}
	rows, err := db.QueryContext(ctx, adapter.activeQuery)
	if err != nil {
		return fmt.Errorf("query active %s: %w", adapter.name, err)
	}
	var activeIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan active %s id: %w", adapter.name, err)
		}
		activeIDs = append(activeIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("rows active %s: %w", adapter.name, err)
	}
	for _, malID := range activeIDs {
		if err := wait(ctx, cfg.SleepAmount); err != nil {
			return err
		}
		item, fetchErr := fetchWithRetry(ctx, cfg, func() (T, error) { return adapter.byID(malID) })
		select {
		case <-ctx.Done():
			return ctx.Err()
		case results <- adapter.result(item, malID, fetchErr):
		}
	}
	startPage := 1
	if lastMalID != 0 {
		startPage, err = findPage(ctx, lastMalID, cfg, adapter)
		if err != nil {
			return err
		}
	}
	_, err = scrapePagesFrom(ctx, results, cfg, startPage, adapter)
	return err
}

func ScrapeAllAnime(ctx context.Context, results chan<- ScrapedAnime, cfg config.Config) error {
	defer close(results)
	_, err := scrapePagesFrom(ctx, results, cfg, 1, animeAdapter)
	return err
}

func ScrapeAllManga(ctx context.Context, results chan<- ScrapedManga, cfg config.Config) error {
	defer close(results)
	_, err := scrapePagesFrom(ctx, results, cfg, 1, mangaAdapter)
	return err
}

func ScrapePartialAnime(ctx context.Context, results chan<- ScrapedAnime, cfg config.Config, db *sql.DB) error {
	return scrapePartial(ctx, results, cfg, db, animeAdapter)
}

func ScrapePartialManga(ctx context.Context, results chan<- ScrapedManga, cfg config.Config, db *sql.DB) error {
	return scrapePartial(ctx, results, cfg, db, mangaAdapter)
}
