import json
import hashlib
import os
import sys
import argparse

def calculate_sha256(filepath):
    """Calculate the SHA-256 checksum of a file in 4KB chunks."""
    sha256_hash = hashlib.sha256()
    try:
        with open(filepath, "rb") as f:
            for byte_block in iter(lambda: f.read(4096), b""):
                sha256_hash.update(byte_block)
        return sha256_hash.hexdigest()
    except FileNotFoundError:
        print(f"Error: Database file '{filepath}' not found.")
        sys.exit(1)

def validate_manifest(db_path, manifest_path):
    if not os.path.exists(manifest_path):
        print(f"Error: Manifest file '{manifest_path}' not found.")
        sys.exit(1)
        
    try:
        with open(manifest_path, 'r') as f:
            manifest = json.load(f)
    except json.JSONDecodeError:
        print(f"Error: '{manifest_path}' is not valid JSON.")
        sys.exit(1)
            
    expected_checksum = manifest.get('checksum')
    expected_size = manifest.get('size')
    
    if not expected_checksum or expected_size is None:
        print("Manifest is missing 'checksum' or 'size' fields.")
        sys.exit(1)

    print(f"Validating: {db_path}")
    print(f"Manifest Version: {manifest.get('version', 'unknown')}")
    
    # Check size
    actual_size = os.path.getsize(db_path)
    if actual_size != expected_size:
        print(f"SIZE MISMATCH")
        print(f"\tExpected: {expected_size} bytes")
        print(f"\tActual:   {actual_size} bytes")
        sys.exit(1)
    
    print(f"Size matches: {actual_size} bytes")

    # 2Check SHA-256
    print("Calculating SHA-256")
    actual_checksum = calculate_sha256(db_path)
    
    if actual_checksum == expected_checksum:
        print(f"Checksum matches: {actual_checksum}")
        print("Validation Successful")
    else:
        print(f"CHECKSUM MISMATCH")
        print(f"\tExpected: {expected_checksum}")
        print(f"\tActual:   {actual_checksum}")
        sys.exit(1)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Validate published anime database.")
    parser.add_argument("db_path", help="Path to the published anime.db file")
    
    args = parser.parse_args()
    
    manifest_path = f"{args.db_path}.manifest.json"    
    validate_manifest(args.db_path, manifest_path)