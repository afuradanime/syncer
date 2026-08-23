package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	gosync "sync"
)

type SyncError struct {
	MalID  int    `json:"mal_id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

type SyncReport struct {
	mu                   gosync.Mutex
	TotalProcessed       int         `json:"total_processed"`
	SuccessCount         int         `json:"success_count"`
	SkippedCount         int         `json:"skipped_count"`
	SkippedEntries       []SyncError `json:"skipped_entries"`
	AnimeScrapeStartedAt string      `json:"anime_scrape_started_at"`
	AnimeScrapeEndedAt   string      `json:"anime_scrape_ended_at"`
	MangaScrapeStartedAt string      `json:"manga_scrape_started_at"`
	MangaScrapeEndedAt   string      `json:"manga_scrape_ended_at"`
	SyncMode             string      `json:"sync_mode"`
}

func SaveReport(path, version string, report *SyncReport) (string, error) {

	reportPath := filepath.Join(filepath.Dir(path), fmt.Sprintf("sync_report_%s.json", version))
	file, err := os.Create(reportPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return reportPath, encoder.Encode(report)
}
