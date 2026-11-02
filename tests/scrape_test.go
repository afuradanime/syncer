package tests

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"syncer/src/config"
	"syncer/src/scrape"
	"syncer/src/sync"
	"syncer/tests/mock"
	"testing"

	jikan "github.com/afuradanime/tenrai-go"
)

func searchQuery(page int) url.Values {
	return url.Values{"page": {strconv.Itoa(page)}, "limit": {"50"}, "order_by": {"mal_id"}, "sort": {"asc"}}
}

func searchPageResponse(firstID, count, page, lastPage int) mock.JikanResponse {
	items := make([]map[string]any, count)
	for index := range items {
		items[index] = map[string]any{"mal_id": firstID + index}
	}
	return mock.JikanResponse{Body: map[string]any{
		"data":       items,
		"pagination": map[string]any{"has_next_page": page < lastPage, "last_visible_page": lastPage},
	}}
}

func TestScrapeWithoutRelations(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial=%t", kind, partial), func(t *testing.T) {
				api := mock.NewJikan(t)
				path := "/v1/" + kind
				firstPage := searchPageResponse(1, 50, 1, 2)
				api.ExpectGET(path, searchQuery(1), firstPage)
				if partial {
					api.ExpectGET(path+"/1", nil, mock.JikanResponse{Body: map[string]any{"data": map[string]any{"mal_id": 1}}})
					api.ExpectGET(path, searchQuery(1), firstPage, firstPage)
				}
				api.ExpectGET(path, searchQuery(2), searchPageResponse(51, 1, 2, 2))
				cfg := config.Config{MaxAttempts: 1}
				db, err := sync.OpenDatabase(":memory:")
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if _, err := db.Exec(`
					CREATE TABLE anime (id INTEGER, airing INTEGER);
					CREATE TABLE mangas (id INTEGER, status TEXT);
					INSERT INTO anime VALUES (1, 1);
					INSERT INTO mangas VALUES (1, 'Publishing');
				`); err != nil {
					t.Fatal(err)
				}
				count := 0
				if kind == "anime" {
					results := make(chan scrape.ScrapedAnime, 100)
					if partial {
						err = scrape.ScrapePartialAnime(context.Background(), results, cfg, db)
					} else {
						err = scrape.ScrapeAllAnime(context.Background(), results, cfg)
					}
					if err != nil {
						t.Fatal(err)
					}
					for item := range results {
						if item.Relations != nil || item.Error != nil || item.Anime.MalId == 0 {
							t.Fatalf("unexpected anime result: %+v", item)
						}
						count++
					}
				} else {
					results := make(chan scrape.ScrapedManga, 100)
					if partial {
						err = scrape.ScrapePartialManga(context.Background(), results, cfg, db)
					} else {
						err = scrape.ScrapeAllManga(context.Background(), results, cfg)
					}
					if err != nil {
						t.Fatal(err)
					}
					for item := range results {
						if item.Relations != nil || item.Error != nil || item.Manga.MalId == 0 {
							t.Fatalf("unexpected manga result: %+v", item)
						}
						count++
					}
				}
				wantCount, wantPages, wantRefreshes := 51, 2, 0
				if partial {
					wantCount, wantPages, wantRefreshes = 52, 4, 1
				}
				pages, refreshes := 0, 0
				for _, request := range api.Requests() {
					if request.URL.Path == path {
						pages++
					} else {
						refreshes++
					}
				}
				if count != wantCount || pages != wantPages || refreshes != wantRefreshes {
					t.Fatalf("items/pages/refreshes = %d/%d/%d, want %d/%d/%d", count, pages, refreshes, wantCount, wantPages, wantRefreshes)
				}
			})
		}
	}
}

func scrapeAllForTest(t *testing.T, ctx context.Context, kind string, cfg config.Config) ([]int, error) {
	t.Helper()
	var ids []int
	if kind == "anime" {
		results := make(chan scrape.ScrapedAnime, 100)
		err := scrape.ScrapeAllAnime(ctx, results, cfg)
		for item := range results {
			if item.Error != nil || item.Relations != nil {
				t.Fatalf("unexpected result: %+v", item)
			}
			ids = append(ids, item.Anime.MalId)
		}
		return ids, err
	}
	results := make(chan scrape.ScrapedManga, 100)
	err := scrape.ScrapeAllManga(ctx, results, cfg)
	for item := range results {
		if item.Error != nil || item.Relations != nil {
			t.Fatalf("unexpected result: %+v", item)
		}
		ids = append(ids, item.Manga.MalId)
	}
	return ids, err
}

func TestScrapeRetryableErrors(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		for _, status := range []int{429, 500, 502, 504} {
			t.Run(fmt.Sprintf("%s/%d", kind, status), func(t *testing.T) {
				api := mock.NewJikan(t)
				api.ExpectGET("/v1/"+kind, searchQuery(1),
					mock.JikanResponse{StatusCode: status, Body: map[string]any{"status": status, "message": http.StatusText(status)}},
					searchPageResponse(7, 1, 1, 1),
				)
				ids, err := scrapeAllForTest(t, context.Background(), kind, config.Config{MaxAttempts: 2})
				if err != nil {
					t.Fatal(err)
				}
				if len(ids) != 1 || ids[0] != 7 || len(api.Requests()) != 2 {
					t.Fatalf("ids = %v, requests = %d; want [7] and 2 requests", ids, len(api.Requests()))
				}
			})
		}
	}
}

func TestScrapeFailures(t *testing.T) {
	transportErr := errors.New("connection reset")
	cases := []struct {
		name     string
		response mock.JikanResponse
		requests int
	}{
		{"retry exhaustion", mock.JikanResponse{StatusCode: 429, Body: map[string]any{"status": 429}}, 4},
		{"nonretryable status", mock.JikanResponse{StatusCode: 400, Body: map[string]any{"status": 400}}, 2},
		{"invalid JSON", mock.JikanResponse{Body: "{broken"}, 2},
		{"transport failure", mock.JikanResponse{Err: transportErr}, 2},
	}
	for _, kind := range []string{"anime", "manga"} {
		for _, testCase := range cases {
			t.Run(kind+"/"+testCase.name, func(t *testing.T) {
				api := mock.NewJikan(t)
				for attempt := 0; attempt < testCase.requests; attempt++ {
					api.ExpectGET("/v1/"+kind, searchQuery(1), testCase.response)
				}
				ids, err := scrapeAllForTest(t, context.Background(), kind, config.Config{MaxAttempts: 2})
				if err == nil || len(ids) != 0 {
					t.Fatalf("ids = %v, error = %v; want no items and an error", ids, err)
				}
				if len(api.Requests()) != testCase.requests {
					t.Fatalf("requests = %d, want %d", len(api.Requests()), testCase.requests)
				}
				if testCase.response.Err != nil && !errors.Is(err, transportErr) {
					t.Fatalf("transport error not preserved: %v", err)
				}
			})
		}
	}
}

func TestScrapeEmptyPage(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		t.Run(kind, func(t *testing.T) {
			api := mock.NewJikan(t)
			api.ExpectGET("/v1/"+kind, searchQuery(1), searchPageResponse(1, 0, 1, 2))
			ids, err := scrapeAllForTest(t, context.Background(), kind, config.Config{MaxAttempts: 1})
			if err != nil || len(ids) != 0 || len(api.Requests()) != 1 {
				t.Fatalf("ids = %v, error = %v, requests = %d", ids, err, len(api.Requests()))
			}
		})
	}
}

func TestScrapeCancelledBeforeRequest(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		t.Run(kind, func(t *testing.T) {
			api := mock.NewJikan(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			ids, err := scrapeAllForTest(t, ctx, kind, config.Config{MaxAttempts: 1})
			if !errors.Is(err, context.Canceled) || len(ids) != 0 || len(api.Requests()) != 0 {
				t.Fatalf("ids = %v, error = %v, requests = %d", ids, err, len(api.Requests()))
			}
		})
	}
}

func TestJikanMockRestoresClient(t *testing.T) {
	previous := jikan.Client
	t.Run("mock client", func(t *testing.T) {
		api := mock.NewJikan(t)
		api.ExpectGET("/v1/anime", searchQuery(1), searchPageResponse(1, 0, 1, 1))
		if _, err := jikan.GetAnimeSearch(searchQuery(1)); err != nil {
			t.Fatal(err)
		}
		requests := api.Requests()
		if len(requests) != 1 {
			t.Fatalf("recorded requests = %d, want 1", len(requests))
		}
		requests[0].URL.Path = "/changed"
		if api.Requests()[0].URL.Path != "/v1/anime" {
			t.Fatal("request snapshots share mutable state")
		}
	})
	if jikan.Client != previous {
		t.Fatal("Jikan client was not restored")
	}
}
