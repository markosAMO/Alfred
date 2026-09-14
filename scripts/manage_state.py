#!/usr/bin/env python3
"""Track which installed files Alfred manages, by content hash.

A file whose hash still matches the one recorded at install time was not touched by the
user and may be replaced. A file whose hash differs was modified and is reported instead
of overwritten.
"""
import hashlib
import json
import sys
from pathlib import Path


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


NOT_MANAGED = {"state.json", "profile.json"}


def managed_files(root: Path, payload: list[str] | None = None) -> list[Path]:
    """Files Alfred owns.

    Only the payload is installed: the repository also holds its own README, licence,
    installer and documentation, none of which belong in an installation. Without this
    filter an update copies the whole repository into the install directory.
    """
    roots = [root / item for item in payload] if payload else [root]
    found = []

    for entry in roots:
        if entry.is_file():
            found.append(entry)
        elif entry.is_dir():
            found.extend(p for p in entry.rglob("*") if p.is_file())

    return sorted(
        p for p in found
        if ".git" not in p.parts and p.name not in NOT_MANAGED
    )


def write(root: Path, version: str, payload: list[str]) -> int:
    files = managed_files(root, payload)
    state = {
        "version": version,
        "files": {str(p.relative_to(root)): sha256(p) for p in files},
    }
    (root / "state.json").write_text(json.dumps(state, indent=2) + "\n")
    return len(files)


def compare(root: Path, source: Path, payload: list[str]) -> dict[str, list[str]]:
    """Classify every source file against what was installed."""
    state_path = root / "state.json"
    recorded = json.loads(state_path.read_text())["files"] if state_path.exists() else {}

    result: dict[str, list[str]] = {"new": [], "unchanged": [], "modified": [], "updatable": []}

    for src in managed_files(source, payload):
        rel = str(src.relative_to(source))
        installed = root / rel

        if not installed.exists():
            result["new"].append(rel)
        elif rel in recorded and sha256(installed) != recorded[rel]:
            result["modified"].append(rel)
        elif sha256(installed) == sha256(src):
            result["unchanged"].append(rel)
        else:
            result["updatable"].append(rel)

    return result


def main() -> int:
    command = sys.argv[1]

    if command == "write":
        count = write(Path(sys.argv[2]), sys.argv[3], sys.argv[4].split(","))
        print(f"{count} files recorded")
        return 0

    if command == "compare":
        print(json.dumps(compare(Path(sys.argv[2]), Path(sys.argv[3]), sys.argv[4].split(","))))
        return 0

    print(f"unknown command: {command}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
