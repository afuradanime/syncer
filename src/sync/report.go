package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type SyncError struct {
	MalID  int    `json:"mal_id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

type SyncReport struct {
	TotalProcessed int         `json:"total_processed"`
	SuccessCount   int         `json:"success_count"`
	SkippedCount   int         `json:"skipped_count"`
	SkippedEntries []SyncError `json:"skipped_entries"`
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
