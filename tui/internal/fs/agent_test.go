// internal/fs/agent_test.go
package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadAgent_ValidManifest(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "alice")
	os.MkdirAll(agentDir, 0o755)

	manifest := map[string]interface{}{
		"agent_name":   "alice",
		"address":      "alice",
		"state":        "ACTIVE",
		"admin":        map[string]interface{}{"karma": true},
		"capabilities": []string{"file", "vision"},
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(agentDir, ".agent.json"), data, 0o644)

	node, err := ReadAgent(agentDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.AgentName != "alice" {
		t.Errorf("agent_name = %q, want %q", node.AgentName, "alice")
	}
	if node.State != "ACTIVE" {
		t.Errorf("state = %q, want %q", node.State, "ACTIVE")
	}
	if node.IsHuman {
		t.Error("is_human = true, want false")
	}
	if len(node.Capabilities) != 2 {
		t.Errorf("capabilities len = %d, want 2", len(node.Capabilities))
	}
}

func TestReadAgent_HumanAgent(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "human")
	os.MkdirAll(agentDir, 0o755)

	// admin: null → is_human = true
	manifest := map[string]interface{}{
		"agent_name": "human",
		"address":    "human",
		"admin":      nil,
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(agentDir, ".agent.json"), data, 0o644)

	node, err := ReadAgent(agentDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !node.IsHuman {
		t.Error("is_human = false, want true (admin: null)")
	}
}

func TestReadAgent_MissingAdminKey(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "human2")
	os.MkdirAll(agentDir, 0o755)

	// admin key absent → is_human = true
	manifest := map[string]interface{}{
		"agent_name": "human2",
		"address":    "human2",
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(agentDir, ".agent.json"), data, 0o644)

	node, err := ReadAgent(agentDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !node.IsHuman {
		t.Error("is_human = false, want true (admin key absent)")
	}
}

func TestReadAgent_NoManifest(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadAgent(dir)
	if err == nil {
		t.Error("expected error for missing .agent.json")
	}
}
