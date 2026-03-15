"""Grep intrinsic — search file contents by regex."""
from __future__ import annotations
import re
from pathlib import Path

SCHEMA = {
    "type": "object",
    "properties": {
        "pattern": {"type": "string", "description": "Regex pattern to search for"},
        "path": {"type": "string", "description": "File or directory to search in"},
        "glob": {"type": "string", "description": "File glob filter (e.g., '*.py')", "default": "*"},
        "max_matches": {"type": "integer", "description": "Maximum matches to return", "default": 200},
    },
    "required": ["pattern"],
}
DESCRIPTION = "Search file contents for lines matching a regex pattern."

def handle_grep(args: dict) -> dict:
    pattern = args["pattern"]
    search_path = Path(args.get("path", "."))
    file_glob = args.get("glob", "*")
    max_matches = args.get("max_matches", 200)
    try:
        regex = re.compile(pattern)
    except re.error as e:
        return {"error": f"Invalid regex: {e}"}
    matches = []
    if search_path.is_file():
        files = [search_path]
    elif search_path.is_dir():
        files = sorted(search_path.rglob(file_glob))
    else:
        return {"error": f"Path not found: {search_path}"}
    for f in files:
        if not f.is_file():
            continue
        try:
            for i, line in enumerate(f.read_text(encoding="utf-8", errors="replace").splitlines(), 1):
                if regex.search(line):
                    matches.append({"file": str(f), "line": i, "text": line.rstrip()})
        except Exception:
            continue
        if len(matches) >= max_matches:
            break
    return {"matches": matches, "count": len(matches), "truncated": len(matches) >= max_matches}
