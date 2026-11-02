from pathlib import Path

import pytest

import syncer_helper

@pytest.mark.system
def test001_syncer_build(
    syncer_binary: Path, config_path: Path, database_path: Path, tmp_path: Path
):
    """
    Test that the syncer binary was built and is executable

    Preconditions:
    - The syncer binary exists and is executable
    - A valid configuration file is provided

    Test steps:
    1. Run the syncer binary with the --help flag

    Expected results:
    - The syncer binary runs successfully and displays the help message
    - The output contains the -mode option
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