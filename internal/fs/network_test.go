package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func setupTestNetwork(t *testing.T) string {
	t.Helper()
	base := t.TempDir()

	aliceDir := filepath.Join(base, "alice")
	os.MkdirAll(filepath.Join(aliceDir, "mailbox", "inbox"), 0o755)
	os.MkdirAll(filepath.Join(aliceDir, "mailbox", "sent"), 0o755)
	os.MkdirAll(filepath.Join(aliceDir, "delegates"), 0o755)

	writeJSON(t, filepath.Join(aliceDir, ".agent.json"), map[string]interface{}{
		"agent_name": "alice", "address": aliceDir, "state": "ACTIVE",
		"admin": map[string]interface{}{"karma": true},
	})

	bobDir := filepath.Join(base, "bob")
	ledger := fmt.Sprintf(`{"event":"avatar","name":"bob","working_dir":%q,"ts":1000}`, bobDir)
	os.WriteFile(filepath.Join(aliceDir, "delegates", "ledger.jsonl"), []byte(ledger+"\n"), 0o644)

	contacts := []map[string]string{{"address": bobDir, "name": "bob"}}
	data, _ := json.Marshal(contacts)
	os.WriteFile(filepath.Join(aliceDir, "mailbox", "contacts.json"), data, 0o644)

	os.MkdirAll(filepath.Join(bobDir, "mailbox", "inbox"), 0o755)
	writeJSON(t, filepath.Join(bobDir, ".agent.json"), map[string]interface{}{
		"agent_name": "bob", "address": bobDir, "state": "IDLE",
		"admin": map[string]interface{}{"karma": false},
	})

	humanDir := filepath.Join(base, "human")
	os.MkdirAll(filepath.Join(humanDir, "mailbox", "inbox"), 0o755)
	writeJSON(t, filepath.Join(humanDir, ".agent.json"), map[string]interface{}{
		"agent_name": "human", "address": humanDir, "admin": nil,
	})

	return base
}

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	data, _ := json.Marshal(v)
	os.WriteFile(path, data, 0o644)
}

func TestBuildNetwork(t *testing.T) {
	base := setupTestNetwork(t)

	net, err := BuildNetwork(base)
	if err != nil {
		t.Fatalf("build network: %v", err)
	}

	if len(net.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3", len(net.Nodes))
	}
	if len(net.AvatarEdges) != 1 {
		t.Errorf("avatar edges = %d, want 1", len(net.AvatarEdges))
	}
	if len(net.ContactEdges) != 1 {
		t.Errorf("contact edges = %d, want 1", len(net.ContactEdges))
	}
	if net.Stats.Active != 1 {
		t.Errorf("active = %d, want 1", net.Stats.Active)
	}
	if net.Stats.Idle != 1 {
		t.Errorf("idle = %d, want 1", net.Stats.Idle)
	}
}
