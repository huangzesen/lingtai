from stoai_kernel.state import AgentState


def test_agent_state_values():
    assert AgentState.ACTIVE.value == "active"
    assert AgentState.IDLE.value == "idle"
