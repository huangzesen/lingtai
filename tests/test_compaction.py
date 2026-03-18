"""Tests for context compaction in LLMService.check_and_compact().

Exercises the full compaction pipeline:
  1. estimate_context_tokens() detects context > threshold
  2. find_compaction_boundary() splits at the right turn
  3. format_for_summary() produces text for the summarizer
  4. The new ChatInterface has: system + summary + ack + recent turns
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

from stoai.llm.interface import (
    ChatInterface,
    TextBlock,
    ToolCallBlock,
    ToolResultBlock,
)
from stoai.llm.base import ChatSession, FunctionSchema
from stoai.llm.service import LLMService, get_context_limit


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _build_long_conversation(
    num_turns: int = 10,
    text_size: int = 5000,
) -> ChatInterface:
    """Build a ChatInterface with many turns of large text.

    Each turn = user message + assistant response.
    """
    iface = ChatInterface()
    iface.add_system("You are a helpful assistant.")

    filler = "x" * text_size  # ~text_size/4 tokens with char estimate

    for i in range(num_turns):
        iface.add_user_message(f"Turn {i}: {filler}")
        iface.add_assistant_message(
            [TextBlock(text=f"Response {i}: {filler}")],
        )

    return iface


def _build_conversation_with_tools(
    num_turns: int = 8,
    text_size: int = 5000,
) -> ChatInterface:
    """Build a ChatInterface with user messages, tool calls, and tool results."""
    iface = ChatInterface()
    tools = [{"name": "search", "description": "Search", "parameters": {}}]
    iface.add_system("You are a helpful assistant.", tools=tools)

    filler = "x" * text_size

    for i in range(num_turns):
        iface.add_user_message(f"Turn {i}: Please search for info about topic {i}")
        # Assistant calls a tool
        iface.add_assistant_message(
            [ToolCallBlock(id=f"call_{i}", name="search", args={"q": f"topic {i}"})],
        )
        # Tool result comes back
        iface.add_tool_results(
            [ToolResultBlock(id=f"call_{i}", name="search", content=f"Result {i}: {filler}")]
        )
        # Assistant responds with the result
        iface.add_assistant_message(
            [TextBlock(text=f"Based on the search, here's what I found about topic {i}.")],
        )

    return iface


class FakeChatSession(ChatSession):
    """Minimal ChatSession for testing compaction logic."""

    def __init__(self, interface: ChatInterface, ctx_window: int = 0):
        self._interface = interface
        self._context_window_val = ctx_window
        self._model = "test-model"
        self._agent_type = "test"
        self._tracked = True
        self.session_id = "test-session"

    @property
    def interface(self) -> ChatInterface:
        return self._interface

    def send(self, message):
        raise NotImplementedError

    def context_window(self) -> int:
        return self._context_window_val


# ---------------------------------------------------------------------------
# Tests — ChatInterface compaction primitives
# ---------------------------------------------------------------------------


class TestCompactionBoundary:
    """Tests for find_compaction_boundary()."""

    def test_short_conversation_returns_none(self):
        """Conversations with < 6 non-system entries cannot be compacted."""
        iface = ChatInterface()
        iface.add_system("sys")
        iface.add_user_message("hello")
        iface.add_assistant_message([TextBlock(text="hi")])
        assert iface.find_compaction_boundary(keep_turns=3) is None

    def test_finds_boundary_with_enough_turns(self):
        """Should find a boundary that keeps the last 3 turns."""
        iface = _build_long_conversation(num_turns=6, text_size=100)
        boundary = iface.find_compaction_boundary(keep_turns=3)
        assert boundary is not None

        # Entries after boundary should contain the last 3 user messages
        conv = [e for e in iface.entries if e.role != "system"]
        kept = [e for e in conv if e.id >= boundary]
        kept_user_texts = [
            e for e in kept
            if e.role == "user"
            and any(isinstance(b, TextBlock) for b in e.content)
        ]
        assert len(kept_user_texts) == 3

    def test_boundary_with_tool_turns(self):
        """Tool call/result exchanges within a turn should not be split."""
        iface = _build_conversation_with_tools(num_turns=6, text_size=100)
        boundary = iface.find_compaction_boundary(keep_turns=3)
        assert boundary is not None

        # The kept portion should have complete tool turns (no orphaned results)
        conv = [e for e in iface.entries if e.role != "system"]
        kept = [e for e in conv if e.id >= boundary]
        # Each kept turn has: user msg, assistant tool_call, tool_result, assistant text = 4 entries
        # 3 turns = 12 entries
        assert len(kept) == 12


class TestFormatForSummary:
    """Tests for format_for_summary()."""

    def test_formats_text_entries(self):
        iface = _build_long_conversation(num_turns=6, text_size=100)
        boundary = iface.find_compaction_boundary(keep_turns=3)
        text = iface.format_for_summary(boundary)
        assert "[user]" in text
        assert "[assistant]" in text
        # Should NOT contain entries from the kept portion
        assert "Turn 5" not in text  # last turn should be kept

    def test_formats_tool_entries(self):
        iface = _build_conversation_with_tools(num_turns=6, text_size=100)
        boundary = iface.find_compaction_boundary(keep_turns=3)
        text = iface.format_for_summary(boundary)
        assert "tool_use: search" in text
        assert "tool_result(search)" in text


class TestEstimateContextTokens:
    """Tests for estimate_context_tokens()."""

    def test_grows_with_conversation(self):
        small = _build_long_conversation(num_turns=2, text_size=100)
        large = _build_long_conversation(num_turns=10, text_size=100)
        assert large.estimate_context_tokens() > small.estimate_context_tokens()

    def test_accounts_for_system_prompt(self):
        iface = ChatInterface()
        iface.add_system("A" * 1000)
        estimate = iface.estimate_context_tokens()
        assert estimate > 100  # 1000 chars / ~8 = ~125 tokens (Gemini) or ~4 = ~250 tokens (tiktoken)


# ---------------------------------------------------------------------------
# Tests — LLMService.check_and_compact() integration
# ---------------------------------------------------------------------------


class TestCheckAndCompact:
    """Integration tests for the full compaction pipeline via LLMService."""

    def _make_service_and_session(
        self, num_turns=10, text_size=5000, ctx_window=None
    ):
        """Create an LLMService + FakeChatSession with a long conversation.

        Sets ctx_window so that the conversation exceeds the 0.8 threshold.
        """
        iface = _build_long_conversation(num_turns=num_turns, text_size=text_size)
        estimate = iface.estimate_context_tokens()

        if ctx_window is None:
            # Set context window so estimate is ~90% of it (above 80% threshold)
            ctx_window = int(estimate / 0.9)

        session = FakeChatSession(iface, ctx_window=ctx_window)

        # Mock LLMService — we need create_session to return a FakeChatSession
        # but we want the real check_and_compact logic
        service = MagicMock(spec=LLMService)
        service.check_and_compact = LLMService.check_and_compact.__get__(service)

        # When create_session is called (for the compacted session), return a
        # FakeChatSession backed by the new interface
        def fake_create_session(system_prompt, tools=None, **kwargs):
            new_iface = kwargs.get("interface")
            if new_iface is None:
                new_iface = ChatInterface()
                new_iface.add_system(system_prompt)
            new_session = FakeChatSession(new_iface, ctx_window=ctx_window)
            return new_session

        service.create_session.side_effect = fake_create_session

        return service, session

    def test_compaction_triggers_above_threshold(self):
        """Compaction should trigger when estimate > threshold * ctx_window."""
        service, session = self._make_service_and_session(
            num_turns=10, text_size=5000
        )

        def fake_summarizer(text):
            return "Summary of previous conversation."

        result = service.check_and_compact(
            session, summarizer=fake_summarizer, threshold=0.8
        )
        assert result is not None, "Compaction should have triggered"

    def test_compaction_preserves_recent_turns(self):
        """The compacted session should keep the last 3 turns intact."""
        service, session = self._make_service_and_session(
            num_turns=10, text_size=5000
        )

        def fake_summarizer(text):
            return "Summary: discussed topics 0-6."

        result = service.check_and_compact(
            session, summarizer=fake_summarizer, threshold=0.8
        )
        assert result is not None

        new_iface = result.interface
        conv = new_iface.conversation_entries()

        # Should have: summary (user) + ack (assistant) + 3 turns (6 entries) = 8
        # Check that the last 3 original user turns are preserved
        user_texts = []
        for e in conv:
            if e.role == "user":
                for b in e.content:
                    if isinstance(b, TextBlock) and b.text.startswith("Turn"):
                        user_texts.append(b.text)

        assert len(user_texts) == 3
        assert "Turn 7" in user_texts[0]
        assert "Turn 8" in user_texts[1]
        assert "Turn 9" in user_texts[2]

    def test_compaction_includes_summary(self):
        """The compacted session should contain the summary text."""
        service, session = self._make_service_and_session()

        summary_text = "The user discussed 10 topics about data analysis."

        result = service.check_and_compact(
            session,
            summarizer=lambda _: summary_text,
            threshold=0.8,
        )
        assert result is not None

        # Find the summary in the new interface
        new_iface = result.interface
        found_summary = False
        for e in new_iface.entries:
            for b in e.content:
                if isinstance(b, TextBlock) and summary_text in b.text:
                    found_summary = True
        assert found_summary, "Summary text should be in the compacted interface"

    def test_compaction_preserves_system_prompt(self):
        """System prompt should be preserved after compaction."""
        service, session = self._make_service_and_session()

        result = service.check_and_compact(
            session, summarizer=lambda _: "Summary.", threshold=0.8
        )
        assert result is not None

        # create_session should have been called with the original system prompt
        call_args, call_kwargs = service.create_session.call_args
        system_prompt = call_args[0] if call_args else call_kwargs.get("system_prompt")
        assert system_prompt == "You are a helpful assistant."

    def test_no_compaction_below_threshold(self):
        """No compaction when context usage is below threshold."""
        iface = _build_long_conversation(num_turns=10, text_size=5000)
        estimate = iface.estimate_context_tokens()

        # Set context window much larger than estimate
        session = FakeChatSession(iface, ctx_window=estimate * 10)
        service = MagicMock(spec=LLMService)
        service.check_and_compact = LLMService.check_and_compact.__get__(service)

        result = service.check_and_compact(
            session, summarizer=lambda _: "Summary.", threshold=0.8
        )
        assert result is None, "Should not compact when below threshold"

    def test_no_compaction_without_summarizer(self):
        """No compaction when summarizer is None."""
        service, session = self._make_service_and_session()

        result = service.check_and_compact(session, summarizer=None)
        assert result is None

    def test_no_compaction_when_context_window_zero(self):
        """No compaction when context_window is 0 (unknown)."""
        iface = _build_long_conversation(num_turns=10, text_size=5000)
        session = FakeChatSession(iface, ctx_window=0)
        service = MagicMock(spec=LLMService)
        service.check_and_compact = LLMService.check_and_compact.__get__(service)

        result = service.check_and_compact(
            session, summarizer=lambda _: "Summary.", threshold=0.8
        )
        assert result is None

    def test_compaction_reduces_token_count(self):
        """The compacted session should have fewer estimated tokens."""
        service, session = self._make_service_and_session(
            num_turns=10, text_size=5000
        )
        before = session.interface.estimate_context_tokens()

        result = service.check_and_compact(
            session, summarizer=lambda _: "Brief summary.", threshold=0.8
        )
        assert result is not None

        after = result.interface.estimate_context_tokens()
        assert after < before, (
            f"Compacted tokens ({after}) should be less than original ({before})"
        )

    def test_compaction_with_tool_conversation(self):
        """Compaction should work correctly with tool call/result turns."""
        iface = _build_conversation_with_tools(num_turns=8, text_size=5000)
        estimate = iface.estimate_context_tokens()
        ctx_window = int(estimate / 0.9)

        session = FakeChatSession(iface, ctx_window=ctx_window)
        service = MagicMock(spec=LLMService)
        service.check_and_compact = LLMService.check_and_compact.__get__(service)

        def fake_create_session(system_prompt, tools=None, **kwargs):
            new_iface = kwargs.get("interface")
            return FakeChatSession(new_iface or ChatInterface(), ctx_window=ctx_window)

        service.create_session.side_effect = fake_create_session

        result = service.check_and_compact(
            session, summarizer=lambda _: "Tool usage summary.", threshold=0.8
        )
        assert result is not None

        # Verify tool results in kept portion are intact
        new_iface = result.interface
        tool_results = [
            b for e in new_iface.entries
            for b in e.content
            if isinstance(b, ToolResultBlock)
        ]
        assert len(tool_results) > 0, "Kept turns should include tool results"


# ---------------------------------------------------------------------------
# Tests — get_context_limit
# ---------------------------------------------------------------------------


class TestGetContextLimit:
    """Tests for the context limit resolver."""

    def test_default_for_unknown_model(self):
        """Unknown models get the 256k default."""
        limit = get_context_limit("completely-made-up-model-v99")
        assert limit == 256_000

    def test_default_for_empty_string(self):
        """Empty model name gets default."""
        assert get_context_limit("") == 256_000


# ---------------------------------------------------------------------------
# Tests — Compaction pressure system in BaseAgent._handle_request
# ---------------------------------------------------------------------------


def _make_agent_with_anima(tmp_path):
    """Create an Agent with anima capability and mocked LLM service."""
    from stoai.agent import Agent

    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return Agent(
        agent_name="test",
        service=svc,
        base_dir=tmp_path,
        capabilities=["anima"],
    )


def test_compaction_warning_injected_at_80_percent(tmp_path):
    """At 80%+ context, a [system] warning should be prepended to content."""
    agent = _make_agent_with_anima(tmp_path)
    agent.start()
    try:
        # Mock session to report 85% context pressure
        agent._session.get_context_pressure = lambda: 0.85
        agent._session._compaction_warnings = 0

        # Capture what gets sent to LLM
        sent_content = []

        def capture_send(content):
            sent_content.append(content)
            # Return a mock LLMResponse
            resp = MagicMock()
            resp.text = "ok"
            resp.tool_calls = []
            resp.usage = None
            return resp

        agent._session.send = capture_send

        # Use _handle_request directly with a mock message
        from stoai.message import _make_message, MSG_REQUEST
        msg = _make_message(MSG_REQUEST, sender="test", content="do something")
        agent._handle_request(msg)

        assert len(sent_content) > 0
        assert any("[system]" in c for c in sent_content)
        assert any("compact" in c.lower() for c in sent_content)
        assert agent._session._compaction_warnings == 1
    finally:
        agent.stop()


def test_compaction_resets_warning_counter(tmp_path):
    """After successful compact, warning counter should reset to 0."""
    from stoai.agent import Agent
    from stoai.llm.interface import ChatInterface

    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"

    def fake_create_session(**kwargs):
        mock_chat = MagicMock()
        iface = ChatInterface()
        iface.add_system("You are helpful.")
        mock_chat.interface = iface
        mock_chat.context_window.return_value = 100_000
        return mock_chat

    svc.create_session.side_effect = fake_create_session

    agent = Agent(
        agent_name="test", service=svc, base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        # Ensure a session exists
        agent._session.ensure_session()
        agent._session._compaction_warnings = 2  # simulate 2 warnings

        mgr = agent.get_capability("anima")
        result = mgr.handle({
            "object": "context",
            "action": "compact",
            "summary": "My important context summary.",
        })

        assert result["status"] == "ok"
        assert agent._session._compaction_warnings == 0
    finally:
        agent.stop()
