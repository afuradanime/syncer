CREATE TABLE anime_type (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL
);

INSERT INTO anime_type (name) VALUES
    ('TV'),
    ('OVA'),
    ('Movie'),
    ('Special'),
    ('ONA'),
    ('Music'),
    ('Unknown');

CREATE TABLE company_type (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL
);

INSERT INTO company_type (name) VALUES
    ('Producer'),
    ('Licensor'),
    ('Studio');


CREATE TABLE anime_status (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL
);

INSERT INTO anime_status (name) VALUES 
    ('Finished Airing'),
    ('Currently Airing'),
    ('Not yet aired'),
    ('Unknown');

CREATE TABLE relation_types (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL  -- 'prequel', 'sequel', 'side story', 'parent story', 'summary', 'alternative version', 'alternative setting', 'spin-off', 'other'
);

INSERT INTO relation_types (name) VALUES
    ('Parent Story'),
    ('Sequel'),
    ('Character'),
    ('Other'),
    ('Spin-Off'),
    ('Summary'),
    ('Adaptation'),
    ('Alternative Version'),
    ('Side Story'),
    ('Full Story'),
    ('Alternative Setting'),
    ('Prequel');

CREATE TABLE anime (
    id INTEGER PRIMARY KEY,  -- MAL ID
    url TEXT,
    title TEXT NOT NULL,
    type_id INTEGER NOT NULL,
    source TEXT,
    episodes INTEGER,
    status_id INTEGER NOT NULL,
    airing BOOLEAN DEFAULT 0,
    duration TEXT,
    quality_score BOOLEAN DEFAULT 0,
    start_date DATETIME,
    end_date DATETIME,
    season TEXT,
    year INTEGER,
    broadcast_day TEXT,
    broadcast_time TEXT,
    broadcast_timezone TEXT,
    image_url TEXT,
    small_image_url TEXT,
    large_image_url TEXT,
    trailer_embed_url TEXT,
    
    FOREIGN KEY (type_id) REFERENCES anime_type(id),
    FOREIGN KEY (status_id) REFERENCES anime_status(id)
);

CREATE TABLE synonyms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    anime_id INTEGER NOT NULL,
    type TEXT NOT NULL,  -- 'Default', 'Japanese', 'English', 'Synonym'
    title TEXT NOT NULL,
    FOREIGN KEY (anime_id) REFERENCES anime(id) ON DELETE CASCADE
);

CREATE TABLE anime_descriptions (
    anime_id INTEGER NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (anime_id),
    FOREIGN KEY (anime_id) REFERENCES anime(id) ON DELETE CASCADE
);

CREATE TABLE companies (
    mal_id  int PRIMARY KEY,
    name    varchar(100) NOT NULL,
    mal_url varchar,
    type_id int NOT NULL,
    FOREIGN KEY (type_id) REFERENCES company_type(id)
);

CREATE TABLE anime_companies (
    anime_id   int NOT NULL REFERENCES anime(id)         ON DELETE CASCADE,
    company_id int NOT NULL REFERENCES companies(mal_id) ON DELETE CASCADE,
    role       varchar(25) NOT NULL,
    PRIMARY KEY (anime_id, company_id, role)
);

CREATE TABLE tags (
    id INTEGER PRIMARY KEY,  -- MAL ID
    name TEXT UNIQUE NOT NULL,
    type TEXT NOT NULL,  -- 'genre', 'theme', 'demographic', 'explicit_genre'
    url TEXT
);

CREATE TABLE anime_tags (
    anime_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (anime_id, tag_id),
    FOREIGN KEY (anime_id) REFERENCES anime(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

CREATE TABLE anime_relations (
    anime_id INTEGER NOT NULL,
    relation_type_id INTEGER NOT NULL,
    related_anime_id INTEGER NOT NULL,
    PRIMARY KEY (anime_id, relation_type_id, related_anime_id),
    FOREIGN KEY (anime_id) REFERENCES anime(id) ON DELETE CASCADE,
    FOREIGN KEY (relation_type_id) REFERENCES relation_types(id) ON DELETE CASCADE,
    FOREIGN KEY (related_anime_id) REFERENCES anime(id) ON DELETE CASCADE
);


CREATE TABLE mangas (
    id              int PRIMARY KEY,
    title           varchar NOT NULL,
    title_english   varchar,
    title_japanese  varchar,
    synopsis        text,
    kind            varchar(25) NOT NULL,
    status          varchar(25) NOT NULL,
    num_chapters    int,
    num_volumes     int,
    start_date      date,
    end_date        date,
    cover_url_small varchar,
    cover_url_large varchar
);

CREATE TABLE manga_synonyms (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    manga_id int         NOT NULL REFERENCES mangas(id) ON DELETE CASCADE,
    type     varchar(25) NOT NULL,
    title    varchar(255) NOT NULL,
    UNIQUE (manga_id, type, title)
);

CREATE TABLE manga_descriptions (
    manga_id    int PRIMARY KEY REFERENCES mangas(id) ON DELETE CASCADE,
    description text NOT NULL
);

CREATE TABLE manga_tags (
    manga_id int NOT NULL REFERENCES mangas(id) ON DELETE CASCADE,
    tag_id   int NOT NULL REFERENCES tags(id)        ON DELETE CASCADE,
    PRIMARY KEY (manga_id, tag_id)
);

-- individuals credited on a manga
CREATE TABLE people (
    mal_id  int PRIMARY KEY,
    name    varchar(150) NOT NULL,
    mal_url varchar
);

CREATE TABLE manga_authors (
    manga_id  int NOT NULL REFERENCES mangas(id) ON DELETE CASCADE,
    person_id int NOT NULL REFERENCES people(mal_id) ON DELETE CASCADE,
    role      varchar(25) NOT NULL,  -- 'Story', 'Art', 'Story & Art'
    PRIMARY KEY (manga_id, person_id, role)
);

CREATE TABLE manga_relations (
    manga_id         int NOT NULL REFERENCES mangas(id) ON DELETE CASCADE,
    related_manga_id int NOT NULL REFERENCES mangas(id) ON DELETE CASCADE,
    relation_type    varchar(25) NOT NULL,
    PRIMARY KEY (manga_id, related_manga_id, relation_type)
);

-- Index for filtering by manga tag
CREATE INDEX idx_manga_tags_tag ON manga_tags(tag_id);
-- Index for finding all manga by an author
CREATE INDEX idx_manga_authors_person ON manga_authors(person_id, role);
-- Index for filtering by anime season
CREATE INDEX idx_anime_year_season ON anime(year, season);
-- Index for filtering currently airing anime
CREATE INDEX idx_anime_airing ON anime(airing) WHERE airing = 1;
-- Index for title searching
CREATE INDEX idx_anime_title ON anime(title COLLATE NOCASE);
-- Index for looking up anime by any title variant
CREATE INDEX idx_synonyms_title ON synonyms(title COLLATE NOCASE);
CREATE INDEX idx_synonyms_anime_id ON synonyms(anime_id);
-- Index for finding anime by genre
CREATE INDEX idx_anime_tags_tag_id ON anime_tags(tag_id);
-- Index for getting all tags of an anime
CREATE INDEX idx_anime_tags_anime_id ON anime_tags(anime_id);
CREATE INDEX idx_tags_type_name ON tags(type, name);
-- Index for finding related anime
CREATE INDEX idx_anime_relations_anime_id ON anime_relations(anime_id);
CREATE INDEX idx_anime_relations_related_anime_id ON anime_relations(related_anime_id);
-- Index for getting descriptions by anime_id
CREATE INDEX idx_anime_descriptions_anime_id ON anime_descriptions(anime_id);
-- Index for getting companies by type
CREATE INDEX idx_companies_type_id ON companies(type_id);
CREATE INDEX idx_anime_companies_company ON anime_companies(company_id, role);

-- Views
CREATE VIEW IF NOT EXISTS random_anime
AS
	SELECT a.id, a.url, a.title, a.type_id, a.source, a.episodes, a.status_id, a.airing, a.duration, a.start_date, a.end_date, a.season, a.year, a.broadcast_day, a.broadcast_time, a.broadcast_timezone, a.image_url, a.small_image_url, a.large_image_url, a.trailer_embed_url
    FROM anime a
    ORDER BY RANDOM() 
    LIMIT 1;
