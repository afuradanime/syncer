package store

import (
	"strings"
	"syncer/src/config"

	jikan "github.com/afuradanime/tenrai-go"
)

func CalculateQualityScore(score float64, opts config.Config) int {
	if !opts.UseQualityFiltering || opts.QualityTiers <= 0 || score <= 0 {
		return -1
	}
	tierDivision := 10.0 / float64(opts.QualityTiers)
	return int(score / tierDivision)
}

func IsBlacklisted(anime jikan.AnimeBase, opts config.Config) bool {

	if containsFolds(opts.BlacklistedStudios, anime.Studios) {
		return true
	}

	if containsFolds(opts.BlacklistedProducers, anime.Producers) {
		return true
	}

	if containsFold(opts.BlackListedTypes, anime.Type) {
		return true
	}

	tagGroups := [][]jikan.MalItem{
		anime.Genres,
		anime.ExplicitGenres,
		anime.Themes,
		anime.Demographics,
	}

	for _, group := range tagGroups {
		for _, tag := range group {
			if containsFold(opts.BlackListedTags, tag.Name) {
				return true
			}
		}
	}

	return false
}

func containsFolds(list []string, value []jikan.MalItem) bool {
	if len(value) == 0 {
		return false
	}

	for _, v := range value {
		if containsFold(list, v.Name) {
			return true
		}
	}

	return false
}

func containsFold(list []string, value string) bool {
	if value == "" {
		return false
	}
	for _, v := range list {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}
