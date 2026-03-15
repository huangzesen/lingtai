"""Write intrinsic — create or overwrite a file."""
from __future__ import annotations
from pathlib import Path

SCHEMA = {
    "type": "object",
    "properties": {
        "file_path": {"type": "string", "description": "Absolute path to the file to write"},
        "content": {"type": "string", "description": "Content to write"},
    },
    "required": ["file_path", "content"],
}
DESCRIPTION = "Create or overwrite a file with the given content."

def handle_write(args: dict) -> dict:
    path = Path(args["file_path"])
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(args["content"], encoding="utf-8")
        return {"status": "ok", "path": str(path), "bytes": len(args["content"])}
    except Exception as e:
        return {"error": f"Cannot write {path}: {e}"}
