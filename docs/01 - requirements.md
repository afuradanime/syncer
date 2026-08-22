# Anime syncher requirements

## Table of contents

- [Anime syncher requirements](#anime-syncher-requirements)
  - [Table of contents](#table-of-contents)
  - [Overview](#overview)
  - [Requirement definition](#requirement-definition)
    - [FR-01: partial synchronisation](#fr-01-partial-synchronisation)
    - [FR-02: full synchronisation](#fr-02-full-synchronisation)
    - [FR-03: fetch resilience](#fr-03-fetch-resilience)
    - [FR-04: configuration](#fr-04-configuration)
    - [FR-05: version manifest](#fr-05-version-manifest)
  - [Non-functional requirements](#non-functional-requirements)
    - [NFR-01: database configuration](#nfr-01-database-configuration)
    - [NFR-02: database update atomicity](#nfr-02-database-update-atomicity)
    - [NFR-03: synchronisation schedule](#nfr-03-synchronisation-schedule)
    - [NFR-04: data integrity verification](#nfr-04-data-integrity-verification)


## Overview
The Anime Syncher is a command line application that scrapes anime data from the open-source Jikan API and persists it to a local SQLite3 database, enabling fast local querying without repeated network calls.
## Requirement definition
### FR-01: partial synchronisation

The system shall be able to perform a cheap incremental update to the database

- The system shall insert any new anime/manga that were added to the jikan API 
- The system shall update all airing anime currently in the database
- The system shall iteratively update or insert all the related entities of the newly added anime (i.e. studios, producers, licensors, related anime)
- The system shall abort the operation if the database file is not present
- The system shall replace the existing database on a successful operation

### FR-02: full synchronisation

The system shall insert or update all relevant entities present in the jikan API
- The system shall create a new database file for a full synchronization
- The system shall replace any existing database on a successful operation

### FR-03: fetch resilience

The system shall retry failed network requests, up to a configurable maximum number of attempts

- The system shall respect jikan's rate limiting by pausing and resuming
- A failure to fetch or persist a single anime shall not abort the entire sync; it shall be logged and skipped, with a final report of skipped entries, including the error occured

### FR-04: configuration

The system shall provide command line options to customise the syncher's behaviours

- The system shall allow the user to choose a sync strategy (partial or full) via a flag
- The system shall allow the user to specify a config file with importing rules (e.g. which image format to store, which tags to blacklist, etc)
- The system shall require the database path to be configured as an environment variable named `DATABASE_PATH`

### FR-05: version manifest

The system shall publish a manifest alongside each generated database file, describing that database's version

- The manifest shall include a version identifier (e.g. a timestamp) of the generated database file
- The manifest shall include a checksum (SHA-256) of the generated database file
- The manifest shall include the size in bytes of the generated database file

## Non-functional requirements
### NFR-01: database configuration

The system shall output a read-only, performance configured, SQLite3 database file
### NFR-02: database update atomicity

The system shall update the database atomically

- The system shall write the database to a temporary path during generation
- The system shall only make the database available to consumers by renaming the temporary file into its final path once generation and verification (see NFR-04) have completed successfully
- The system shall retain the previous database version until the new version has been successfully published, to allow rollback if publication fails

### NFR-03: synchronisation schedule

The system shall be configured to run periodically

- The system shall perform a partial update every day at 2AM UTC+0
- The system shall perform a full update at the start of the month at 2AM UTC+0
- The system shall not do a partial update if a full update is scheduled for the same day

### NFR-04: data integrity verification

The system shall verify the integrity of the database file before it is published

- Consuming instances shall verify a downloaded database file's checksum against the manifest before swapping it into use, and shall discard the file on mismatch
- Consuming instances shall expose their database's manifest version for observability purposes