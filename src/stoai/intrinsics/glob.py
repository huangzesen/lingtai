"""Glob intrinsic — find files by pattern."""
from __future__ import annotations
from pathlib import Path

SCHEMA = {
    "type": "object",
    "properties": {
        "pattern": {"type": "string", "description": "Glob pattern (e.g., '**/*.py')"},
        "path": {"type": "string", "description": "Directory to search in"},
    },
    "required": ["pattern"],
}
DESCRIPTION = "Find files matching a glob pattern."

def handle_glob(args: dict) -> dict:
    pattern = args["pattern"]
    search_dir = Path(args.get("path", "."))
    if not search_dir.is_dir():
        return {"error": f"Not a directory: {search_dir}"}
    try:
        matches = sorted(str(p) for p in search_dir.glob(pattern) if p.is_file())
        return {"matches": matches, "count": len(matches)}
    except Exception as e:
        return {"error": f"Glob failed: {e}"}
