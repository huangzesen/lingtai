package fs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupPortalTestNetwork(t *testing.T) string {
	t.Helper()
	base := t.TempDir()

	// alice: active agent, has a ledger entry for bob (relative path)
	aliceDir := filepath.Join(base, "alice")
	os.MkdirAll(filepath.Join(aliceDir, "delegates"), 0o755)
	os.MkdirAll(filepath.Join(aliceDir, "mailbox", "inbox"), 0o755)
	writeAgentManifest(t, aliceDir, "alice", false)

	// ledger entry uses relative path — ReadLedger will resolve to absolute
	ledger := `{"event":"avatar","name":"bob","working_dir":"bob","ts":1000}`
	os.WriteFile(filepath.Join(aliceDir, "delegates", "ledger.jsonl"), []byte(ledger+"\n"), 0o644)

	// bob: discovered by DiscoverAgents (relative address from .agent.json)
	bobDir := filepath.Join(base, "bob")
	os.MkdirAll(filepath.Join(bobDir, "mailbox", "inbox"), 0o755)
	writeAgentManifest(t, bobDir, "bob", false)
	writeHeartbeat(t, bobDir)

	// human: discovered by DiscoverAgents (relative address)
	humanDir := filepath.Join(base, "human")
	os.MkdirAll(filepath.Join(humanDir, "mailbox", "inbox"), 0o755)
	writeAgentManifest(t, humanDir, "human", true)

	return base
}

func writeHeartbeat(t *testing.T, dir string) {
	t.Helper()
	content := time.Now().Format(time.RFC3339)
	os.WriteFile(filepath.Join(dir, ".agent.heartbeat"), []byte(content), 0o644)
}

func TestBuildNetwork_Portal(t *testing.T) {
	base := setupPortalTestNetwork(t)

	net, err := BuildNetwork(base)
	if err != nil {
		t.Fatalf("build network: %v", err)
	}

	if len(net.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3", len(net.Nodes))
	}
}

func TestBuildNetwork_AllAddressesRelative(t *testing.T) {
	base := setupPortalTestNetwork(t)

	net, err := BuildNetwork(base)
	if err != nil {
		t.Fatalf("build network: %v", err)
	}

	for _, n := range net.Nodes {
		if len(n.Address) > 0 && n.Address[0] == '/' {
			t.Errorf("node %s has absolute address: %s", n.AgentName, n.Address)
		}
	}
}

// Regression test: ledger entries using relative paths must be relativized
// so they don't create duplicate nodes alongside DiscoverAgents entries.
func TestBuildNetwork_NoDuplicateNodesFromLedger(t *testing.T) {
	base := setupPortalTestNetwork(t)

	net, err := BuildNetwork(base)
	if err != nil {
		t.Fatalf("build network: %v", err)
	}

	// Count nodes by address — no duplicates allowed
	seen := make(map[string]bool)
	for _, n := range net.Nodes {
		if seen[n.Address] {
			t.Errorf("duplicate node address: %s", n.Address)
		}
		seen[n.Address] = true
	}
}
