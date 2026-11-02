"""System-test setup smoke check; no catalogue synchronization is performed."""

from pathlib import Path

import pytest

import syncer_helper


@pytest.mark.system
def test_setup_works(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    result = syncer_helper.run_syncer(
        syncer_binary,
        "--help",
        config_path=config_path,
        database_path=database_path,
        working_directory=tmp_path,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert "-mode" in result.stdout + result.stderr
    assert not database_path.exists()