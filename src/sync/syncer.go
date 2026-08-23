package sync

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	gosync "sync"
	"syncer/src/config"
	"syncer/src/publish"
	"syncer/src/scrape"
	"syncer/src/store"
	"time"
)

type Syncer struct {
	Config config.Config
}

func (s *Syncer) RunPartialSync() error {

	_, err := os.Stat(s.Config.DbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("database path does not exist: %q", s.Config.DbPath)
		}

		return fmt.Errorf("cannot access database path %q: %w", s.Config.DbPath, err)
	}

	// Check manifest
	manifest, err := publish.ReadManifest(s.Config.DbPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	checksum, err := publish.CalculateChecksum(s.Config.DbPath, manifest.Version)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	if *checksum != manifest.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", manifest.Checksum, *checksum)
	}

	if publish.CURRENT_SCHEMA_VERSION != manifest.SchemaVersion {
		return fmt.Errorf("schema version mismatch: expected %d, got %d", manifest.SchemaVersion, publish.CURRENT_SCHEMA_VERSION)
	}

	fmt.Printf("[Syncer] Checksum verified for database: %s\n", *checksum)

	// Create a temporary copy of the database for partial sync
	tempDbPath, err := CopyDatabaseToTemp(s.Config.DbPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary database copy: %w", err)
	}

	fmt.Printf("[Syncer] Created temporary database copy for partial sync at: %s\n", tempDbPath)

	// Prepare for reading
	db, err := OpenDatabase(tempDbPath)
	if err != nil {
		return fmt.Errorf("failed to open temporary database copy: %w", err)
	}
	dbClosed := false
	closeDB := func() error {
		if dbClosed {
			return nil
		}
		dbClosed = true
		return CloseDatabase(db)
	}
	defer closeDB()

	if os.Chmod(tempDbPath, 0777) != nil {
		return fmt.Errorf("failed to set temporary database copy to readable: %w", err)
	}

	if _, err := db.Exec("PRAGMA query_only = OFF"); err != nil {
		return fmt.Errorf("failed to set query_only mode: %w", err)
	}

	lookups, err := store.LoadLookups(db)
	if err != nil {
		return fmt.Errorf("load database lookups: %w", err)
	}

	persister := store.NewPersister(db, lookups, s.Config)

	syncReport := &SyncReport{
		SkippedEntries: make([]SyncError, 0),
		SyncMode:       "partial",
	}

	// Scrape jikan data from last database insert
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	resultsQueue := make(chan scrape.ScrapedAnime, 100)
	mangaQueue := make(chan scrape.ScrapedManga, 100)
	var wg gosync.WaitGroup

	wg.Add(2)
	go processAnimeQueue(resultsQueue, &wg, persister, syncReport)
	go processMangaQueue(mangaQueue, &wg, persister, syncReport)

	syncReport.AnimeScrapeStartedAt = time.Now().UTC().Format(time.RFC3339)
	err = scrape.ScrapePartialAnime(ctx, resultsQueue, s.Config, db)
	if err != nil {
		return fmt.Errorf("scrape partial anime: %w", err)
	}

	syncReport.AnimeScrapeEndedAt = time.Now().UTC().Format(time.RFC3339)
	syncReport.MangaScrapeStartedAt = time.Now().UTC().Format(time.RFC3339)
	if err := scrape.ScrapePartialManga(ctx, mangaQueue, s.Config, db); err != nil {
		return fmt.Errorf("scrape partial manga: %w", err)
	}
	syncReport.MangaScrapeEndedAt = time.Now().UTC().Format(time.RFC3339)

	wg.Wait()

	if err := publish.SetupDatabasePragmas(db); err != nil {
		return fmt.Errorf("setup database pragmas failed: %w", err)
	}

	if err := closeDB(); err != nil {
		return fmt.Errorf("close temporary database: %w", err)
	}

	runVersion := fmt.Sprintf("%d", time.Now().Unix())

	if err := publish.PublishDatabase(tempDbPath, s.Config.DbPath, runVersion); err != nil {
		return fmt.Errorf("publication failed: %w", err)
	}

	fmt.Printf("[Syncer] Published partial sync database to: %s (Version: %s)\n", s.Config.DbPath, runVersion)

	if reportPath, err := SaveReport(s.Config.DbPath, runVersion, syncReport); err != nil {
		fmt.Printf("[Warning] Failed to write sync report %v\n", err)
	} else {
		fmt.Printf("[Syncer] Wrote sync report to: %s\n", reportPath)
	}

	return nil
}

func processAnimeQueue(results <-chan scrape.ScrapedAnime, wg *gosync.WaitGroup, persister *store.Persister, report *SyncReport) {
	defer wg.Done()

	processedCount := 0

	for item := range results {
		processedCount++

		if item.Error != nil {

			fmt.Printf("[Persistency Worker error] Failed relations for MalID %d: %v\n", item.Anime.MalId, item.Error)

			// Record the failure
			report.mu.Lock()
			report.SkippedCount++
			report.SkippedEntries = append(report.SkippedEntries, SyncError{
				MalID:  item.Anime.MalId,
				Title:  item.Anime.Title,
				Reason: item.Error.Error(),
			})
			report.mu.Unlock()
		} else {

			fmt.Printf("[Persistency Worker] Persisting MalID %d: %s\n", item.Anime.MalId, item.Anime.Title)
			persister.InsertScrapedAnime(context.Background(), item)
			report.mu.Lock()
			report.SuccessCount++
			report.mu.Unlock()
		}
	}

	fmt.Printf("[Persistency Worker] Finished processing. Total: %d, Success: %d, Skipped: %d\n",
		report.TotalProcessed, report.SuccessCount, report.SkippedCount)
}

func processMangaQueue(results <-chan scrape.ScrapedManga, wg *gosync.WaitGroup, persister *store.Persister, report *SyncReport) {
	defer wg.Done()

	for item := range results {

		if item.Error != nil {

			report.mu.Lock()
			report.SkippedCount++
			report.SkippedEntries = append(report.SkippedEntries, SyncError{
				MalID:  item.Manga.MalId,
				Title:  item.Manga.Title,
				Reason: item.Error.Error(),
			})

			report.mu.Unlock()
			continue
		}

		if err := persister.InsertScrapedManga(context.Background(), item); err != nil {

			report.mu.Lock()
			report.SkippedCount++
			report.SkippedEntries = append(report.SkippedEntries, SyncError{
				MalID:  item.Manga.MalId,
				Title:  item.Manga.Title,
				Reason: err.Error(),
			})

			report.mu.Unlock()
			continue
		}

		report.mu.Lock()
		report.SuccessCount++
		report.mu.Unlock()
	}
}

func (s *Syncer) RunFullSync() error {

	// Create a temporary database
	tempDbPath, err := CreateTempDatabase(s.Config.DbPath)
	if err != nil {
		return fmt.Errorf("create temporary database: %w", err)
	}

	fmt.Printf("Creating temporary database at: %s\n", tempDbPath)

	db, err := OpenDatabase(tempDbPath)
	if err != nil {
		return fmt.Errorf("open temporary database: %w", err)
	}

	if err := CreateDatabaseSchema(tempDbPath, db); err != nil {
		return fmt.Errorf("create database schema: %w", err)
	}

	// Closing is done before publishing (below), but we
	// still want the DB to close on error paths, so this is a
	// safety net
	dbClosed := false
	closeDB := func() error {
		if dbClosed {
			return nil
		}
		dbClosed = true
		return CloseDatabase(db)
	}
	defer closeDB()

	lookups, err := store.LoadLookups(db)
	if err != nil {
		return fmt.Errorf("load database lookups: %w", err)
	}

	persister := store.NewPersister(db, lookups, s.Config)

	syncReport := &SyncReport{
		SkippedEntries: make([]SyncError, 0),
		SyncMode:       "full",
	}

	// Scrape all jikan data & insert it into the database in parallel
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt) // Safe os context
	defer stop()

	// Buffer of 100 for the scraper to write to
	resultsQueue := make(chan scrape.ScrapedAnime, 100)
	mangaQueue := make(chan scrape.ScrapedManga, 100)
	var wg gosync.WaitGroup

	wg.Add(2)
	go processAnimeQueue(resultsQueue, &wg, persister, syncReport)
	go processMangaQueue(mangaQueue, &wg, persister, syncReport)

	syncReport.AnimeScrapeStartedAt = time.Now().UTC().Format(time.RFC3339)
	err = scrape.ScrapeAllAnime(ctx, resultsQueue, s.Config)
	if err != nil {
		return fmt.Errorf("scrape all anime: %w", err)
	}

	syncReport.AnimeScrapeEndedAt = time.Now().UTC().Format(time.RFC3339)
	syncReport.MangaScrapeStartedAt = time.Now().UTC().Format(time.RFC3339)
	if err := scrape.ScrapeAllManga(ctx, mangaQueue, s.Config); err != nil {
		return fmt.Errorf("scrape all manga: %w", err)
	}
	syncReport.MangaScrapeEndedAt = time.Now().UTC().Format(time.RFC3339)

	wg.Wait()

	if err := publish.SetupDatabasePragmas(db); err != nil {
		return fmt.Errorf("setup database pragmas failed: %w", err)
	}

	if err := closeDB(); err != nil {
		return fmt.Errorf("close temporary database: %w", err)
	}

	runVersion := fmt.Sprintf("%d", time.Now().Unix())

	if err := publish.PublishDatabase(tempDbPath, s.Config.DbPath, runVersion); err != nil {
		return fmt.Errorf("publication failed: %w", err)
	}

	fmt.Printf("[Syncer] Published database to: %s\n", s.Config.DbPath)

	if reportPath, err := SaveReport(s.Config.DbPath, runVersion, syncReport); err != nil {
		fmt.Printf("[Warning] Failed to write sync report %v\n", err)
	} else {
		fmt.Printf("[Syncer] Wrote sync report to: %s\n", reportPath)
	}

	return nil
}
