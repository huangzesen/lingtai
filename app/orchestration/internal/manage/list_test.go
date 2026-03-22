//go:build !windows

package manage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestScanSpirits_Empty(t *testing.T) {
	dir := t.TempDir()
	spirits := ScanSpirits(dir)
	if len(spirits) != 0 {
		t.Errorf("expected 0 spirits, got %d", len(spirits))
	}
}

func TestScanSpirits_FindsPID(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "myagent")
	os.MkdirAll(agentDir, 0755)

	pidInfo := map[string]interface{}{
		"pid":     os.Getpid(), // use our own PID so it's "alive"
		"port":    8501,
		"config":  "/path/to/config.json",
		"started": "2026-03-20T12:00:00Z",
	}
	data, _ := json.Marshal(pidInfo)
	os.WriteFile(filepath.Join(agentDir, "agent.pid"), data, 0644)

	spirits := ScanSpirits(dir)
	if len(spirits) != 1 {
		t.Fatalf("expected 1 spirit, got %d", len(spirits))
	}
	if spirits[0].Name != "myagent" {
		t.Errorf("name: got %q, want %q", spirits[0].Name, "myagent")
	}
	if !spirits[0].Alive {
		t.Error("spirit should be alive (our own PID)")
	}
}

func TestScanSpirits_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "deadagent")
	os.MkdirAll(agentDir, 0755)

	pidInfo := map[string]interface{}{
		"pid":     999999999, // almost certainly not a real PID
		"port":    8501,
		"started": "2026-03-20T12:00:00Z",
	}
	data, _ := json.Marshal(pidInfo)
	os.WriteFile(filepath.Join(agentDir, "agent.pid"), data, 0644)

	spirits := ScanSpirits(dir)
	if len(spirits) != 1 {
		t.Fatalf("expected 1 spirit, got %d", len(spirits))
	}
	if spirits[0].Alive {
		t.Error("spirit with PID 999999999 should be dead")
	}
}
