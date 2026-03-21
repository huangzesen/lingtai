"""Add-ons --- optional extensions that may depend on capabilities.

Add-ons are set up after capabilities. They use the same
setup(agent, **kwargs) interface but live separately to signal
they are optional and may have external dependencies.

Addon managers can implement start() and stop() methods for
lifecycle hooks --- called by Agent.start() and Agent.stop().

Usage:
    agent = Agent(
        capabilities=["email", "file"],
        addons={"imap": {"email_address": "...", "email_password": "..."}},
    )
"""
from __future__ import annotations

import importlib
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

_BUILTIN: dict[str, str] = {
    "imap": ".imap",
    "telegram": ".telegram",
}


def setup_addon(agent: "BaseAgent", name: str, **kwargs: Any) -> Any:
    """Look up an addon by name and call its setup(agent, **kwargs).

    Returns whatever the addon's setup function returns (typically a manager).
    Raises ValueError if the name is unknown.
    """
    module_path = _BUILTIN.get(name)
    if module_path is None:
        raise ValueError(
            f"Unknown addon: {name!r}. "
            f"Available: {', '.join(sorted(_BUILTIN))}"
        )
    mod = importlib.import_module(module_path, package=__package__)
    setup_fn = getattr(mod, "setup", None)
    if setup_fn is None:
        raise ValueError(
            f"Addon module {name!r} does not export a setup() function"
        )
    return setup_fn(agent, **kwargs)
