#!/usr/bin/env python3
from __future__ import annotations

from pathlib import Path
import sys


TEXT_EXTENSIONS = {
    ".css",
    ".go",
    ".html",
    ".js",
    ".json",
    ".md",
    ".sql",
    ".txt",
    ".yml",
    ".yaml",
}

SKIP_DIRS = {
    ".git",
}


def should_check(path: Path) -> bool:
    if path.suffix.lower() in TEXT_EXTENSIONS:
        return True
    if path.name in {"Dockerfile", ".editorconfig", ".gitignore", ".dockerignore"}:
        return True
    return False


def main() -> int:
    root = Path(".").resolve()
    invalid: list[Path] = []

    for path in root.rglob("*"):
        if not path.is_file():
            continue
        if any(part in SKIP_DIRS for part in path.parts):
            continue
        rel = path.relative_to(root)
        if not should_check(rel):
            continue
        try:
            path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            invalid.append(rel)

    if invalid:
        print("Files that are not valid UTF-8:")
        for item in invalid:
            print(f" - {item.as_posix()}")
        return 1

    print("UTF-8 check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
