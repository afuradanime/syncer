# DISCLAIMER: Old script copied from old commit

import argparse;
from pathlib import Path;
import time;

BACKUP_PATH = "./_backups";

def restore_database(restore_path: Path):

    # Get lastest backup file
    backup_folder = Path(BACKUP_PATH);
    if not backup_folder.exists():
        print("No backup folder found. Cannot restore database.");
        return;

    backup_files = sorted(backup_folder.glob("*.backup"), key=lambda f: f.stat().st_mtime, reverse=True);
    if not backup_files:
        print("No backup files found. Cannot restore database.");
        return;

    backup_file = backup_files[0];

    try:
        with backup_file.open("rb") as src, restore_path.open("wb") as dst:
            dst.write(src.read());
        print(f"Database restored successfully from {backup_file} to {restore_path}");

        # Delete backup file after restore
        backup_file.unlink();

    except Exception as e:
        print(f"Error restoring database: {e}");

def delete_old_backups(keep_count: int = 1):

    backup_folder = Path(BACKUP_PATH);
    if not backup_folder.exists():
        print("No backup folder found. Skipping old backup deletion.");
        return;

    backup_files = sorted(backup_folder.glob("*.backup"), key=lambda f: f.stat().st_mtime, reverse=True);
    if len(backup_files) <= keep_count:
        print("No old backups to delete.");
        return;

    for old_backup in backup_files[keep_count:]:
        try:
            old_backup.unlink();
            print(f"Deleted old backup: {old_backup}");
        except Exception as e:
            print(f"Error deleting old backup {old_backup}: {e}");

def backup_local(db_path: Path):

    date_string = time.strftime("%Y%m%d-%H%M%S");
    backup_path = Path(BACKUP_PATH) / f"{db_path.stem}_{date_string}.backup";
    try:
        with db_path.open("rb") as src, backup_path.open("wb") as dst:
            dst.write(src.read());
        print(f"Database backed up successfully to {backup_path}");
    except Exception as e:
        print(f"Error backing up database: {e}");

def main():
    
    parser = argparse.ArgumentParser(
        description="Backup database"
    );

    parser.add_argument(
        "--db-path",
        type=Path,
        required=True,
        help="Path to database file"
    );

    parser.add_argument(
        "--restore",
        action="store_true",
        help="Restore database instead of backing up"
    );

    parser.add_argument(
        "--keep-count",
        type=int,
        default=0,
        help="Number of old backups to keep (local backup only)"
    )

    parser.add_argument(
        "--backup_path",
        type=Path,
        default=Path(BACKUP_PATH),
        help="Path to backup folder (local backup only)"
    );

    args = parser.parse_args();

    if args.restore:
        restore_database(args.db_path);
        return;
    
    if not args.db_path.exists():
        print(f"Database file not found at {args.db_path}. Please check the path and try again.");
        return;

    backup_local(args.db_path);

    if args.keep_count > 0:
        delete_old_backups(args.keep_count);

if __name__ == "__main__":
    main()