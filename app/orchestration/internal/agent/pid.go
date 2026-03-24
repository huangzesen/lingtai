package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type PIDInfo struct {
	PID     int    `json:"pid"`
	Port    int    `json:"port"`
	Config  string `json:"config"`
	Started string `json:"started"`
}

func WritePIDFile(path string, pid, port int, configPath string) error {
	os.MkdirAll(filepath.Dir(path), 0755)
	info := PIDInfo{
		PID:     pid,
		Port:    port,
		Config:  configPath,
		Started: time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(path, data, 0644)
}

func ReadPIDFile(path string) (*PIDInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info PIDInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("invalid PID file: %w", err)
	}
	return &info, nil
}

func RemovePIDFile(path string) {
	os.Remove(path)
}
