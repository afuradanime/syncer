import os
import shutil
from pathlib import Path

import pytest

PROJECT_ROOT = Path(__file__).resolve().parents[2]
TEST_ROOT = Path(__file__).resolve().parents[0]

@pytest.fixture(scope="session")
def syncer_binary() -> Path:
    binary = Path(
        os.environ.get("SYNCER_BINARY", PROJECT_ROOT / "bin" / "anime-syncer")
    ).resolve()
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise pytest.UsageError(
            f"Syncer binary is missing or not executable: {binary}. "
            "Run 'make build' or set SYNCER_BINARY."
        )
    return binary


@pytest.fixture
def config_path(tmp_path: Path) -> Path:
    path = tmp_path / "config.ini"
    shutil.copyfile(TEST_ROOT / "testfiles" / "test.ini", path)
    return path


@pytest.fixture
def database_path(tmp_path: Path) -> Path:
    return tmp_path / "syncer.db"