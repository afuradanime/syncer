package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type Manifest struct {
	Filename      string `json:"filename"`
	SchemaVersion string `json:"schema_version"`
	GeneratedAt   string `json:"generated_at"`
	Version       string `json:"version"`
	Checksum      string `json:"checksum"`
	Size          int64  `json:"size"`
}

func CalculateChecksum(dbPath, version string) (*string, error) {

	dbFile, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open temp db for hashing: %w", err)
	}
	defer dbFile.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, dbFile); err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}
	return new(hex.EncodeToString(hasher.Sum(nil))), nil
}

func CreateManifest(dbPath, version string) (*Manifest, error) {
	stat, err := os.Stat(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat db file: %w", err)
	}

	checksum, err := CalculateChecksum(dbPath, version)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	manifest := &Manifest{
		Filename:      dbPath,
		SchemaVersion: "1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Version:       version,
		Checksum:      *checksum,
		Size:          stat.Size(),
	}

	return manifest, nil
}

func WriteManifest(file *os.File, manifest *Manifest) error {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	return nil
}

func ReadManifest(dbPath string) (*Manifest, error) {
	manifestPath := dbPath + ".manifest.json"
	file, err := os.Open(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest file: %w", err)
	}
	defer file.Close()

	var manifest Manifest
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	return &manifest, nil
}
