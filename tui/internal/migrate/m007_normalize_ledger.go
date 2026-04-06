package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// migrateNormalizeLedger rewrites legacy avatar ledger formats into the
// current canonical format: {"ts":..., "event":"avatar", "name":..., "working_dir":..., ...}
//
// Known legacy formats:
//   - {agent_name, address, type, role, spawned_at}
//   - {name, dir, type, role, spawned_at} (with # comment header lines)
//   - {avatar_name, dir, type, role, spawn_time}
//   - {id, name, parent, spawned_at, type, perspective}
func migrateNormalizeLedger(lingtaiDir string) error {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ledgerPath := filepath.Join(lingtaiDir, entry.Name(), "delegates", "ledger.jsonl")
		if _, err := os.Stat(ledgerPath); os.IsNotExist(err) {
			continue
		}
		if err := normalizeLedger(ledgerPath); err != nil {
			return err
		}
	}
	return nil
}

func normalizeLedger(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // skip unreadable
	}

	lines := strings.Split(string(data), "\n")
	var normalized []string
	changed := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			changed = true
			continue
		}

		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			normalized = append(normalized, line) // keep unparseable lines
			continue
		}

		// Already in current format?
		if _, hasEvent := record["event"]; hasEvent {
			if _, hasWD := record["working_dir"]; hasWD {
				normalized = append(normalized, line)
				continue
			}
		}

		// Normalize: extract name and working_dir from various legacy keys
		name := firstString(record, "name", "agent_name", "avatar_name")
		workingDir := firstString(record, "working_dir", "address", "dir", "id")

		if name == "" || workingDir == "" {
			normalized = append(normalized, line) // can't normalize, keep as-is
			continue
		}

		// Parse timestamp from various formats
		ts := 0.0
		if v, ok := record["ts"].(float64); ok {
			ts = v
		} else {
			for _, key := range []string{"spawned_at", "spawn_time"} {
				if s, ok := record[key].(string); ok && s != "" {
					if t, err := time.Parse(time.RFC3339, s); err == nil {
						ts = float64(t.Unix())
					}
					break
				}
			}
		}

		// Build normalized record
		norm := map[string]interface{}{
			"ts":          ts,
			"event":       "avatar",
			"name":        name,
			"working_dir": workingDir,
		}
		// Preserve optional fields
		if v, ok := record["type"]; ok {
			norm["type"] = v
		}
		if v, ok := record["mission"]; ok {
			norm["mission"] = v
		} else if v, ok := record["role"]; ok {
			norm["mission"] = v
		} else if v, ok := record["perspective"]; ok {
			norm["mission"] = v
		}
		if v, ok := record["pid"]; ok {
			norm["pid"] = v
		}

		out, err := json.Marshal(norm)
		if err != nil {
			normalized = append(normalized, line)
			continue
		}
		normalized = append(normalized, string(out))
		changed = true
	}

	if !changed {
		return nil
	}

	// Atomic write
	content := strings.Join(normalized, "\n") + "\n"
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
