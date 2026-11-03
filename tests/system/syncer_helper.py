from collections.abc import Iterable, Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from enum import Enum
import os
import subprocess
from pathlib import Path
import sqlite3
import json

from tools.validate_database import validate_manifest

class SyncerMode(Enum):
	FULL_SYNC = "full"
	PARTIAL_SYNC = "partial"

def run_mode(
    syncer_binary: Path,
    config_path: Path,
    database_path: Path,
    working_directory: Path,
    mode: SyncerMode,
):
    return run_syncer(
        syncer_binary,
        "--mode",
        mode.value,
        config_path=config_path,
        database_path=database_path,
        working_directory=working_directory,
    )

def run_syncer(
	binary: Path,
	*args: str,
	config_path: Path,
	database_path: Path,
	working_directory: Path,
	timeout: float = 30,
) -> subprocess.CompletedProcess[str]:
	result = subprocess.run(
		[str(binary), "--config", str(config_path), *args],
		cwd=working_directory,
		env={**os.environ, "DATABASE_PATH": str(database_path)},
		capture_output=True,
		text=True,
		timeout=timeout,
		check=False,
	)

	print("STDOUT:\n" + result.stdout)
	print("STDERR:\n" + result.stderr)

	return result

CORE_TABLES = frozenset({
    "anime",
    "anime_type",
    "anime_status",
    "tags",
    "anime_tags",
    "manga",
    "manga_relations",
})

@contextmanager
def open_db(path: Path) -> Iterator[sqlite3.Connection]:
    """Open the published database read-only and close it on exit.

    Opens the file in read-only mode so a test that accidentally writes
    doesn't corrupt the artifact under test.
    """
    uri = f"file:{path}?mode=ro"
    conn = sqlite3.connect(uri, uri=True)
    try:
        conn.row_factory = sqlite3.Row
        yield conn
    finally:
        conn.close()

def assert_database_file_exists(path: Path) -> None:
    assert path.exists(), f"database not written to {path}"
    assert path.is_file(), f"database path {path} is not a regular file"
    assert path.stat().st_size > 0, f"database {path} is zero bytes"


def assert_sqlite_integrity(path: Path) -> None:
    """``PRAGMA integrity_check`` must return ``ok``."""
    with open_db(path) as conn:
        result = conn.execute("PRAGMA integrity_check").fetchone()[0]
    assert result == "ok", f"integrity_check failed for {path}: {result}"


def table_names(path: Path) -> set[str]:
    with open_db(path) as conn:
        rows = conn.execute(
            "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"
        ).fetchall()
    return {row["name"] for row in rows}


def assert_has_tables(path: Path, expected: Iterable[str] = CORE_TABLES) -> None:
    missing = set(expected) - table_names(path)
    assert not missing, f"database {path} is missing tables: {sorted(missing)}"


def count_rows(path: Path, table: str) -> int:
    with open_db(path) as conn:
        return conn.execute(f"SELECT COUNT(*) FROM {table}").fetchone()[0]


def assert_table_not_empty(path: Path, table: str, *, minimum: int = 1) -> None:
    n = count_rows(path, table)
    assert n >= minimum, f"table {table!r} has {n} rows, want >= {minimum}"


def assert_table_empty(path: Path, table: str) -> None:
    n = count_rows(path, table)
    assert n == 0, f"table {table!r} has {n} rows, want 0"

@dataclass(frozen=True)
class Entity:
    """A minimal projection of a row used for spot-checks."""
    id: int
    title: str


def fetch_entity(path: Path, table: str, entity_id: int) -> Entity | None:
    with open_db(path) as conn:
        row = conn.execute(
            f"SELECT id, title FROM {table} WHERE id = ?", (entity_id,)
        ).fetchone()
    return Entity(id=row["id"], title=row["title"]) if row else None


def assert_entity_present(path: Path, table: str, entity_id: int, *, title: str | None = None) -> None:
    entity = fetch_entity(path, table, entity_id)
    assert entity is not None, f"{table} id={entity_id} missing from {path}"
    if title is not None:
        assert entity.title == title, (
            f"{table} id={entity_id} title = {entity.title!r}, want {title!r}"
        )

def assert_database_populated(
    path: Path,
    *,
    require_anime: bool = True,
    require_manga: bool = True,
    check_manifest: bool = True,
) -> None:
    """Assert the database is well-formed, has the full schema, and has rows."""
    assert_database_file_exists(path)
    assert_sqlite_integrity(path)
    assert_has_tables(path)

    if require_anime:
        assert_table_not_empty(path, "anime")
    if require_manga:
        assert_table_not_empty(path, "manga")

    if check_manifest:
        validate_manifest(str(path), f"{path}.manifest.json")

def latest_report(directory: Path) -> dict:
    reports = list(directory.glob("sync_report_*.json"))
    assert reports, f"no sync report was written in {directory}"
    report_path = max(reports, key=lambda path: path.stat().st_mtime_ns)
    return json.loads(report_path.read_text(encoding="utf-8"))