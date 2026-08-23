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

func fetchAnimeByIDWithRetry(ctx context.Context, malID int, baseBackoff time.Duration, maxRetries int) (*jikan.AnimeById, error) {
	backoff := baseBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		anime, err := jikan.GetAnimeById(malID)
		if err == nil {
			return anime, nil
		}

		errStr := err.Error()
		isRateLimitOrTimeout := strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "504") ||
			strings.Contains(errStr, "500") ||
			strings.Contains(errStr, "502")

		if isRateLimitOrTimeout && attempt < maxRetries {
			fmt.Printf("[AnimeById Retry] MalID %d failed (%v). Retrying (%d/%d) in %v...\n",
				malID, err, attempt, maxRetries, backoff)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("exceeded max retries fetching anime by id %d", malID)
}

func fetchMangaRelationsWithRetry(ctx context.Context, malID int, baseBackoff time.Duration, maxRetries int) (*jikan.MangaRelations, error) {
	backoff := baseBackoff
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		relations, err := jikan.GetMangaRelations(malID)
		if err == nil {
			return relations, nil
		}
		if isRetryable(err) && attempt < maxRetries {
			fmt.Printf("[Manga Relations Retry] MalID %d failed (%v). Retrying (%d/%d) in %v...\n", malID, err, attempt, maxRetries, backoff)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("exceeded max retries fetching manga relations for MalID %d", malID)
}

func fetchMangaByIDWithRetry(ctx context.Context, malID int, baseBackoff time.Duration, maxRetries int) (*jikan.MangaById, error) {
	backoff := baseBackoff
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		manga, err := jikan.GetMangaById(malID)
		if err == nil {
			return manga, nil
		}
		if isRetryable(err) && attempt < maxRetries {
			fmt.Printf("[MangaById Retry] MalID %d failed (%v). Retrying (%d/%d) in %v...\n", malID, err, attempt, maxRetries, backoff)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("exceeded max retries fetching manga by id %d", malID)
}

func fetchMangaSearchWithRetry(ctx context.Context, query url.Values, baseBackoff time.Duration, maxRetries int) (*jikan.MangaSearch, error) {
	backoff := baseBackoff
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		response, err := jikan.GetMangaSearch(query)
		if err == nil {
			return response, nil
		}
		if isRetryable(err) && attempt < maxRetries {
			fmt.Printf("[Manga Search Retry] page %s failed (%v). Retrying (%d/%d) in %v...\n", query.Get("page"), err, attempt, maxRetries, backoff)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("exceeded max retries fetching manga page %s", query.Get("page"))
}

func isRetryable(err error) bool {
	errString := err.Error()
	return strings.Contains(errString, "429") || strings.Contains(errString, "500") ||
		strings.Contains(errString, "502") || strings.Contains(errString, "504")
}

func scrapeMangaPagesFrom(ctx context.Context, results chan<- ScrapedManga, cfg config.Config, startPage int, updateRelationsToo bool) (int, error) {
	page, totalProcessed, consecutiveErrors := startPage, 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return totalProcessed, err
		}

		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("limit", strconv.Itoa(LIMIT))
		query.Set("order_by", "mal_id")
		query.Set("sort", "asc")
		fmt.Printf("[Manga Scraper] Fetching search page %d...\n", page)

		response, err := fetchMangaSearchWithRetry(ctx, query, cfg.BaseBackoff, cfg.MaxAttempts)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= cfg.MaxAttempts {
				return totalProcessed, fmt.Errorf("too many consecutive manga search errors: %w", err)
			}
			time.Sleep(cfg.SleepAmount)
			continue
		}
		consecutiveErrors = 0
		if len(response.Data) == 0 {
			break
		}

		for _, manga := range response.Data {
			time.Sleep(cfg.SleepAmount)
			relations, relationErr := fetchMangaRelationsWithRetry(ctx, manga.MalId, cfg.BaseBackoff, cfg.MaxAttempts)
			if relationErr == nil && relations != nil && updateRelationsToo {
				for _, relation := range relations.Data {
					for _, entry := range relation.Entry {
						if entry.Type != "manga" {
							continue
						}
						time.Sleep(cfg.SleepAmount)
						related, relatedErr := fetchMangaByIDWithRetry(ctx, entry.MalId, cfg.BaseBackoff, cfg.MaxAttempts)
						if relatedErr != nil {
							fmt.Printf("[Persistency Worker error] Failed to fetch related manga for MalID %d: %v\n", entry.MalId, relatedErr)
							continue
						}
						select {
						case <-ctx.Done():
							return totalProcessed, ctx.Err()
						case results <- ScrapedManga{Manga: related.Data}:
						}
					}
				}
			}

			select {
			case <-ctx.Done():
				return totalProcessed, ctx.Err()
			case results <- ScrapedManga{Manga: manga, Relations: relations, Error: relationErr}:
				totalProcessed++
			}
		}

		if !response.Pagination.HasNextPage {
			break
		}
		page++
		time.Sleep(cfg.SleepAmount)
	}
	return totalProcessed, nil
}

func scrapeAnimePagesFrom(ctx context.Context, results chan<- ScrapedAnime, cfg config.Config, startPage int, updateRelationsToo bool) (int, error) {
	page := startPage
	consecutiveErrors := 0
	totalProcessed := 0

	for {
		if err := ctx.Err(); err != nil {
			return totalProcessed, err
		}

		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("limit", strconv.Itoa(LIMIT))
		query.Set("order_by", "mal_id")
		query.Set("sort", "asc")

		fmt.Printf("[Scraper] Fetching search page %d...\n", page)

		listResp, err := fetchAnimeSearchWithRetry(ctx, query, cfg.BaseBackoff, cfg.MaxAttempts)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= cfg.MaxAttempts {
				return totalProcessed, fmt.Errorf("too many consecutive search errors: %w", err)
			}

			fmt.Printf("[Scraper Error] Page %d failed after retries: %v. Moving on (%d/%d consecutive page failures)...\n",
				page, err, consecutiveErrors, cfg.MaxAttempts)

			time.Sleep(cfg.SleepAmount)
			continue
		}

		consecutiveErrors = 0

		if len(listResp.Data) == 0 {
			fmt.Printf("[Scraper] No data found on page %d. Stopping.\n", page)
			break
		}

		for _, anime := range listResp.Data {
			time.Sleep(cfg.SleepAmount)

			relations, relErr := fetchRelationsWithRetry(ctx, anime.MalId, cfg.BaseBackoff, cfg.MaxAttempts)

			// Send relations to be persisted as well
			if relErr == nil && relations != nil && updateRelationsToo {

				for _, rel := range relations.Data {
					for _, relation := range rel.Entry {

						time.Sleep(cfg.SleepAmount)

						relationMalID := relation.MalId

						// Fetch the related anime details
						relatedAnimeResp, relatedAnimeErr := fetchAnimeByIDWithRetry(ctx, relationMalID, cfg.BaseBackoff, cfg.MaxAttempts)
						if relatedAnimeErr != nil {
							fmt.Printf("[Persistency Worker error] Failed to fetch related anime for MalID %d: %v\n", relationMalID, relatedAnimeErr)
							continue
						}

						// Send the related anime to the results channel
						select {
						case <-ctx.Done():
							return totalProcessed, ctx.Err()
						case results <- ScrapedAnime{Anime: relatedAnimeResp.Data}:
						}
					}
				}
			}

			select {
			case <-ctx.Done():
				return totalProcessed, ctx.Err()
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

	return totalProcessed, nil
}

func fetchAnimeSearchWithRetry(ctx context.Context, query url.Values, baseBackoff time.Duration, maxRetries int) (*jikan.AnimeSearch, error) {
	backoff := baseBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := jikan.GetAnimeSearch(query)
		if err == nil {
			return resp, nil
		}

		errStr := err.Error()
		isRateLimitOrTimeout := strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "504") ||
			strings.Contains(errStr, "500") ||
			strings.Contains(errStr, "502")

		if isRateLimitOrTimeout && attempt < maxRetries {
			fmt.Printf("[FindPage Retry] page %s failed (%v). Retrying (%d/%d) in %v...\n",
				query.Get("page"), err, attempt, maxRetries, backoff)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("exceeded max retries fetching page %s", query.Get("page"))
}

func ScrapeAllAnime(ctx context.Context, results chan<- ScrapedAnime, cfg config.Config) error {
	fmt.Println("Starting anime scraper. Press Ctrl+C to stop.")
	defer close(results)

	totalProcessed, err := scrapeAnimePagesFrom(ctx, results, cfg, 1, false)
	if err != nil {
		return err
	}

	fmt.Printf("[Scraper] Finished successfully. Total items sent to queue: %d\n", totalProcessed)
	return nil
}

func ScrapeAllManga(ctx context.Context, results chan<- ScrapedManga, cfg config.Config) error {
	fmt.Println("Starting manga scraper. Press Ctrl+C to stop.")
	defer close(results)
	_, err := scrapeMangaPagesFrom(ctx, results, cfg, 1, false)
	if err != nil {
		return err
	}
	fmt.Println("[Manga Scraper] Finished successfully.")
	return nil
}

func findPageOfAnime(malID int, cfg config.Config) (int, error) {

	// Get number of pages from first page
	query := url.Values{}
	query.Set("page", "1")
	query.Set("limit", strconv.Itoa(LIMIT))
	query.Set("order_by", "mal_id")
	query.Set("sort", "asc")

	listResp, err := fetchAnimeSearchWithRetry(context.Background(), query, cfg.BaseBackoff, cfg.MaxAttempts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch first page: %w", err)
	}

	totalItems := listResp.Pagination.LastVisiblePage

	// Now we can do a binary search to find the page containing the malID
	left, right := 1, totalItems
	for left <= right {
		mid := (left + right) / 2

		query.Set("page", strconv.Itoa(mid))
		listResp, err := fetchAnimeSearchWithRetry(context.Background(), query, cfg.BaseBackoff, cfg.MaxAttempts)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch page %d: %w", mid, err)
		}

		if len(listResp.Data) == 0 {
			return 0, fmt.Errorf("no data found on page %d", mid)
		}

		firstID := listResp.Data[0].MalId
		lastID := listResp.Data[len(listResp.Data)-1].MalId

		if malID >= firstID && malID <= lastID {
			return mid, nil
		} else if malID < firstID {
			right = mid - 1
		} else {
			left = mid + 1
		}
	}

	return 0, fmt.Errorf("id %d not found in any page", malID)
}

func findPageOfManga(malID int, cfg config.Config) (int, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("limit", strconv.Itoa(LIMIT))
	query.Set("order_by", "mal_id")
	query.Set("sort", "asc")
	response, err := fetchMangaSearchWithRetry(context.Background(), query, cfg.BaseBackoff, cfg.MaxAttempts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch first manga page: %w", err)
	}
	left, right := 1, response.Pagination.LastVisiblePage
	for left <= right {
		middle := (left + right) / 2
		query.Set("page", strconv.Itoa(middle))
		response, err = fetchMangaSearchWithRetry(context.Background(), query, cfg.BaseBackoff, cfg.MaxAttempts)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch manga page %d: %w", middle, err)
		}
		if len(response.Data) == 0 {
			return 0, fmt.Errorf("no manga data found on page %d", middle)
		}
		firstID, lastID := response.Data[0].MalId, response.Data[len(response.Data)-1].MalId
		if malID >= firstID && malID <= lastID {
			return middle, nil
		} else if malID < firstID {
			right = middle - 1
		} else {
			left = middle + 1
		}
	}
	return 0, fmt.Errorf("manga id %d not found in any page", malID)
}

func ScrapePartialAnime(ctx context.Context, results chan<- ScrapedAnime, cfg config.Config, db *sql.DB) error {
	defer close(results)

	var lastMalID int
	if err := db.QueryRow("SELECT id FROM anime ORDER BY id DESC LIMIT 1").Scan(&lastMalID); err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("query last mal_id: %w", err)
		}
		lastMalID = 0
	}

	fmt.Printf("[Scraper] Last mal_id in database: %d\n", lastMalID)

	var airingAnime []int
	rows, err := db.Query("SELECT id FROM anime WHERE airing = 1")
	if err != nil {
		return fmt.Errorf("query airing anime: %w", err)
	}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan airing anime id: %w", err)
		}
		airingAnime = append(airingAnime, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("rows airing anime: %w", err)
	}
	rows.Close()

	fmt.Println("Starting partial anime sync. Press Ctrl+C to stop.")
	fmt.Printf("[Scraper] Updating %d airing anime\n", len(airingAnime))

	for _, malID := range airingAnime {
		if err := ctx.Err(); err != nil {
			return err
		}

		time.Sleep(cfg.SleepAmount)

		var item ScrapedAnime
		animeResp, animeErr := fetchAnimeByIDWithRetry(ctx, malID, cfg.BaseBackoff, cfg.MaxAttempts)
		if animeErr != nil {
			// Keep the mal_id for identification later
			item = ScrapedAnime{Anime: jikan.AnimeBase{MalId: malID}, Error: animeErr}
		} else {
			relations, relErr := fetchRelationsWithRetry(ctx, malID, cfg.BaseBackoff, cfg.MaxAttempts)
			item = ScrapedAnime{Anime: animeResp.Data, Relations: relations, Error: relErr}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case results <- item:
		}
	}

	// Safeguard for empty database
	startPage := 1
	if lastMalID != 0 {
		startPage, err = findPageOfAnime(lastMalID, cfg)
		if err != nil {
			return fmt.Errorf("find page of anime: %w", err)
		}
	}

	fmt.Printf("[Scraper] Scanning for new anime starting from page %d (last known mal_id %d)\n", startPage, lastMalID)

	newCount, err := scrapeAnimePagesFrom(ctx, results, cfg, startPage, true)
	if err != nil {
		return err
	}

	fmt.Printf("[Scraper] Partial sync finished. %d airing refreshed, %d new/updated from page scan.\n",
		len(airingAnime), newCount)

	return nil
}

func ScrapePartialManga(ctx context.Context, results chan<- ScrapedManga, cfg config.Config, db *sql.DB) error {
	defer close(results)
	var lastMalID int
	if err := db.QueryRow("SELECT id FROM mangas ORDER BY id DESC LIMIT 1").Scan(&lastMalID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("query last manga mal_id: %w", err)
	}

	var publishing []int
	rows, err := db.Query("SELECT id FROM mangas WHERE status = 'Publishing'")
	if err != nil {
		return fmt.Errorf("query publishing manga: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan publishing manga id: %w", err)
		}
		publishing = append(publishing, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows publishing manga: %w", err)
	}

	for _, malID := range publishing {
		if err := ctx.Err(); err != nil {
			return err
		}
		time.Sleep(cfg.SleepAmount)
		mangaResponse, mangaErr := fetchMangaByIDWithRetry(ctx, malID, cfg.BaseBackoff, cfg.MaxAttempts)
		if mangaErr != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case results <- ScrapedManga{Manga: jikan.MangaBase{MalId: malID}, Error: mangaErr}:
			}
			continue
		}
		relations, relationErr := fetchMangaRelationsWithRetry(ctx, malID, cfg.BaseBackoff, cfg.MaxAttempts)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case results <- ScrapedManga{Manga: mangaResponse.Data, Relations: relations, Error: relationErr}:
		}
	}

	startPage := 1
	if lastMalID != 0 {
		startPage, err = findPageOfManga(lastMalID, cfg)
		if err != nil {
			return fmt.Errorf("find page of manga: %w", err)
		}
	}
	_, err = scrapeMangaPagesFrom(ctx, results, cfg, startPage, true)
	return err
}
