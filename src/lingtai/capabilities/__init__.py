"""Composable agent capabilities — add via Agent(capabilities=[...])."""
from __future__ import annotations

import importlib
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

# Registry of built-in capability names → module paths (relative to this package).
_BUILTIN: dict[str, str] = {
    "psyche": ".psyche",
    "library": ".library",
    "bash": ".bash",
    "avatar": ".avatar",
    "email": ".email",
    "draw": ".draw",
    "compose": ".compose",
    "talk": ".talk",
    "listen": ".listen",
    "vision": ".vision",
    "web_search": ".web_search",
    "web_read": ".web_read",
    "read": ".read",
    "write": ".write",
    "edit": ".edit",
    "glob": ".glob",
    "grep": ".grep",
}

# Group names that expand to multiple capabilities.
_GROUPS: dict[str, list[str]] = {
    "file": ["read", "write", "edit", "glob", "grep"],
}


def expand_groups(names: list[str]) -> list[str]:
    """Expand group names (e.g. 'file') into individual capability names."""
    result = []
    for name in names:
        if name in _GROUPS:
            result.extend(_GROUPS[name])
        else:
            result.append(name)
    return result


def setup_capability(agent: "BaseAgent", name: str, **kwargs: Any) -> Any:
    """Look up a capability by *name* and call its ``setup(agent, **kwargs)``.

    Returns whatever the capability's ``setup`` function returns (typically
    a manager instance).

    Raises ``ValueError`` if the name is unknown or the module lacks ``setup``.
    """
    module_path = _BUILTIN.get(name)
    if module_path is None:
        raise ValueError(
            f"Unknown capability: {name!r}. "
            f"Available: {', '.join(sorted(_BUILTIN))}. "
            f"Groups: {', '.join(sorted(_GROUPS))}"
        )
    mod = importlib.import_module(module_path, package=__package__)
    setup_fn = getattr(mod, "setup", None)
    if setup_fn is None:
        raise ValueError(
            f"Capability module {name!r} does not export a setup() function"
        )
    return setup_fn(agent, **kwargs)
