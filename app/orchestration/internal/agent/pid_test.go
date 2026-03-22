package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.pid")

	err := WritePIDFile(path, 12345, 8501, "/path/to/config.json")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var info PIDInfo
	json.Unmarshal(data, &info)
	if info.PID != 12345 {
		t.Errorf("PID: got %d, want %d", info.PID, 12345)
	}
	if info.Port != 8501 {
		t.Errorf("Port: got %d, want %d", info.Port, 8501)
	}
}

func TestReadPIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.pid")

	WritePIDFile(path, 99999, 8501, "/cfg")

	info, err := ReadPIDFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.PID != 99999 {
		t.Errorf("got PID %d", info.PID)
	}
}

func TestRemovePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.pid")

	WritePIDFile(path, 1, 1, "")
	RemovePIDFile(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("PID file should be deleted")
	}
}
