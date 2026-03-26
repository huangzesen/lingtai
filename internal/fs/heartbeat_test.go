package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsAlive_FreshHeartbeat(t *testing.T) {
	dir := t.TempDir()
	ts := fmt.Sprintf("%f", float64(time.Now().Unix()))
	os.WriteFile(filepath.Join(dir, ".agent.heartbeat"), []byte(ts), 0o644)
	if !IsAlive(dir, 2.0) {
		t.Error("expected alive for fresh heartbeat")
	}
}

func TestIsAlive_StaleHeartbeat(t *testing.T) {
	dir := t.TempDir()
	ts := fmt.Sprintf("%f", float64(time.Now().Add(-5*time.Second).Unix()))
	os.WriteFile(filepath.Join(dir, ".agent.heartbeat"), []byte(ts), 0o644)
	if IsAlive(dir, 2.0) {
		t.Error("expected dead for stale heartbeat")
	}
}

func TestIsAlive_MissingFile(t *testing.T) {
	dir := t.TempDir()
	if IsAlive(dir, 2.0) {
		t.Error("expected dead when heartbeat file missing")
	}
}

func TestIsAlive_HumanAlwaysAlive(t *testing.T) {
	if !IsAliveHuman() {
		t.Error("human agents are always alive")
	}
}
