import hashlib
import json
import sqlite3
from contextlib import closing
from pathlib import Path

import pytest

import syncer_helper
from tools.jikan_mock.jikan_mock import JikanHandler


@pytest.mark.system
# OFT-REQ-FR-04: the CLI exposes the selected sync mode.
def test001_syncer_build(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    """
    Test that the syncer binary was built and is executable.

    Preconditions:
    - The syncer binary exists and is executable.
    - A valid configuration file is provided.

    Test steps:
    1. Run the syncer binary with the --help flag.

    Expected results:
    - The syncer binary runs successfully and displays the help message.
    - The output contains the -mode option.
    """
    result = syncer_helper.run_syncer(
        syncer_binary,
        "--help",
        config_path=config_path,
        database_path=database_path,
        working_directory=tmp_path,
    )

    assert result.returncode == 0, result.stdout + result.stderr
    assert "-mode" in result.stdout + result.stderr


@pytest.mark.system
# OFT-REQ-FR-01: partial sync inserts new records, refreshes active records, and preserves edits.
def test002_syncer_partial_sync_plan(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    """
    Test the partial sync workflow for existing and newly added records.

    Preconditions:
    - The syncer binary exists and is executable.
    - A valid configuration file is provided.
    - A populated test database with existing anime and manga rows is available.

    Test steps:
    1. Run the syncer in partial-sync mode.
    2. Check that new rows are inserted.
    3. Check that active rows are refreshed.
    4. Check that unchanged rows remain untouched.

    Expected results:
    - The database is updated incrementally without replacing the existing version.
    - Newly added records are inserted and stale records are refreshed.
    - Existing user edits are preserved.
    """
    initial = syncer_helper.run_mode(
        syncer_binary, config_path, database_path, tmp_path, syncer_helper.SyncerMode.FULL_SYNC
    )
    assert initial.returncode == 0, initial.stdout + initial.stderr

    # Edit active and inactive records, then remove the newest anime so the
    # partial scan must resume through the page containing it.
    with closing(sqlite3.connect(database_path)) as db:
        with db:
            db.execute("UPDATE anime SET title = 'Stale active anime' WHERE id = 2")
            db.execute("UPDATE mangas SET title = 'User-edited manga' WHERE id = 1")
            db.execute("DELETE FROM anime WHERE id = 51")

    result = syncer_helper.run_mode(
        syncer_binary, config_path, database_path, tmp_path, syncer_helper.SyncerMode.PARTIAL_SYNC
    )
    assert result.returncode == 0, result.stdout + result.stderr

    syncer_helper.assert_database_populated(database_path)
    syncer_helper.assert_entity_present(database_path, "anime", 2, title="Mock Anime Two")
    syncer_helper.assert_entity_present(database_path, "anime", 51, title="Mock Anime 51")
    syncer_helper.assert_entity_present(database_path, "mangas", 1, title="User-edited manga")
    syncer_helper.assert_entity_present(database_path, "mangas", 51, title="Mock Manga 51")


@pytest.mark.system
# OFT-REQ-FR-02: full sync creates the required database and replaces existing output.
def test003_syncer_full_sync_plan(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    """
    Test the full sync workflow and database replacement behaviour.

    Preconditions:
    - The syncer binary exists and is executable.
    - A valid configuration file is provided.
    - A valid destination database path is available.

    Test steps:
    1. Run the syncer in full-sync mode.
    2. Confirm a fresh database is created when none exists.
    3. Confirm an existing database is replaced on success.

    Expected results:
    - The full-sync database contains all required tables.
    - The previous database is replaced only after a successful publish.
    """
    result = syncer_helper.run_mode(
        syncer_binary, config_path, database_path, tmp_path, syncer_helper.SyncerMode.FULL_SYNC
    )
    assert result.returncode == 0, result.stdout + result.stderr

    syncer_helper.assert_database_populated(database_path)
    syncer_helper.assert_entity_present(database_path, "anime", 1, title="Mock Anime One")
    syncer_helper.assert_entity_present(database_path, "mangas", 1, title="Mock Manga One")

    with closing(sqlite3.connect(database_path)) as db:
        with db:
            db.execute("CREATE TABLE stale_marker (value TEXT)")
            db.execute("INSERT INTO stale_marker VALUES ('old database')")

    replacement = syncer_helper.run_mode(
        syncer_binary, config_path, database_path, tmp_path, syncer_helper.SyncerMode.FULL_SYNC
    )
    assert replacement.returncode == 0, replacement.stdout + replacement.stderr

    syncer_helper.assert_database_populated(database_path)
    assert "stale_marker" not in syncer_helper.table_names(database_path)
    syncer_helper.assert_entity_present(database_path, "anime", 51, title="Mock Anime 51")


@pytest.mark.system
# OFT-REQ-FR-03: retryable fetch failures are retried and reported as skipped entries.
def test004_syncer_retry_and_skip_plan(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    """
    Test retry behaviour and skipped-entry handling during network failures.

    Preconditions:
    - The syncer binary exists and is executable.
    - A valid configuration file is provided.
    - The Jikan API is configured to return retryable failures for selected requests.

    Test steps:
    1. Run the syncer and trigger retryable failure responses.
    2. Confirm retries occur up to the configured maximum.
    3. Confirm failed entries are logged and skipped instead of aborting the sync.

    Expected results:
    - Retry logic is exercised.
    - The run completes with a skipped-entry report instead of a hard failure.
    """
    initial = syncer_helper.run_mode(
        syncer_binary, config_path, database_path, tmp_path, syncer_helper.SyncerMode.FULL_SYNC
    )
    assert initial.returncode == 0, initial.stdout + initial.stderr

    with closing(sqlite3.connect(database_path)) as db:
        with db:
            db.execute("UPDATE anime SET title = 'Stale active anime' WHERE id = 2")

    JikanHandler.set_failures(("anime", "2"), [500, 500, 500])
    try:
        result = syncer_helper.run_mode(
            syncer_binary, config_path, database_path, tmp_path,
            syncer_helper.SyncerMode.PARTIAL_SYNC,
        )
    finally:
        JikanHandler.clear_failures(("anime", "2"))

    assert result.returncode == 0, result.stdout + result.stderr
    assert result.stdout.count("[Scraper Retry]") == 2
    syncer_helper.assert_entity_present(
        database_path, "anime", 2, title="Stale active anime"
    )
    report = syncer_helper.latest_report(tmp_path)
    assert report["sync_mode"] == "partial"
    assert report["skipped_count"] >= 1
    assert any(entry["mal_id"] == 2 for entry in report["skipped_entries"])


@pytest.mark.system
# OFT-REQ-FR-05: each generated database has a versioned integrity manifest.
def test005_syncer_manifest_plan(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    """
    Test manifest generation and integrity metadata.

    Preconditions:
    - The syncer binary exists and is executable.
    - A valid configuration file is provided.

    Test steps:
    1. Run the syncer and generate a database.
    2. Verify that a manifest file is created beside the database.
    3. Check that the version, checksum, and size fields are present.

    Expected results:
    - The manifest is published alongside the database.
    - The checksum matches the generated database bytes.
    - The database size is recorded in the manifest.
    """
    result = syncer_helper.run_mode(
        syncer_binary, config_path, database_path, tmp_path, syncer_helper.SyncerMode.FULL_SYNC
    )
    assert result.returncode == 0, result.stdout + result.stderr

    syncer_helper.assert_database_populated(database_path)
    manifest_path = Path(f"{database_path}.manifest.json")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    database_bytes = database_path.read_bytes()

    assert manifest["version"]
    assert manifest["schema_version"] == 1
    assert manifest["checksum"] == hashlib.sha256(database_bytes).hexdigest()
    assert manifest["size"] == len(database_bytes) == database_path.stat().st_size


@pytest.mark.system
# OFT-REQ-FR-04: CLI configuration and sync-mode selection are validated.
def test006_syncer_config_and_mode_plan(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    """
    Test CLI config handling and sync-mode selection.

    Preconditions:
    - The syncer binary exists and is executable.
    - A valid configuration file is provided.

    Test steps:
    1. Run with a valid config path and a selected mode.
    2. Confirm that DATABASE_PATH is applied from the environment.
    3. Check that invalid or missing config values are rejected cleanly.

    Expected results:
    - The selected sync mode is accepted by the CLI.
    - The database path is used as expected.
    - Invalid configuration is reported clearly.
    """
    valid = syncer_helper.run_mode(
        syncer_binary, config_path, database_path, tmp_path, syncer_helper.SyncerMode.FULL_SYNC
    )
    assert valid.returncode == 0, valid.stdout + valid.stderr
    syncer_helper.assert_database_file_exists(database_path)

    unknown_mode = syncer_helper.run_syncer(
        syncer_binary,
        "--mode",
        "invalid",
        config_path=config_path,
        database_path=tmp_path / "unused.db",
        working_directory=tmp_path,
    )
    assert unknown_mode.returncode == 0, unknown_mode.stdout + unknown_mode.stderr
    assert "Unknown execution mode: invalid" in unknown_mode.stdout

    invalid_config = tmp_path / "invalid.ini"
    invalid_config.write_text("[Import]\nimage_type=png\n", encoding="utf-8")
    invalid = syncer_helper.run_syncer(
        syncer_binary,
        "--mode",
        syncer_helper.SyncerMode.FULL_SYNC.value,
        config_path=invalid_config,
        database_path=tmp_path / "invalid.db",
        working_directory=tmp_path,
    )
    assert "invalid configuration" in invalid.stdout
    assert "image_type" in invalid.stdout
    assert not (tmp_path / "invalid.db").exists()


@pytest.mark.system
# OFT-REQ-NFR-01: the published database supports runtime edits.
# OFT-REQ-NFR-02: failed publication leaves the previous database and manifest intact.
def test007_syncer_database_integrity_plan(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    """
    Test SQLite database integrity and atomic publish behaviour.

    Preconditions:
    - The syncer binary exists and is executable.
    - A valid configuration file is provided.

    Test steps:
    1. Run the syncer to generate a database.
    2. Confirm the output database is writable and supports runtime edits.
    3. Confirm the destination is swapped into place only after success.

    Expected results:
    - The database is writable and queryable.
    - The final file is published atomically.
    - Runtime edits remain valid after publication.
    """
    initial = syncer_helper.run_mode(
        syncer_binary, config_path, database_path, tmp_path, syncer_helper.SyncerMode.FULL_SYNC
    )
    assert initial.returncode == 0, initial.stdout + initial.stderr
    syncer_helper.assert_database_populated(database_path)

    original_database = database_path.read_bytes()
    original_manifest = Path(f"{database_path}.manifest.json").read_bytes()

    config_path.write_text(
        config_path.read_text(encoding="utf-8")
        .replace("max_attempts=3", "max_attempts=1")
        .replace("sleep_amount=500ms", "sleep_amount=0s"),
        encoding="utf-8",
    )
    JikanHandler.set_failures(("anime",), [500])
    try:
        failed = syncer_helper.run_mode(
            syncer_binary, config_path, database_path, tmp_path,
            syncer_helper.SyncerMode.FULL_SYNC,
        )
    finally:
        JikanHandler.clear_failures(("anime",))

    assert failed.returncode != 0
    assert "Full sync failed" in failed.stdout
    assert database_path.read_bytes() == original_database
    assert Path(f"{database_path}.manifest.json").read_bytes() == original_manifest

    with closing(sqlite3.connect(database_path)) as db:
        with db:
            db.execute("CREATE TABLE runtime_edits (value TEXT)")
            db.execute("INSERT INTO runtime_edits VALUES ('editable')")
            assert db.execute("PRAGMA integrity_check").fetchone()[0] == "ok"
            assert db.execute("SELECT value FROM runtime_edits").fetchone()[0] == "editable"
