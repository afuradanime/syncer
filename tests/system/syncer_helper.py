import os
import subprocess
from pathlib import Path


def run_syncer(
	binary: Path,
	*args: str,
	config_path: Path,
	database_path: Path,
	working_directory: Path,
	timeout: float = 30,
) -> subprocess.CompletedProcess[str]:
	return subprocess.run(
		[str(binary), "--config", str(config_path), *args],
		cwd=working_directory,
		env={**os.environ, "DATABASE_PATH": str(database_path)},
		capture_output=True,
		text=True,
		timeout=timeout,
		check=False,
	)
