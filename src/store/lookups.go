package store

import (
	"database/sql"
	"fmt"
)

type Lookups struct {
	animeTypeIDs    map[string]int
	animeStatusIDs  map[string]int
	companyTypeIDs  map[string]int
	relationTypeIDs map[string]int
}

func LoadLookups(db *sql.DB) (*Lookups, error) {
	l := &Lookups{
		animeTypeIDs:    map[string]int{},
		animeStatusIDs:  map[string]int{},
		companyTypeIDs:  map[string]int{},
		relationTypeIDs: map[string]int{},
	}

	tables := []struct {
		table string
		dst   map[string]int
	}{
		{"anime_type", l.animeTypeIDs},
		{"anime_status", l.animeStatusIDs},
		{"company_type", l.companyTypeIDs},
		{"relation_types", l.relationTypeIDs},
	}

	for _, t := range tables {
		if err := loadLookupTable(db, t.table, t.dst); err != nil {
			return nil, err
		}
	}

	return l, nil
}

func loadLookupTable(db *sql.DB, table string, dst map[string]int) error {
	rows, err := db.Query(fmt.Sprintf("SELECT id, name FROM %s", table))
	if err != nil {
		return fmt.Errorf("load lookup %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return fmt.Errorf("scan lookup %s: %w", table, err)
		}
		dst[name] = id
	}

	return rows.Err()
}

func (l *Lookups) AnimeTypeID(name string) int {
	if id, ok := l.animeTypeIDs[name]; ok {
		return id
	}
	return l.animeTypeIDs["Unknown"]
}

func (l *Lookups) AnimeStatusID(name string) int {
	if id, ok := l.animeStatusIDs[name]; ok {
		return id
	}
	return l.animeStatusIDs["Unknown"]
}

func (l *Lookups) CompanyTypeID(role string) (int, bool) {
	id, ok := l.companyTypeIDs[role]
	return id, ok
}

func (l *Lookups) RelationTypeID(relation string) (int, bool) {
	id, ok := l.relationTypeIDs[relation]
	return id, ok
}
