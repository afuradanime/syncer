package sync

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema/schema.sql
var initMigrationSQL string

func CreateTempDatabase(destination string) (string, error) {
	dir := filepath.Dir(destination)
	base := filepath.Base(destination)

	f, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary database: %w", err)
	}

	path := f.Name()

	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("close temporary database: %w", err)
	}

	return path, nil
}

func OpenDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	return db, nil
}

func CloseDatabase(db *sql.DB) error {
	if err := db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	return nil
}

func CreateDatabaseSchema(path string, db *sql.DB) error {

	_, err := db.Exec(initMigrationSQL)
	if err != nil {
		return fmt.Errorf("create database schema: %w", err)
	}

	return nil
}
