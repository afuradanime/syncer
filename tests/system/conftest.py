import os
import shutil
import sys
from http.server import ThreadingHTTPServer
from pathlib import Path
from threading import Thread

import pytest

PROJECT_ROOT = Path(__file__).resolve().parents[2]
TEST_ROOT = Path(__file__).resolve().parents[0]

sys.path.insert(0, str(PROJECT_ROOT))

from tools.jikan_mock.jikan_mock import JikanHandler

@pytest.fixture(scope="session")
def jikan_mock():
    with ThreadingHTTPServer(("127.0.0.1", 0), JikanHandler) as server:
        thread = Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            yield f"http://127.0.0.1:{server.server_port}/v4"
        finally:
            server.shutdown()
            thread.join()

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
    with path.open("a", encoding="utf-8") as config:
        config.write(
            "\n\n[Network]\nrequest_timeout=1s\nmax_attempts=3\n"
            "base_backoff=1ms\nsleep_amount=0s\n"
        )
    return path


@pytest.fixture
def database_path(tmp_path: Path) -> Path:
    return tmp_path / "syncer.db"

@pytest.fixture(autouse=True)
def set_env_vars(monkeypatch, jikan_mock):
    monkeypatch.setenv("TENRAI_ENDPOINT", jikan_mock)