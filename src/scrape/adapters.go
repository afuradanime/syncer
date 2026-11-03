package scrape

import (
	"net/url"

	jikan "github.com/afuradanime/tenrai-go"
)

type fetchAdapter[T, Result any] struct {
	name        string
	lastIDQuery string
	activeQuery string
	search      func(url.Values) (searchPage[T], error)
	byID        func(int) (T, error)
	id          func(T) int
	result      func(T, int, error) Result
}

var animeAdapter = fetchAdapter[jikan.AnimeBase, ScrapedAnime]{
	name:        "anime",
	lastIDQuery: "SELECT id FROM anime ORDER BY id DESC LIMIT 1",
	activeQuery: "SELECT id FROM anime WHERE airing = 1",
	search: func(query url.Values) (searchPage[jikan.AnimeBase], error) {
		response, err := jikan.GetAnimeSearch(query)
		if err != nil {
			return searchPage[jikan.AnimeBase]{}, err
		}
		return searchPage[jikan.AnimeBase]{response.Data, response.Pagination.HasNextPage, response.Pagination.LastVisiblePage}, nil
	},
	byID: func(id int) (jikan.AnimeBase, error) {
		response, err := jikan.GetAnimeById(id)
		if err != nil {
			return jikan.AnimeBase{}, err
		}
		return response.Data, nil
	},
	id: func(anime jikan.AnimeBase) int { return anime.MalId },
	result: func(anime jikan.AnimeBase, id int, err error) ScrapedAnime {
		if err != nil {
			anime = jikan.AnimeBase{MalId: id}
		}
		return ScrapedAnime{Anime: anime, Error: err}
	},
}

var mangaAdapter = fetchAdapter[jikan.MangaBase, ScrapedManga]{
	name:        "manga",
	lastIDQuery: "SELECT id FROM manga ORDER BY id DESC LIMIT 1",
	activeQuery: "SELECT id FROM manga WHERE status = 'Publishing'",
	search: func(query url.Values) (searchPage[jikan.MangaBase], error) {
		response, err := jikan.GetMangaSearch(query)
		if err != nil {
			return searchPage[jikan.MangaBase]{}, err
		}
		return searchPage[jikan.MangaBase]{response.Data, response.Pagination.HasNextPage, response.Pagination.LastVisiblePage}, nil
	},
	byID: func(id int) (jikan.MangaBase, error) {
		response, err := jikan.GetMangaById(id)
		if err != nil {
			return jikan.MangaBase{}, err
		}
		return response.Data, nil
	},
	id: func(manga jikan.MangaBase) int { return manga.MalId },
	result: func(manga jikan.MangaBase, id int, err error) ScrapedManga {
		if err != nil {
			manga = jikan.MangaBase{MalId: id}
		}
		return ScrapedManga{Manga: manga, Error: err}
	},
}
