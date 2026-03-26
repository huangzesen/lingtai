"""Agent — BaseAgent + composable capabilities.

Layer 2 of the three-layer hierarchy:
    BaseAgent (kernel) → Agent (capabilities) → CustomAgent (domain)

Capabilities are declared at construction and sealed before start().
"""
from __future__ import annotations

from typing import Any

from pathlib import Path

from lingtai_kernel.base_agent import BaseAgent
from lingtai.llm.service import LLMService
from lingtai_kernel.prompt import build_system_prompt


class Agent(BaseAgent):
    """BaseAgent with composable capabilities.

    Args:
        capabilities: Capability names to enable. Either a list of strings
            (no kwargs) or a dict mapping names to kwargs dicts.
            Each capability dict may include ``"provider"`` to route that
            capability to a specific LLM provider (e.g. ``"gemini"``, ``"minimax"``).
            Group names (e.g. ``"file"``) expand to individual capabilities.
        *args, **kwargs: Passed through to BaseAgent.
    """

    def __init__(
        self,
        *args: Any,
        capabilities: list[str] | dict[str, dict] | None = None,
        addons: dict[str, dict] | None = None,
        combo_name: str | None = None,
        **kwargs: Any,
    ):
        # Default karma authority for the primary agent (本我)
        kwargs.setdefault("admin", {"karma": True})

        # Store combo name before super().__init__ (not forwarded to BaseAgent)
        self._combo_name = combo_name

        super().__init__(*args, **kwargs)

        # Persist LLM config for revive (self-sufficient agents contract)
        _service = args[0] if args else kwargs.get("service")
        if _service is not None:
            try:
                import json as _json
                llm_config: dict[str, Any] = {
                    "provider": _service.provider,
                    "model": _service.model,
                }
                _base_url = getattr(_service, "_base_url", None)
                if isinstance(_base_url, str) and _base_url:
                    llm_config["base_url"] = _base_url
                llm_dir = self._working_dir / "system"
                llm_dir.mkdir(exist_ok=True)
                (llm_dir / "llm.json").write_text(
                    _json.dumps(llm_config, ensure_ascii=False)
                )
            except (TypeError, AttributeError, OSError):
                pass  # LLM config not available (e.g., mock service in tests)

        # Auto-create FileIOService if not provided by host
        if self._file_io is None:
            from .services.file_io import LocalFileIOService
            self._file_io = LocalFileIOService(root=self._working_dir)

        # Auto-load MCP servers from working directory
        self._load_mcp_from_workdir()

        # Expand groups and normalize to dict
        if isinstance(capabilities, list):
            from .capabilities import expand_groups
            expanded = expand_groups(capabilities)
            capabilities = {name: {} for name in expanded}
        elif isinstance(capabilities, dict):
            from .capabilities import _GROUPS
            expanded_dict: dict[str, dict] = {}
            for name, cap_kwargs in capabilities.items():
                if name in _GROUPS:
                    for sub in _GROUPS[name]:
                        expanded_dict[sub] = {}
                else:
                    expanded_dict[name] = cap_kwargs
            capabilities = expanded_dict

        # Track for avatar replay
        self._capabilities: list[tuple[str, dict]] = []
        self._capability_managers: dict[str, Any] = {}

        # Register capabilities — provider kwarg flows through to setup() naturally
        if capabilities:
            for name, cap_kwargs in capabilities.items():
                self._setup_capability(name, **cap_kwargs)

        # Register addons (after capabilities, may depend on them)
        self._addon_managers: dict[str, Any] = {}
        if addons:
            from .addons import setup_addon
            for addon_name, addon_kwargs in addons.items():
                mgr = setup_addon(self, addon_name, **(addon_kwargs or {}))
                self._addon_managers[addon_name] = mgr

        # Re-write manifest now that capabilities are registered
        if self._capabilities:
            self._workdir.write_manifest(self._build_manifest())

    def _setup_capability(self, name: str, **kwargs: Any) -> Any:
        """Load a named capability.

        Not directly sealed — but setup() calls add_tool() which checks the seal.
        Must only be called from __init__ (before start()).
        """
        from .capabilities import setup_capability

        serializable_kw = {
            k: v for k, v in kwargs.items()
            if isinstance(v, (str, int, float, bool, type(None), list, dict))
        }
        self._capabilities.append((name, serializable_kw))
        mgr = setup_capability(self, name, **kwargs)
        self._capability_managers[name] = mgr
        return mgr

    _SENSITIVE_KEYS = {"api_key", "api_key_env", "api_secret", "token", "password"}

    def _build_manifest(self) -> dict:
        """Extend kernel manifest with capabilities and combo.

        Strips sensitive fields (api_key, etc.) from capability kwargs
        so they don't leak into the system prompt or outgoing mail identity.
        """
        data = super()._build_manifest()
        caps = getattr(self, "_capabilities", None)
        if caps:
            data["capabilities"] = [
                (name, {k: v for k, v in kw.items() if k not in self._SENSITIVE_KEYS})
                for name, kw in caps
            ]
        if self._combo_name:
            data["combo"] = self._combo_name
        return data

    def _build_system_prompt(self) -> str:
        """Override kernel's prompt builder to inject tool descriptions."""
        lang = self._config.language
        lines = []
        from lingtai_kernel.intrinsics import ALL_INTRINSICS
        for name in self._intrinsics:
            info = ALL_INTRINSICS.get(name)
            if info:
                lines.append(f"### {name}\n{info['module'].get_description(lang)}")
        for s in self._tool_schemas:
            if s.description:
                lines.append(f"### {s.name}\n{s.description}")
        if lines:
            self._prompt_manager.write_section(
                "tools", "\n\n".join(lines), protected=True
            )
        return build_system_prompt(
            prompt_manager=self._prompt_manager,
            language=lang,
        )

    def _load_mcp_from_workdir(self) -> None:
        """Auto-load MCP servers declared in working_dir/mcp/servers.json.

        Format:
            {
              "server-name": {
                "command": "xhelio-spice-mcp",
                "args": [],
                "env": {}
              }
            }

        The file is created by setup agents or manually. Each server's
        tools are auto-registered via connect_mcp().
        """
        import json

        mcp_config = self._working_dir / "mcp" / "servers.json"
        if not mcp_config.is_file():
            return

        try:
            servers = json.loads(mcp_config.read_text())
        except (json.JSONDecodeError, OSError):
            return

        if not isinstance(servers, dict):
            return

        from lingtai_kernel.logging import get_logger
        logger = get_logger()

        for name, cfg in servers.items():
            if not isinstance(cfg, dict) or "command" not in cfg:
                continue
            try:
                tools = self.connect_mcp(
                    command=cfg["command"],
                    args=cfg.get("args"),
                    env=cfg.get("env"),
                )
                logger.info("[%s] MCP %s: loaded %d tools (%s)",
                            self.agent_name, name, len(tools), ", ".join(tools))
            except Exception as e:
                logger.warning("[%s] MCP %s: failed to load: %s",
                               self.agent_name, name, e)

    def _cpr_agent(self, address: str) -> "Agent | None":
        """Resuscitate a suspended agent from its working dir."""
        import json
        from lingtai_kernel.handshake import is_agent, manifest
        from lingtai_kernel.config import AgentConfig

        target = Path(address)
        if not is_agent(target):
            return None

        agent_meta = manifest(target)

        # Resolve LLM config from combo or llm.json
        combo_name = agent_meta.get("combo")
        llm_config = None

        if combo_name:
            combo_path = Path.home() / ".lingtai" / "combos" / f"{combo_name}.json"
            if not combo_path.is_file():
                combo_path = target / "combo.json"
            if combo_path.is_file():
                combo_data = json.loads(combo_path.read_text())
                model_cfg = combo_data.get("model", {})
                llm_config = {
                    "provider": model_cfg.get("provider"),
                    "model": model_cfg.get("model"),
                    "base_url": model_cfg.get("base_url"),
                }
                import os
                for key, val in combo_data.get("env", {}).items():
                    if val:
                        os.environ.setdefault(key, val)

        if llm_config is None:
            llm_path = target / "system" / "llm.json"
            if not llm_path.is_file():
                return None
            llm_config = json.loads(llm_path.read_text())

        svc = LLMService(
            provider=llm_config["provider"],
            model=llm_config["model"],
            base_url=llm_config.get("base_url"),
        )

        caps_raw = agent_meta.get("capabilities")
        capabilities = None
        if caps_raw:
            capabilities = {name: kw for name, kw in caps_raw}

        revived_config = AgentConfig(
            provider=llm_config["provider"],
            model=llm_config["model"],
            stamina=agent_meta.get("stamina", 3600.0),
            soul_delay=agent_meta.get("soul_delay", 120.0),
            language=agent_meta.get("language", "en"),
        )

        revived = Agent(
            svc,
            agent_name=agent_meta.get("agent_name"),
            working_dir=target,
            capabilities=capabilities,
            admin=agent_meta.get("admin", {}),
            config=revived_config,
            combo_name=combo_name,
        )
        revived._molt_count = agent_meta.get("molt_count", 0)
        revived.start()
        return revived

    def start(self) -> None:
        super().start()
        for name, mgr in self._addon_managers.items():
            if hasattr(mgr, "start"):
                mgr.start()

    def connect_mcp(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
    ) -> list[str]:
        """Connect to an MCP server and auto-register all its tools.

        Args:
            command: Executable to run (e.g., "uvx", "xhelio-spice-mcp").
            args: Arguments to the command.
            env: Environment variables for the subprocess.

        Returns:
            List of registered tool names.
        """
        from .services.mcp import MCPClient

        client = MCPClient(command=command, args=args, env=env)
        client.start()

        # Track for cleanup
        if not hasattr(self, "_mcp_clients"):
            self._mcp_clients: list = []
        self._mcp_clients.append(client)

        # List tools and register each one
        tools = client.list_tools()
        registered = []
        for tool in tools:
            name = tool["name"]

            def _make_handler(c: MCPClient, tool_name: str):
                def handler(tool_args: dict) -> dict:
                    return c.call_tool(tool_name, tool_args)
                return handler

            # Extract schema properties (MCP uses inputSchema with JSON Schema)
            schema = tool.get("schema", {})
            # Remove top-level keys that aren't valid for our FunctionSchema
            schema.pop("additionalProperties", None)

            self.add_tool(
                name,
                schema=schema,
                handler=_make_handler(client, name),
                description=tool.get("description", ""),
            )
            registered.append(name)

        return registered

    def stop(self, timeout: float = 5.0) -> None:
        # Close MCP clients
        for client in getattr(self, "_mcp_clients", []):
            try:
                client.close()
            except Exception:
                pass

        for name, mgr in self._addon_managers.items():
            if hasattr(mgr, "stop"):
                try:
                    mgr.stop()
                except Exception:
                    pass
        super().stop(timeout=timeout)

    def get_capability(self, name: str) -> Any:
        """Return the manager instance for a registered capability, or None."""
        return self._capability_managers.get(name)

    # ------------------------------------------------------------------
    # Deep refresh — full reconstruct from init.json
    # ------------------------------------------------------------------

    def _read_init(self) -> dict | None:
        """Read and validate init.json from working directory."""
        import json
        from .init_schema import validate_init

        init_path = self._working_dir / "init.json"
        if not init_path.is_file():
            return None

        try:
            data = json.loads(init_path.read_text())
        except (json.JSONDecodeError, OSError):
            self._log("refresh_init_error", error="failed to read init.json")
            return None

        try:
            validate_init(data)
        except ValueError as e:
            self._log("refresh_init_error", error=str(e))
            return None

        return data

    def _perform_refresh(self) -> None:
        """Full reconstruct from init.json, preserving conversation history."""
        self._log("refresh_start")

        data = self._read_init()
        if data is None:
            self._log("refresh_skipped", reason="no valid init.json")
            return

        from .config_resolve import (
            load_env_file,
            resolve_env,
            _resolve_capabilities,
            _resolve_addons,
        )
        from lingtai_kernel.config import AgentConfig

        env_file = data.get("env_file")
        if env_file:
            load_env_file(env_file)

        m = data["manifest"]

        # Save conversation history
        saved_interface = None
        if self._session.chat is not None:
            saved_interface = self._session.chat.interface

        # Tear down
        # Cancel soul timer to prevent racing on config/service during rebuild
        self._cancel_soul_timer()

        for name, mgr in self._addon_managers.items():
            if hasattr(mgr, "stop"):
                try:
                    mgr.stop()
                except Exception:
                    pass

        for client in getattr(self, "_mcp_clients", []):
            try:
                client.close()
            except Exception:
                pass
        self._mcp_clients = []

        self._sealed = False
        self._tool_handlers.clear()
        self._tool_schemas.clear()
        self._capabilities.clear()
        self._capability_managers.clear()
        self._addon_managers.clear()

        self._intrinsics.clear()
        self._wire_intrinsics()

        # Reset capability-owned flags
        self._eigen_owns_memory = False
        self._mailbox_name = "mail box"
        self._mailbox_tool = "mail"
        if hasattr(self, "_post_molt_hooks"):
            self._post_molt_hooks.clear()

        # Reset prompt manager
        self._prompt_manager._sections.clear()

        # Reconstruct LLM service if changed
        llm = m["llm"]
        api_key = resolve_env(llm["api_key"], llm.get("api_key_env"))
        new_provider = llm["provider"]
        new_model = llm["model"]
        new_base_url = llm["base_url"]

        if (
            new_provider != self.service.provider
            or new_model != self.service.model
            or new_base_url != getattr(self.service, "_base_url", None)
        ):
            self.service = LLMService(
                provider=new_provider, model=new_model,
                api_key=api_key, base_url=new_base_url,
            )
            self._session._llm_service = self.service

        # Reload config
        soul = m["soul"]
        self._config = AgentConfig(
            stamina=m["stamina"],
            soul_delay=soul["delay"],
            max_turns=m["max_turns"],
            language=m["language"],
            context_limit=m["context_limit"],
            molt_pressure=m["molt_pressure"],
            molt_prompt=m["molt_prompt"],
        )
        self._soul_delay = max(1.0, self._config.soul_delay)
        self._session._config = self._config

        # Reload covenant and memory
        covenant = data.get("covenant", "")
        system_dir = self._working_dir / "system"
        covenant_file = system_dir / "covenant.md"
        memory_file = system_dir / "memory.md"

        if not covenant and covenant_file.is_file():
            covenant = covenant_file.read_text()
        if covenant:
            self._prompt_manager.write_section("covenant", covenant, protected=True)

        loaded_memory = ""
        if memory_file.is_file():
            loaded_memory = memory_file.read_text()
        if loaded_memory.strip():
            self._prompt_manager.write_section("memory", loaded_memory)

        # Reload principle
        principle = data.get("principle", "")
        if principle:
            self._prompt_manager.write_section("principle", principle, protected=True)

        # Re-run capability setup
        capabilities = _resolve_capabilities(m["capabilities"])
        if capabilities:
            from .capabilities import expand_groups, _GROUPS
            expanded: dict[str, dict] = {}
            for name, cap_kwargs in capabilities.items():
                if name in _GROUPS:
                    for sub in _GROUPS[name]:
                        expanded[sub] = {}
                else:
                    expanded[name] = cap_kwargs
            capabilities = expanded
            for name, cap_kwargs in capabilities.items():
                self._setup_capability(name, **cap_kwargs)

        # Re-run addon setup
        addons = _resolve_addons(data.get("addons"))
        if addons:
            from .addons import setup_addon
            for addon_name, addon_kwargs in addons.items():
                mgr = setup_addon(self, addon_name, **(addon_kwargs or {}))
                self._addon_managers[addon_name] = mgr

        # Reload MCP
        self._load_mcp_from_workdir()

        # Persist LLM config
        try:
            import json as _json
            llm_config: dict = {
                "provider": self.service.provider,
                "model": self.service.model,
            }
            _base_url = getattr(self.service, "_base_url", None)
            if isinstance(_base_url, str) and _base_url:
                llm_config["base_url"] = _base_url
            llm_dir = self._working_dir / "system"
            llm_dir.mkdir(exist_ok=True)
            (llm_dir / "llm.json").write_text(
                _json.dumps(llm_config, ensure_ascii=False)
            )
        except (TypeError, AttributeError, OSError):
            pass

        # Re-write manifest and identity
        self._update_identity()

        # Re-seal
        self._sealed = True

        # Rebuild session with preserved history
        if saved_interface is not None:
            self._session._rebuild_session(saved_interface)

        # Start addon managers
        for name, mgr in self._addon_managers.items():
            if hasattr(mgr, "start"):
                mgr.start()

        self._log(
            "refresh_complete",
            capabilities=[name for name, _ in self._capabilities],
            addons=list(self._addon_managers.keys()),
            tools=list(self._tool_handlers.keys()),
        )
