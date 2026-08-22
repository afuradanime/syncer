package scrape

import (
	"context"
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

func fetchRelationsWithRetry(ctx context.Context, malID int, baseBackoff time.Duration, maxRetries int) (*jikan.AnimeRelations, error) {

	backoff := baseBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		relations, err := jikan.GetAnimeRelations(malID)
		if err == nil {
			return relations, nil
		}

		errStr := err.Error()

		// Check for rate limit (429) or gateway timeout (504/500/502)
		isRateLimitOrTimeout := strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "504") ||
			strings.Contains(errStr, "500") ||
			strings.Contains(errStr, "502")

		if isRateLimitOrTimeout && attempt < maxRetries {
			fmt.Printf("[Relations Retry] MalID %d failed (%v). Retrying (%d/%d) in %v...\n",
				malID, err, attempt, maxRetries, backoff)

			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("exceeded max retries fetching relations for MalID %d", malID)
}

func ScrapeAllAnime(ctx context.Context, results chan<- ScrapedAnime, cfg config.Config) error {
	page := 1
	consecutiveErrors := 0
	totalProcessed := 0

	backoff := cfg.BaseBackoff

	fmt.Println("Starting anime scraper. Press Ctrl+C to stop.")

	defer close(results)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("limit", strconv.Itoa(LIMIT))

		fmt.Printf("[Scraper] Fetching search page %d...\n", page)

		listResp, err := jikan.GetAnimeSearch(query)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= cfg.MaxAttempts {
				return fmt.Errorf("too many consecutive search errors: %w", err)
			}

			fmt.Printf("[Scraper Error] Page %d failed: %v. Retrying in %v (%d/%d)...\n",
				page, err, backoff, consecutiveErrors, cfg.MaxAttempts)

			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
			continue
		}

		backoff = cfg.BaseBackoff
		consecutiveErrors = 0

		if len(listResp.Data) == 0 {
			fmt.Printf("[Scraper] No data found on page %d. Stopping.\n", page)
			break
		}

		// Process each anime entry
		for _, anime := range listResp.Data {
			time.Sleep(cfg.SleepAmount)

			relations, relErr := fetchRelationsWithRetry(ctx, anime.MalId, cfg.BaseBackoff, cfg.MaxAttempts)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case results <- ScrapedAnime{
				Anime:     anime,
				Relations: relations,
				Error:     relErr,
			}:
				totalProcessed++
				if totalProcessed%REPORT_FREQUENCY == 0 {
					fmt.Printf("[Report] Queue pushed: %d total anime queued so far\n", totalProcessed)
				}
			}
		}

		if !listResp.Pagination.HasNextPage {
			fmt.Println("[Scraper] Reached the last page. Scraping completed.")
			break
		}

		page++
		time.Sleep(cfg.SleepAmount)
	}

	fmt.Printf("[Scraper] Finished successfully. Total items sent to queue: %d\n", totalProcessed)
	return nil
}
