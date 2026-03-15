"""Tests for composable layers (diary, plan)."""
import tempfile
from pathlib import Path
from unittest.mock import MagicMock

from stoai.layers.diary import DiaryManager, add_diary_layer, SCHEMA as DIARY_SCHEMA, DESCRIPTION as DIARY_DESC
from stoai.layers.plan import PlanManager, add_plan_layer, SCHEMA as PLAN_SCHEMA, DESCRIPTION as PLAN_DESC
from stoai.agent import BaseAgent
from stoai.config import AgentConfig


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Diary layer
# ---------------------------------------------------------------------------

def test_diary_schema():
    assert isinstance(DIARY_SCHEMA, dict)
    assert "properties" in DIARY_SCHEMA
    assert DIARY_DESC


def test_diary_manager(tmp_path):
    mgr = DiaryManager(diary_dir=tmp_path)
    result = mgr.handle({"action": "save", "title": "test-entry",
                          "summary": "A test diary entry", "content": "Full details here."})
    assert result["status"] == "ok"
    result = mgr.handle({"action": "catalogue", "n": 10})
    assert len(result["entries"]) == 1
    assert result["entries"][0]["title"] == "test-entry"
    result = mgr.handle({"action": "view", "title": "test-entry"})
    assert "Full details here." in result["content"]


def test_diary_disabled():
    mgr = DiaryManager(diary_dir=None)
    result = mgr.handle({"action": "save", "title": "x", "summary": "y", "content": "z"})
    assert "not configured" in result["status"]


def test_diary_unknown_action():
    mgr = DiaryManager(diary_dir=None)
    result = mgr.handle({"action": "delete"})
    assert "error" in result


def test_add_diary_layer(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = add_diary_layer(agent, diary_dir=tmp_path)
    assert isinstance(mgr, DiaryManager)
    # Tool should be registered as MCP handler
    assert "manage_diary" in agent._mcp_handlers
    # System prompt section should be set
    assert agent._prompt_manager.read_section("diary_instructions") is not None


# ---------------------------------------------------------------------------
# Plan layer
# ---------------------------------------------------------------------------

def test_plan_schema():
    assert isinstance(PLAN_SCHEMA, dict)
    assert "properties" in PLAN_SCHEMA
    assert PLAN_DESC


def test_plan_create_read(tmp_path):
    mgr = PlanManager(working_dir=tmp_path)
    result = mgr.handle({"action": "create", "content": "# My Plan\n- [ ] Step 1\n- [ ] Step 2"})
    assert result["status"] == "ok"
    result = mgr.handle({"action": "read"})
    assert "Step 1" in result["content"]


def test_plan_create_empty():
    mgr = PlanManager()
    result = mgr.handle({"action": "create", "content": ""})
    assert "error" in result


def test_plan_read_nonexistent(tmp_path):
    mgr = PlanManager(working_dir=tmp_path)
    result = mgr.handle({"action": "read"})
    assert "error" in result


def test_plan_update(tmp_path):
    mgr = PlanManager(working_dir=tmp_path)
    mgr.handle({"action": "create", "content": "# Plan v1"})
    result = mgr.handle({"action": "update", "content": "# Plan v2"})
    assert result["status"] == "ok"
    result = mgr.handle({"action": "read"})
    assert "Plan v2" in result["content"]


def test_plan_update_nonexistent(tmp_path):
    mgr = PlanManager(working_dir=tmp_path)
    result = mgr.handle({"action": "update", "content": "new"})
    assert "error" in result


def test_plan_check_off(tmp_path):
    mgr = PlanManager(working_dir=tmp_path)
    mgr.handle({"action": "create", "content": "# Plan\n- [ ] Step 1\n- [ ] Step 2"})
    result = mgr.handle({"action": "check_off", "step": "Step 1"})
    assert result["status"] == "ok"
    assert "- [x] Step 1" in result["checked"]
    # Verify file updated
    result = mgr.handle({"action": "read"})
    assert "- [x] Step 1" in result["content"]
    assert "- [ ] Step 2" in result["content"]


def test_plan_check_off_partial_match(tmp_path):
    mgr = PlanManager(working_dir=tmp_path)
    mgr.handle({"action": "create", "content": "# Plan\n- [ ] Implement the feature\n- [ ] Write tests"})
    result = mgr.handle({"action": "check_off", "step": "implement"})
    assert result["status"] == "ok"


def test_plan_check_off_not_found(tmp_path):
    mgr = PlanManager(working_dir=tmp_path)
    mgr.handle({"action": "create", "content": "# Plan\n- [ ] Step 1"})
    result = mgr.handle({"action": "check_off", "step": "nonexistent step"})
    assert "error" in result


def test_plan_unknown_action(tmp_path):
    mgr = PlanManager(working_dir=tmp_path)
    result = mgr.handle({"action": "delete"})
    assert "error" in result


def test_add_plan_layer(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = add_plan_layer(agent, working_dir=tmp_path)
    assert isinstance(mgr, PlanManager)
    # Tool should be registered as MCP handler
    assert "plan" in agent._mcp_handlers
    # System prompt section should be set
    assert agent._prompt_manager.read_section("plan_instructions") is not None
