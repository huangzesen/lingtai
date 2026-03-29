"""Resolve the Python venv for running lingtai agents.

Resolution order:
1. init.json → venv_path → test → use if working
2. ~/.lingtai/runtime/venv/ → test → use if working
3. Neither → create ~/.lingtai/runtime/venv/ automatically
"""
from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path


_DEFAULT_RUNTIME_DIR = Path.home() / ".lingtai" / "runtime" / "venv"


def resolve_venv(init_data: dict | None = None) -> Path:
    """Return the path to a working venv directory.

    Tries init.json venv_path first, then ~/.lingtai/runtime/venv/.
    Auto-creates the global venv if nothing works.
    """
    # 1. init.json venv_path
    if init_data and init_data.get("venv_path"):
        venv = Path(init_data["venv_path"])
        if _test_venv(venv):
            return venv

    # 2. ~/.lingtai/runtime/venv/
    if _test_venv(_DEFAULT_RUNTIME_DIR):
        return _DEFAULT_RUNTIME_DIR

    # 3. Auto-create
    _create_venv(_DEFAULT_RUNTIME_DIR)
    return _DEFAULT_RUNTIME_DIR


def venv_python(venv_dir: Path) -> str:
    """Return the path to the Python executable inside a venv."""
    if sys.platform == "win32":
        return str(venv_dir / "Scripts" / "python.exe")
    return str(venv_dir / "bin" / "python")


def _test_venv(venv_dir: Path) -> bool:
    """Test that a venv exists and has lingtai importable."""
    python = venv_python(venv_dir)
    if not os.path.isfile(python):
        return False
    try:
        result = subprocess.run(
            [python, "-c", "import lingtai"],
            capture_output=True, timeout=10,
        )
        return result.returncode == 0
    except (OSError, subprocess.TimeoutExpired):
        return False


def _create_venv(venv_dir: Path) -> None:
    """Create a fresh venv and install lingtai into it."""
    venv_dir.parent.mkdir(parents=True, exist_ok=True)
    print(f"Creating lingtai runtime at {venv_dir} ...", file=sys.stderr)

    # Find a working Python 3.11+
    python = _find_python()
    if not python:
        raise RuntimeError(
            "Cannot create venv: Python 3.11+ not found. "
            "Install Python from python.org and try again."
        )

    # Create venv
    subprocess.run(
        [python, "-m", "venv", str(venv_dir)],
        check=True,
    )

    # Install lingtai
    pip = str(venv_dir / "bin" / "pip")
    if sys.platform == "win32":
        pip = str(venv_dir / "Scripts" / "pip.exe")

    print("Installing lingtai...", file=sys.stderr)
    subprocess.run(
        [pip, "install", "lingtai"],
        check=True,
    )
    print("Runtime ready.", file=sys.stderr)


def ensure_package(pip_name: str, import_name: str | None = None) -> None:
    """Install a package into the current Python environment if missing.

    Tries uv first (fast), falls back to pip.
    """
    import_name = import_name or pip_name
    try:
        __import__(import_name)
        return
    except ImportError:
        pass

    python = sys.executable
    import shutil
    uv = shutil.which("uv")
    if uv:
        subprocess.run(
            [uv, "pip", "install", pip_name, "-p", python],
            check=True, capture_output=True,
        )
    else:
        subprocess.run(
            [python, "-m", "pip", "install", pip_name],
            check=True, capture_output=True,
        )

    # Verify
    try:
        __import__(import_name)
    except ImportError:
        raise ImportError(
            f"Failed to auto-install {pip_name}. "
            f"Try manually: pip install {pip_name}"
        )


def _find_python() -> str | None:
    """Find a Python ≥ 3.11 on the system."""
    import shutil
    for name in ("python3", "python"):
        path = shutil.which(name)
        if path:
            try:
                result = subprocess.run(
                    [path, "-c",
                     "import sys; sys.exit(0 if sys.version_info >= (3, 11) else 1)"],
                    capture_output=True, timeout=5,
                )
                if result.returncode == 0:
                    return path
            except (OSError, subprocess.TimeoutExpired):
                continue
    return None
