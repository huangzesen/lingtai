"""Read intrinsic — read text file contents."""
from __future__ import annotations
from pathlib import Path

SCHEMA = {
    "type": "object",
    "properties": {
        "file_path": {"type": "string", "description": "Absolute path to the file to read"},
        "offset": {"type": "integer", "description": "Line number to start from (1-based)", "default": 1},
        "limit": {"type": "integer", "description": "Max lines to read", "default": 2000},
    },
    "required": ["file_path"],
}
DESCRIPTION = "Read the contents of a text file."

def handle_read(args: dict) -> dict:
    path = Path(args["file_path"])
    if not path.exists():
        return {"error": f"File not found: {path}"}
    if not path.is_file():
        return {"error": f"Not a file: {path}"}
    offset = args.get("offset", 1)
    limit = args.get("limit", 2000)
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            lines = f.readlines()
    except Exception as e:
        return {"error": f"Cannot read {path}: {e}"}
    start = max(0, offset - 1)
    selected = lines[start:start + limit]
    numbered = "".join(f"{start + i + 1}\t{line}" for i, line in enumerate(selected))
    return {"content": numbered, "total_lines": len(lines), "lines_shown": len(selected)}
