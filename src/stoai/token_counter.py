"""Token counting with provider-agnostic fallback chain.

Priority: google.genai LocalTokenizer → tiktoken → len(text) // 4
"""
from __future__ import annotations

import warnings
from .logging import get_logger

_tokenizer = None
_backend: str = "none"

def _init_tokenizer() -> None:
    global _tokenizer, _backend
    logger = get_logger()

    # Try google-genai first
    try:
        from google.genai._common import ExperimentalWarning
        with warnings.catch_warnings():
            warnings.filterwarnings("ignore", category=ExperimentalWarning)
            from google.genai.local_tokenizer import LocalTokenizer
        _tokenizer = LocalTokenizer()
        _backend = "gemini"
        logger.debug("token_counter: using google-genai LocalTokenizer")
        return
    except (ImportError, Exception):
        pass

    # Try tiktoken
    try:
        import tiktoken
        _tokenizer = tiktoken.get_encoding("cl100k_base")
        _backend = "tiktoken"
        logger.debug("token_counter: using tiktoken cl100k_base")
        return
    except (ImportError, Exception):
        pass

    # Final fallback: character estimate
    _backend = "char_estimate"
    logger.debug("token_counter: using character estimate (len // 4)")


def count_tokens(text: str) -> int:
    """Count tokens in text using the best available tokenizer."""
    if not text:
        return 0
    global _tokenizer, _backend
    if _backend == "none":
        _init_tokenizer()

    if _backend == "gemini":
        return _tokenizer.count_tokens(text).total_tokens
    elif _backend == "tiktoken":
        return len(_tokenizer.encode(text))
    else:
        return len(text) // 4


def count_tool_tokens(schemas: list[dict]) -> int:
    """Estimate tokens consumed by tool schemas."""
    import json
    text = json.dumps(schemas)
    return count_tokens(text)
