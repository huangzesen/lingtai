"""Package-internal logging for stoai."""
from __future__ import annotations

import logging
from pathlib import Path

_logger: logging.Logger | None = None

def setup_logging(
    verbose: bool = False,
    log_dir: Path | str | None = None,
    logger_name: str = "stoai",
) -> logging.Logger:
    """Initialize the package logger.

    Args:
        verbose: If True, set console to DEBUG; otherwise INFO.
        log_dir: Directory for log files. None = no file logging.
        logger_name: Logger name (default: "stoai").
    """
    global _logger
    logger = logging.getLogger(logger_name)
    logger.setLevel(logging.DEBUG)

    if not logger.handlers:
        ch = logging.StreamHandler()
        ch.setLevel(logging.DEBUG if verbose else logging.INFO)
        fmt = logging.Formatter("%(asctime)s %(levelname)s %(name)s: %(message)s",
                                datefmt="%H:%M:%S")
        ch.setFormatter(fmt)
        logger.addHandler(ch)

    if log_dir is not None:
        log_path = Path(log_dir)
        log_path.mkdir(parents=True, exist_ok=True)
        fh = logging.FileHandler(log_path / "agent.log")
        fh.setLevel(logging.DEBUG)
        fmt = logging.Formatter("%(asctime)s %(levelname)s %(name)s: %(message)s")
        fh.setFormatter(fmt)
        logger.addHandler(fh)

    _logger = logger
    return logger


def get_logger() -> logging.Logger:
    """Get the package logger. Creates a default if setup_logging() was not called."""
    global _logger
    if _logger is None:
        _logger = setup_logging()
    return _logger
