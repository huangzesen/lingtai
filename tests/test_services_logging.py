"""Tests for lingtai.services.logging."""
import json
import threading
from pathlib import Path

from lingtai_kernel.services.logging import LoggingService, JSONLLoggingService


class TestJSONLLoggingService:

    def test_log_writes_jsonl(self, tmp_path):
        """Events are written as JSON lines."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.log({"type": "test", "value": 42})
        svc.log({"type": "test", "value": 99})
        svc.close()

        lines = log_file.read_text().strip().split("\n")
        assert len(lines) == 2
        assert json.loads(lines[0]) == {"type": "test", "value": 42}
        assert json.loads(lines[1]) == {"type": "test", "value": 99}

    def test_log_default_str_for_non_serializable(self, tmp_path):
        """Non-JSON-serializable values are converted via str()."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.log({"path": Path("/tmp/foo")})
        svc.close()

        line = json.loads(log_file.read_text().strip())
        assert line["path"] == "/tmp/foo"

    def test_close_is_idempotent(self, tmp_path):
        """Calling close() twice does not raise."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.close()
        svc.close()  # should not raise

    def test_log_after_close_is_noop(self, tmp_path):
        """Logging after close does not raise or write."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.close()
        svc.log({"type": "test"})  # should not raise
        assert log_file.read_text().strip() == ""

    def test_creates_parent_dirs(self, tmp_path):
        """Parent directories are created if they don't exist."""
        log_file = tmp_path / "nested" / "dir" / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.log({"type": "test"})
        svc.close()
        assert log_file.exists()

    def test_append_mode(self, tmp_path):
        """Opening an existing file appends, does not truncate."""
        log_file = tmp_path / "test.jsonl"
        log_file.write_text('{"existing": true}\n')

        svc = JSONLLoggingService(log_file)
        svc.log({"type": "new"})
        svc.close()

        lines = log_file.read_text().strip().split("\n")
        assert len(lines) == 2
        assert json.loads(lines[0])["existing"] is True
        assert json.loads(lines[1])["type"] == "new"

    def test_thread_safety(self, tmp_path):
        """Concurrent writes don't corrupt the file."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)

        def writer(thread_id):
            for i in range(50):
                svc.log({"thread": thread_id, "i": i})

        threads = [threading.Thread(target=writer, args=(t,)) for t in range(4)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        svc.close()

        lines = log_file.read_text().strip().split("\n")
        assert len(lines) == 200  # 4 threads * 50 writes
        # Every line must be valid JSON
        for line in lines:
            json.loads(line)

    def test_abc_cannot_instantiate(self):
        """LoggingService ABC cannot be instantiated directly."""
        try:
            LoggingService()
            assert False, "Should have raised TypeError"
        except TypeError:
            pass


# ---------------------------------------------------------------------------
# BaseAgent + LoggingService integration
# ---------------------------------------------------------------------------

from unittest.mock import MagicMock
from lingtai import BaseAgent, AgentState
from lingtai.llm import ToolCall
from lingtai_kernel.loop_guard import LoopGuard


def make_mock_service():
    svc = MagicMock()
    svc.model = "test-model"
    svc.make_tool_result.return_value = {"role": "tool", "content": "ok"}
    return svc


class TestBaseAgentLoggingIntegration:

    def test_tool_call_logged(self, tmp_path):
        """Executing a tool logs tool_call and tool_result events."""
        from lingtai_kernel.tool_executor import ToolExecutor

        agent = BaseAgent(
            agent_name="test",
            service=make_mock_service(),
            base_dir=tmp_path,
        )
        agent.add_tool("greet", schema={"type": "object", "properties": {}}, handler=lambda args: {"status": "ok"})

        guard = LoopGuard()
        errors = []
        tc = ToolCall(name="greet", args={})
        executor = ToolExecutor(
            dispatch_fn=agent._dispatch_tool,
            make_tool_result_fn=lambda name, result, **kw: agent.service.make_tool_result(
                name, result, provider=agent._config.provider, **kw
            ),
            guard=guard,
            known_tools=set(agent._intrinsics) | set(agent._mcp_handlers),
            logger_fn=agent._log,
        )
        executor.execute([tc], collected_errors=errors)

        # Log file should exist in working dir
        log_file = tmp_path / "test" / "logs" / "events.jsonl"
        assert log_file.is_file()
        events = agent._log_service.get_events()
        types = [e["type"] for e in events]
        assert "tool_call" in types
        assert "tool_result" in types
        # Verify agent_name is injected
        assert all(e["agent_name"] == "test" for e in events)
        # Verify ts is present
        assert all("ts" in e for e in events)

    def test_auto_logging_to_working_dir(self, tmp_path):
        """Agent always creates JSONL log in working dir."""
        from lingtai_kernel.tool_executor import ToolExecutor

        agent = BaseAgent(
            agent_name="test",
            service=make_mock_service(),
            base_dir=tmp_path,
        )
        agent.add_tool("greet", schema={"type": "object", "properties": {}}, handler=lambda args: {"status": "ok"})

        guard = LoopGuard()
        errors = []
        tc = ToolCall(name="greet", args={})
        executor = ToolExecutor(
            dispatch_fn=agent._dispatch_tool,
            make_tool_result_fn=lambda name, result, **kw: agent.service.make_tool_result(
                name, result, provider=agent._config.provider, **kw
            ),
            guard=guard,
            known_tools=set(agent._intrinsics) | set(agent._mcp_handlers),
            logger_fn=agent._log,
        )
        executor.execute([tc], collected_errors=errors)

        # Log file should exist in working dir
        log_file = tmp_path / "test" / "logs" / "events.jsonl"
        assert log_file.is_file()
        events = agent._log_service.get_events()
        types = [e["type"] for e in events]
        assert "tool_call" in types

    def test_state_change_logged(self, tmp_path):
        """State transitions are logged."""
        agent = BaseAgent(
            agent_name="test",
            service=make_mock_service(),
            base_dir=tmp_path,
        )
        agent._set_state(AgentState.ACTIVE, reason="test")

        events = agent._log_service.get_events()
        state_events = [e for e in events if e["type"] == "agent_state"]
        assert len(state_events) >= 1
