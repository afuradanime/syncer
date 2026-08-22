package publish

import (
	"database/sql"
	"fmt"
	"os"
)

func PublishDatabase(tempPath, destPath string, version string) error {

	manifest, err := CreateManifest(tempPath, version)
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}

	tempManifestPath := tempPath + ".manifest.json"
	destManifestPath := destPath + ".manifest.json"

	// Write Manifest to a temp file
	manifestFile, err := os.Create(tempManifestPath)
	if err != nil {
		return fmt.Errorf("create temp manifest file: %w", err)
	}

	if err := WriteManifest(manifestFile, manifest); err != nil {
		manifestFile.Close()
		return fmt.Errorf("write manifest: %w", err)
	}
	manifestFile.Close()

	// Atomic renames
	if err := os.Rename(tempPath, destPath); err != nil {
		os.Remove(tempManifestPath) // Clean up the temp manifest if DB publish fails
		return fmt.Errorf("rename temp db to final destination: %w", err)
	}

	// Rename the manifest
	if err := os.Rename(tempManifestPath, destManifestPath); err != nil {
		return fmt.Errorf("rename temp manifest to final destination: %w", err)
	}

	// File readonly
	if err := os.Chmod(destPath, 0444); err != nil {
		fmt.Printf("[Warning] Failed to set database to read-only: %v\n", err)
	}

	fmt.Printf("[Publisher] Published database to: %s\n", destPath)
	fmt.Printf("[Publisher] Manifest written to: %s\n", destManifestPath)

	return nil
}

func SetupDatabasePragmas(db *sql.DB) error {
	// Enable foreign key constraints
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set journal mode to WAL for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("failed to set journal mode to WAL: %w", err)
	}

	// Set synchronous mode to NORMAL for a balance between safety and performance
	if _, err := db.Exec("PRAGMA synchronous = NORMAL"); err != nil {
		return fmt.Errorf("failed to set synchronous mode: %w", err)
	}

	// Set to read-only mode for consumers to prevent accidental writes
	if _, err := db.Exec("PRAGMA query_only = ON"); err != nil {
		return fmt.Errorf("failed to set query_only mode: %w", err)
	}

	return nil
}
