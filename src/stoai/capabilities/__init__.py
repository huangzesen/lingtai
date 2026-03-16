"""Composable agent capabilities — add via agent.add_capability("name")."""
from __future__ import annotations

import importlib
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent

# Registry of built-in capability names → module paths (relative to this package).
_BUILTIN: dict[str, str] = {
    "bash": ".bash",
    "delegate": ".delegate",
    "email": ".email",
    "draw": ".draw",
    "compose": ".compose",
    "talk": ".talk",
    "listen": ".listen",
    "vision": ".vision",
    "web_search": ".web_search",
}


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
            f"Available: {', '.join(sorted(_BUILTIN))}"
        )
    mod = importlib.import_module(module_path, package=__package__)
    setup_fn = getattr(mod, "setup", None)
    if setup_fn is None:
        raise ValueError(
            f"Capability module {name!r} does not export a setup() function"
        )
    return setup_fn(agent, **kwargs)
