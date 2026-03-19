"""Simple single agent for testing — no covenant, all capabilities.

A bare assistant agent with full tool access. Good for testing
individual intrinsics and capabilities interactively.
"""
from __future__ import annotations

from pathlib import Path

from stoai import AgentConfig
from stoai.llm import LLMService

from ..server.state import AppState

USER_PORT = 8300


def setup(llm: LLMService, base_dir: Path) -> AppState:
    """Create a simple test agent."""
    state = AppState(base_dir=base_dir, user_port=USER_PORT)

    state.register_agent(
        key="a",
        agent_name="assistant",
        name="Assistant",
        port=8301,
        llm=llm,
        capabilities={
            "file": {},
            "bash": {"yolo": True},
            "email": {},
            "vision": {},
            "web_search": {},
            "psyche": {},
            "delegate": {},
        },
        config=AgentConfig(max_turns=100, flow_delay=5.0, language="zh"),
    )

    return state
