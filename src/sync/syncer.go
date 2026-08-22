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

	return fmt.Errorf("Not implemented yet")
}

func processAnimeQueue(results <-chan scrape.ScrapedAnime, wg *gosync.WaitGroup, persister *store.Persister) {
	defer wg.Done()

	processedCount := 0

	for item := range results {
		processedCount++

		if item.Error != nil {
			fmt.Printf("[Persistency Worker error] Failed relations for MalID %d: %v\n", item.Anime.MalId, item.Error)
		} else {
			fmt.Printf("[Persistency Worker] Persisting MalID %d: %s\n", item.Anime.MalId, item.Anime.Title)
			persister.InsertScrapedAnime(context.Background(), item)
		}
	}

	fmt.Printf("[Persistency Worker] Finished processing all queue items. Total: %d\n", processedCount)
}

func (s *Syncer) RunFullSync() error {

	// Create a temporary database
	databasePath, err := CreateTempDatabase(s.Config.DbPath)
	if err != nil {
		return fmt.Errorf("create temporary database: %w", err)
	}

	fmt.Printf("Creating temporary database at: %s\n", databasePath)

	db, err := OpenDatabase(databasePath)
	if err != nil {
		return fmt.Errorf("open temporary database: %w", err)
	}

	if err := CreateDatabaseSchema(databasePath, db); err != nil {
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

	// Scrape all jikan data & insert it into the database in parallel
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt) // Safe os context
	defer stop()

	// Buffer of 100 for the scraper to write to
	resultsQueue := make(chan scrape.ScrapedAnime, 100)
	var wg gosync.WaitGroup

	wg.Add(1)
	go processAnimeQueue(resultsQueue, &wg, persister)

	err = scrape.ScrapeAllAnime(ctx, resultsQueue, s.Config)
	if err != nil {
		return fmt.Errorf("scrape all anime: %w", err)
	}

	wg.Wait()

	if err := publish.SetupDatabasePragmas(db); err != nil {
		return fmt.Errorf("setup database pragmas failed: %w", err)
	}

	if err := closeDB(); err != nil {
		return fmt.Errorf("close temporary database: %w", err)
	}

	runVersion := fmt.Sprintf("%d", time.Now().Unix())

	if err := publish.PublishDatabase(databasePath, s.Config.DbPath, runVersion); err != nil {
		return fmt.Errorf("publication failed: %w", err)
	}

	fmt.Printf("[Syncer] Published database to: %s\n", s.Config.DbPath)

	return nil
}
