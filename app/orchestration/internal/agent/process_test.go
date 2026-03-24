package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForAgentJSON_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "test-agent")
	os.MkdirAll(agentDir, 0755)

	// Write .agent.json before calling wait
	manifest := map[string]interface{}{"address": agentDir, "agent_name": "test"}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(agentDir, ".agent.json"), data, 0644)

	err := WaitForAgentJSON(agentDir, 2*time.Second)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestWaitForAgentJSON_TimeoutProcess(t *testing.T) {
	dir := t.TempDir()
	err := WaitForAgentJSON(dir, 300*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}
