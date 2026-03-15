"""Edit intrinsic — exact string replacement in a file."""
from __future__ import annotations
from pathlib import Path

SCHEMA = {
    "type": "object",
    "properties": {
        "file_path": {"type": "string", "description": "Absolute path to the file to edit"},
        "old_string": {"type": "string", "description": "The exact text to find and replace"},
        "new_string": {"type": "string", "description": "The replacement text"},
        "replace_all": {"type": "boolean", "description": "Replace all occurrences", "default": False},
    },
    "required": ["file_path", "old_string", "new_string"],
}
DESCRIPTION = "Replace an exact string in a file. Fails if old_string is not found or is ambiguous."

def handle_edit(args: dict) -> dict:
    path = Path(args["file_path"])
    if not path.exists():
        return {"error": f"File not found: {path}"}
    try:
        content = path.read_text(encoding="utf-8")
    except Exception as e:
        return {"error": f"Cannot read {path}: {e}"}
    old = args["old_string"]
    new = args["new_string"]
    replace_all = args.get("replace_all", False)
    count = content.count(old)
    if count == 0:
        return {"error": f"old_string not found in {path}"}
    if count > 1 and not replace_all:
        return {"error": f"old_string found {count} times — use replace_all=true or provide more context"}
    if replace_all:
        updated = content.replace(old, new)
    else:
        updated = content.replace(old, new, 1)
    path.write_text(updated, encoding="utf-8")
    return {"status": "ok", "replacements": count if replace_all else 1}
