package fs

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

type ledgerRecord struct {
	Event      string  `json:"event"`
	Name       string  `json:"name"`
	WorkingDir string  `json:"working_dir"`
	Timestamp  float64 `json:"ts"`
}

func ReadLedger(dir string) ([]AvatarEdge, []string) {
	path := filepath.Join(dir, "delegates", "ledger.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var edges []AvatarEdge
	var childDirs []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec ledgerRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.Event != "avatar" || rec.WorkingDir == "" {
			continue
		}
		edges = append(edges, AvatarEdge{
			Parent:    dir,
			Child:     rec.WorkingDir,
			ChildName: rec.Name,
		})
		childDirs = append(childDirs, rec.WorkingDir)
	}
	return edges, childDirs
}
