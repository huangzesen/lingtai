package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBirthTime_FromCreatedAt(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent-a")
	os.MkdirAll(agentDir, 0o755)

	// Write .agent.json with created_at field
	manifest := map[string]interface{}{
		"agent_name": "agent-a",
		"address":    agentDir,
		"state":      "ACTIVE",
		"created_at": "2026-01-15T10:30:00Z",
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(agentDir, ".agent.json"), data, 0o644)

	got, err := BirthTime(agentDir)
	if err != nil {
		t.Fatalf("BirthTime() error: %v", err)
	}
	want := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("BirthTime() = %v, want %v", got, want)
	}
}

func TestBirthTime_FromStartedAt(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent-b")
	os.MkdirAll(agentDir, 0o755)

	manifest := map[string]interface{}{
		"agent_name": "agent-b",
		"address":    agentDir,
		"state":      "",
		"started_at": "2026-02-20T08:00:00Z",
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(agentDir, ".agent.json"), data, 0o644)

	got, err := BirthTime(agentDir)
	if err != nil {
		t.Fatalf("BirthTime() error: %v", err)
	}
	want := time.Date(2026, 2, 20, 8, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("BirthTime() = %v, want %v", got, want)
	}
}

func TestBirthTime_FallbackToInitJSON(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent-c")
	os.MkdirAll(agentDir, 0o755)

	// .agent.json without timestamp fields
	manifest := map[string]interface{}{
		"agent_name": "agent-c",
		"address":    agentDir,
		"state":      "",
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(agentDir, ".agent.json"), data, 0o644)

	// init.json exists — its modtime is the fallback
	initPath := filepath.Join(agentDir, "init.json")
	os.WriteFile(initPath, []byte(`{"manifest":{}}`), 0o644)

	// Set a known modtime on init.json
	target := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	os.Chtimes(initPath, target, target)

	got, err := BirthTime(agentDir)
	if err != nil {
		t.Fatalf("BirthTime() error: %v", err)
	}
	if !got.Equal(target) {
		t.Errorf("BirthTime() = %v, want %v", got, target)
	}
}

func TestBirthTime_FallbackToAgentJSON(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent-d")
	os.MkdirAll(agentDir, 0o755)

	// .agent.json without timestamp, no init.json
	manifestPath := filepath.Join(agentDir, ".agent.json")
	data, _ := json.Marshal(map[string]interface{}{
		"agent_name": "agent-d",
		"address":    agentDir,
		"state":      "",
	})
	os.WriteFile(manifestPath, data, 0o644)

	target := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	os.Chtimes(manifestPath, target, target)

	got, err := BirthTime(agentDir)
	if err != nil {
		t.Fatalf("BirthTime() error: %v", err)
	}
	if !got.Equal(target) {
		t.Errorf("BirthTime() = %v, want %v", got, target)
	}
}

func TestBirthTime_NoAgent(t *testing.T) {
	dir := t.TempDir()
	_, err := BirthTime(dir)
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
}
