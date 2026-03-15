"""Tests for stoai.services.logging."""
import json
import threading
from pathlib import Path

from stoai.services.logging import LoggingService, JSONLLoggingService


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
from stoai import BaseAgent, AgentState
from stoai.llm import ToolCall
from stoai.loop_guard import LoopGuard


def make_mock_service():
    svc = MagicMock()
    svc.model = "test-model"
    svc.make_tool_result.return_value = {"role": "tool", "content": "ok"}
    return svc


class TestBaseAgentLoggingIntegration:

    def test_tool_call_logged(self, tmp_path):
        """Executing a tool logs tool_call and tool_result events."""
        log_file = tmp_path / "agent.jsonl"
        log_svc = JSONLLoggingService(log_file)

        agent = BaseAgent(
            agent_id="test",
            service=make_mock_service(),
            logging_service=log_svc,
        )
        agent.add_tool("greet", schema={"type": "object", "properties": {}}, handler=lambda args: {"status": "ok"})

        guard = LoopGuard()
        errors = []
        tc = ToolCall(name="greet", args={})
        agent._execute_single_tool(tc, guard, errors)
        log_svc.close()

        lines = log_file.read_text().strip().split("\n")
        events = [json.loads(line) for line in lines]
        types = [e["type"] for e in events]
        assert "tool_call" in types
        assert "tool_result" in types
        # Verify agent_id is injected
        assert all(e["agent_id"] == "test" for e in events)
        # Verify ts is present
        assert all("ts" in e for e in events)

    def test_no_logging_service_is_noop(self, tmp_path):
        """Agent without logging_service does not raise on tool execution."""
        agent = BaseAgent(
            agent_id="test",
            service=make_mock_service(),
        )
        agent.add_tool("greet", schema={"type": "object", "properties": {}}, handler=lambda args: {"status": "ok"})

        guard = LoopGuard()
        errors = []
        tc = ToolCall(name="greet", args={})
        agent._execute_single_tool(tc, guard, errors)  # should not raise

    def test_state_change_logged(self, tmp_path):
        """State transitions are logged."""
        log_file = tmp_path / "agent.jsonl"
        log_svc = JSONLLoggingService(log_file)

        agent = BaseAgent(
            agent_id="test",
            service=make_mock_service(),
            logging_service=log_svc,
        )
        agent._set_state(AgentState.ACTIVE, reason="test")
        log_svc.close()

        lines = log_file.read_text().strip().split("\n")
        events = [json.loads(line) for line in lines]
        state_events = [e for e in events if e["type"] == "agent_state"]
        assert len(state_events) >= 1
