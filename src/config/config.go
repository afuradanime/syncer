package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/ini.v1"
)

type Config struct {
	DbPath               string // Loaded from env
	ImageType            string
	UseQualityFiltering  bool
	QualityTiers         uint
	BlackListedTags      []string
	BlacklistedStudios   []string
	BlacklistedProducers []string
	BlackListedTypes     []string

	RequestTimeout time.Duration
	MaxAttempts    int
	BaseBackoff    time.Duration // exponential backoff
	SleepAmount    time.Duration // sleep between requests
}

func getStringList(section *ini.Section, name string, defaultValue []string) []string {
	if !section.HasKey(name) {
		return defaultValue
	}

	return section.Key(name).Strings(",")
}

func (c Config) Validate() error {
	if c.DbPath == "" {
		return fmt.Errorf("database path is empty")
	}

	switch c.ImageType {
	case "webp", "jpg":
		// Valid
	default:
		return fmt.Errorf("invalid image_type %q: must be webp or jpg", c.ImageType)
	}

	if c.QualityTiers == 0 || c.QualityTiers >= 10 {
		return fmt.Errorf("quality_tiers must be greater than 0 and smaller than 10, got %d", c.QualityTiers)
	}

	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request_timeout must be greater than 0, got %v", c.RequestTimeout)
	}

	if c.MaxAttempts <= 0 {
		return fmt.Errorf("max_attempts must be greater than 0, got %d", c.MaxAttempts)
	}

	if c.BaseBackoff <= 0 {
		return fmt.Errorf("base_backoff must be greater than 0, got %v", c.BaseBackoff)
	}

	if c.SleepAmount < 0 {
		return fmt.Errorf("sleep_amount must be non-negative, got %v", c.SleepAmount)
	}

	return nil
}

func LoadConfig(path string) (Config, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		return Config{}, fmt.Errorf("DATABASE_PATH environment variable is not set")
	}

	section := cfg.Section("Import")
	netSection := cfg.Section("Network")

	config := Config{
		DbPath:              dbPath,
		ImageType:           section.Key("image_type").MustString("webp"),
		UseQualityFiltering: section.Key("use_quality_filtering").MustBool(true),
		QualityTiers:        section.Key("quality_tiers").MustUint(3),

		BlackListedTags:      getStringList(section, "blacklisted_tags", []string{}),
		BlackListedTypes:     getStringList(section, "blacklisted_types", []string{}),
		BlacklistedStudios:   getStringList(section, "blacklisted_studios", []string{}),
		BlacklistedProducers: getStringList(section, "blacklisted_producers", []string{}),

		RequestTimeout: netSection.Key("request_timeout").MustDuration(10 * time.Second),
		MaxAttempts:    netSection.Key("max_attempts").MustInt(3),
		BaseBackoff:    netSection.Key("base_backoff").MustDuration(200 * time.Millisecond),
		SleepAmount:    netSection.Key("sleep_amount").MustDuration(500 * time.Millisecond),
	}

	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}
