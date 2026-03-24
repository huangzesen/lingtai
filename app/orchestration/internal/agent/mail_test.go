package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAgentJSON(t *testing.T, dir string) {
	t.Helper()
	manifest := map[string]interface{}{"address": dir, "agent_name": "test"}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(dir, ".agent.json"), data, 0644)
}

func writeFreshHeartbeat(t *testing.T, dir string) {
	t.Helper()
	ts := fmt.Sprintf("%.6f", float64(time.Now().UnixNano())/1e9)
	os.WriteFile(filepath.Join(dir, ".agent.heartbeat"), []byte(ts), 0644)
}

func TestMailWriter_Send(t *testing.T) {
	dir := t.TempDir()
	writeAgentJSON(t, dir)
	writeFreshHeartbeat(t, dir)

	inboxDir := filepath.Join(dir, "mailbox", "inbox")
	os.MkdirAll(inboxDir, 0755)

	writer := NewMailWriter(dir, "mailbox")
	err := writer.Send(map[string]interface{}{
		"from":    "human@local",
		"message": "hello agent",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check that a message.json was created in a subdirectory of inbox
	entries, _ := os.ReadDir(inboxDir)
	if len(entries) == 0 {
		t.Fatal("no message directory created in inbox")
	}

	msgPath := filepath.Join(inboxDir, entries[0].Name(), "message.json")
	data, err := os.ReadFile(msgPath)
	if err != nil {
		t.Fatalf("message.json not found: %v", err)
	}

	var msg map[string]interface{}
	if json.Unmarshal(data, &msg) != nil {
		t.Fatal("invalid JSON in message.json")
	}
	if msg["message"] != "hello agent" {
		t.Errorf("got message %q, want %q", msg["message"], "hello agent")
	}
}

func TestMailWriter_NoAgentJSON(t *testing.T) {
	dir := t.TempDir()
	writer := NewMailWriter(dir, "mailbox")
	err := writer.Send(map[string]interface{}{"message": "test"})
	if err == nil {
		t.Error("expected error when .agent.json missing")
	}
}

func TestMailWriter_StaleHeartbeat(t *testing.T) {
	dir := t.TempDir()
	writeAgentJSON(t, dir)

	// Write a stale heartbeat (10 seconds ago)
	ts := fmt.Sprintf("%.6f", float64(time.Now().Add(-10*time.Second).UnixNano())/1e9)
	os.WriteFile(filepath.Join(dir, ".agent.heartbeat"), []byte(ts), 0644)

	writer := NewMailWriter(dir, "mailbox")
	err := writer.Send(map[string]interface{}{"message": "test"})
	if err == nil {
		t.Error("expected error for stale heartbeat")
	}
}

func TestMailPoller_NewMessage(t *testing.T) {
	dir := t.TempDir()
	inboxDir := filepath.Join(dir, "mailbox", "inbox")
	os.MkdirAll(inboxDir, 0755)

	received := make(chan map[string]interface{}, 1)
	poller := NewMailPoller(inboxDir, func(msg map[string]interface{}) {
		received <- msg
	})
	poller.Start()
	defer poller.Stop()

	// Write a new message after poller starts
	time.Sleep(100 * time.Millisecond)
	msgDir := filepath.Join(inboxDir, "test-msg-001")
	os.MkdirAll(msgDir, 0755)
	payload, _ := json.Marshal(map[string]interface{}{
		"from":    "agent@local",
		"message": "hello human",
	})
	os.WriteFile(filepath.Join(msgDir, "message.json"), payload, 0644)

	select {
	case msg := <-received:
		if msg["message"] != "hello human" {
			t.Errorf("got %q, want %q", msg["message"], "hello human")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for polled message")
	}
}

func TestMailPoller_IgnoresExisting(t *testing.T) {
	dir := t.TempDir()
	inboxDir := filepath.Join(dir, "mailbox", "inbox")
	os.MkdirAll(inboxDir, 0755)

	// Pre-existing message before poller starts
	preDir := filepath.Join(inboxDir, "pre-existing")
	os.MkdirAll(preDir, 0755)
	payload, _ := json.Marshal(map[string]interface{}{"message": "old"})
	os.WriteFile(filepath.Join(preDir, "message.json"), payload, 0644)

	received := make(chan map[string]interface{}, 1)
	poller := NewMailPoller(inboxDir, func(msg map[string]interface{}) {
		received <- msg
	})
	poller.Start()
	defer poller.Stop()

	select {
	case <-received:
		t.Error("pre-existing message should not be delivered")
	case <-time.After(1 * time.Second):
		// Expected: no message delivered
	}
}

func TestSetupHumanWorkdir(t *testing.T) {
	dir := t.TempDir()
	workdir, err := SetupHumanWorkdir(dir, "human01", "Alice", "en")
	if err != nil {
		t.Fatal(err)
	}

	// Check .agent.json
	data, err := os.ReadFile(filepath.Join(workdir, ".agent.json"))
	if err != nil {
		t.Fatal("missing .agent.json")
	}
	var manifest map[string]interface{}
	json.Unmarshal(data, &manifest)
	if manifest["agent_name"] != "Alice" {
		t.Errorf("got agent_name %q, want %q", manifest["agent_name"], "Alice")
	}
	if manifest["admin"] != nil {
		t.Errorf("expected admin to be null, got %v", manifest["admin"])
	}
	if manifest["address"] != workdir {
		t.Errorf("got address %q, want %q", manifest["address"], workdir)
	}
	if _, has := manifest["agent_id"]; has {
		t.Error("manifest should not contain agent_id")
	}

	// Check inbox directory exists
	if _, err := os.Stat(filepath.Join(workdir, "mailbox", "inbox")); err != nil {
		t.Error("inbox directory not created")
	}
}

func TestWaitForAgentJSON(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "test-agent")
	os.MkdirAll(agentDir, 0755)

	// Write .agent.json after a short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		manifest := map[string]interface{}{"address": agentDir, "agent_name": "test"}
		data, _ := json.MarshalIndent(manifest, "", "  ")
		os.WriteFile(filepath.Join(agentDir, ".agent.json"), data, 0644)
	}()

	err := WaitForAgentJSON(agentDir, 5*time.Second)
	if err != nil {
		t.Fatalf("expected .agent.json to be found: %v", err)
	}
}

func TestWaitForAgentJSON_Timeout(t *testing.T) {
	dir := t.TempDir()
	err := WaitForAgentJSON(dir, 300*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}
