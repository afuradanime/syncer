package tests

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"syncer/src/config"
	"syncer/src/scrape"
	"syncer/src/sync"
	"syncer/tests/unit/mock"

	jikan "github.com/afuradanime/tenrai-go"
)

// OFT-REQ-FR-01: paginated scrape coverage for incremental collection and full walk behavior.
func TestScrapeAll_WalksEveryPage(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		t.Run(kind, func(t *testing.T) {
			api := mock.NewJikan(t)
			path := "/v1/" + kind
			api.ExpectGET(path, searchQuery(1), searchPageResponse(1, 50, 1, 2))
			api.ExpectGET(path, searchQuery(2), searchPageResponse(51, 1, 2, 2))

			ids, err := scrapeAll(t, kind, context.Background(), config.Config{MaxAttempts: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(ids) != 51 {
				t.Fatalf("items = %d, want 51", len(ids))
			}
			pages, _ := countRequestsByPath(api.Requests(), path)
			if pages != 2 {
				t.Fatalf("page requests = %d, want 2", pages)
			}
		})
	}
}

// OFT-REQ-FR-01: partial refresh and resume behavior for seeded records.
func TestScrapePartial_RefreshesThenResumes(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		t.Run(kind, func(t *testing.T) {
			api := mock.NewJikan(t)
			path := "/v1/" + kind
			firstPage := searchPageResponse(1, 50, 1, 2)

			// 1st page: served twice, first partially (refresh) then normally.
			api.ExpectGET(path, searchQuery(1), firstPage)
			api.ExpectGET(path+"/1", nil, mock.JikanResponse{Body: map[string]any{
				"data": map[string]any{"mal_id": 1},
			}})
			api.ExpectGET(path, searchQuery(1), firstPage, firstPage)
			api.ExpectGET(path, searchQuery(2), searchPageResponse(51, 1, 2, 2))

			db, err := sync.OpenDatabase(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			// minimal state: id=1 exists in the kind-specific table
			if kind == "anime" {
				if _, err := db.Exec(`
					CREATE TABLE anime (id INTEGER, airing INTEGER);
					INSERT INTO anime VALUES (1, 1);
				`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`
					CREATE TABLE mangas (id INTEGER, status TEXT);
					INSERT INTO mangas VALUES (1, 'Publishing');
				`); err != nil {
					t.Fatal(err)
				}
			}

			cfg := config.Config{MaxAttempts: 1}
			count := 0
			if kind == "anime" {
				results := make(chan scrape.ScrapedAnime, 100)
				if err := scrape.ScrapePartialAnime(context.Background(), results, cfg, db); err != nil {
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
				if err := scrape.ScrapePartialManga(context.Background(), results, cfg, db); err != nil {
					t.Fatal(err)
				}
				for item := range results {
					if item.Relations != nil || item.Error != nil || item.Manga.MalId == 0 {
						t.Fatalf("unexpected manga result: %+v", item)
					}
					count++
				}
			}

			// 51 from search + 1 refresh of the seeded id.
			if count != 52 {
				t.Fatalf("items = %d, want 52", count)
			}
			pages, refreshes := countRequestsByPath(api.Requests(), path)
			if pages != 4 || refreshes != 1 {
				t.Fatalf("pages/refreshes = %d/%d, want 4/1", pages, refreshes)
			}
		})
	}
}

// OFT-REQ-FR-03: retry policy for transient Jikan status failures.
func TestScrapeAll_RetriesTransientStatuses(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		for _, status := range []int{429, 500, 502, 504} {
			t.Run(kind+"/"+http.StatusText(status), func(t *testing.T) {
				api := mock.NewJikan(t)
				path := "/v1/" + kind
				api.ExpectGET(path, searchQuery(1),
					mock.JikanResponse{StatusCode: status, Body: map[string]any{
						"status": status, "message": http.StatusText(status),
					}},
					searchPageResponse(7, 1, 1, 1),
				)
				ids, err := scrapeAll(t, kind, context.Background(), config.Config{MaxAttempts: 2})
				if err != nil {
					t.Fatal(err)
				}
				if len(ids) != 1 || ids[0] != 7 {
					t.Fatalf("ids = %v, want [7]", ids)
				}
				if got := len(api.Requests()); got != 2 {
					t.Fatalf("requests = %d, want 2", got)
				}
			})
		}
	}
}

// OFT-REQ-FR-03: terminal failure handling without partial results.
func TestScrapeAll_FailsWithoutResults(t *testing.T) {
	cases := []struct {
		name     string
		response mock.JikanResponse
		requests int
	}{
		{"retry exhaustion", mock.JikanResponse{StatusCode: 429, Body: map[string]any{"status": 429}}, 4},
		{"nonretryable status", mock.JikanResponse{StatusCode: 400, Body: map[string]any{"status": 400}}, 2},
		{"invalid JSON", mock.JikanResponse{Body: "{broken"}, 2},
		{"transport failure", mock.JikanResponse{Err: errors.New("connection reset")}, 2},
	}
	for _, kind := range []string{"anime", "manga"} {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				api := mock.NewJikan(t)
				for i := 0; i < tc.requests; i++ {
					api.ExpectGET("/v1/"+kind, searchQuery(1), tc.response)
				}
				ids, err := scrapeAll(t, kind, context.Background(), config.Config{MaxAttempts: 2})
				if err == nil || len(ids) != 0 {
					t.Fatalf("ids = %v, err = %v; want no items and an error", ids, err)
				}
				if got := len(api.Requests()); got != tc.requests {
					t.Fatalf("requests = %d, want %d", got, tc.requests)
				}
			})
		}
	}
}

// OFT-REQ-FR-03: empty-page termination and clean walk shutdown.
func TestScrapeAll_StopsOnEmptyPage(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		t.Run(kind, func(t *testing.T) {
			api := mock.NewJikan(t)
			api.ExpectGET("/v1/"+kind, searchQuery(1), searchPageResponse(1, 0, 1, 2))
			ids, err := scrapeAll(t, kind, context.Background(), config.Config{MaxAttempts: 1})
			if err != nil || len(ids) != 0 {
				t.Fatalf("ids = %v, err = %v", ids, err)
			}
			if got := len(api.Requests()); got != 1 {
				t.Fatalf("requests = %d, want 1", got)
			}
		})
	}
}

// OFT-REQ-FR-03: cancellation behavior during scrape execution.
func TestScrapeAll_HonoursCancellation(t *testing.T) {
	for _, kind := range []string{"anime", "manga"} {
		t.Run(kind, func(t *testing.T) {
			api := mock.NewJikan(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			ids, err := scrapeAll(t, kind, ctx, config.Config{MaxAttempts: 1})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
			if len(ids) != 0 || len(api.Requests()) != 0 {
				t.Fatalf("ids = %v, requests = %d; want 0/0", ids, len(api.Requests()))
			}
		})
	}
}

// Helper-integrity test: the mock restores jikan.Client and snapshots requests.
func TestJikanMock_RestoresClientAndSnapshotsRequests(t *testing.T) {
	previous := jikan.Client
	t.Cleanup(func() {
		if jikan.Client != previous {
			t.Fatal("Jikan client was not restored")
		}
	})

	api := mock.NewJikan(t)
	api.ExpectGET("/v1/anime", searchQuery(1), searchPageResponse(1, 0, 1, 1))
	if _, err := jikan.GetAnimeSearch(searchQuery(1)); err != nil {
		t.Fatal(err)
	}

	reqs := api.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	reqs[0].URL.Path = "/changed"
	if api.Requests()[0].URL.Path != "/v1/anime" {
		t.Fatal("Requests() returned mutable shared state")
	}
}
