package config

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/anthropics/lingtai-tui/internal/fs"
)

// registryEntry is one line in registry.jsonl.
type registryEntry struct {
	Path      string `json:"path"`
	BriefHash string `json:"brief_hash,omitempty"`
}

// Register adds a project path to ~/.lingtai-tui/registry.jsonl (deduplicating).
// projectDir is the parent of .lingtai/ (e.g. /home/user/my-project).
//
// Note: the read-check-append cycle is not atomic. Concurrent TUI launches could
// produce duplicate entries. This is benign for a single-user tool; LoadAndPrune
// will return duplicates but they won't cause errors.
func Register(globalDir, projectDir string) error {
	regPath := filepath.Join(globalDir, "registry.jsonl")
	hash := fs.ProjectHash(projectDir)

	// Read existing entries to deduplicate
	existing := readRegistry(regPath)
	for _, e := range existing {
		if e.Path == projectDir {
			// Backfill brief_hash if missing
			if e.BriefHash == "" {
				rewriteRegistryEntries(regPath, existing, projectDir, hash)
			}
			ensureBriefMeta(globalDir, hash, projectDir)
			return nil
		}
	}

	// Append
	f, err := os.OpenFile(regPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(registryEntry{Path: projectDir, BriefHash: hash})
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	if err != nil {
		return err
	}

	ensureBriefMeta(globalDir, hash, projectDir)
	return nil
}

// LoadAndPrune reads registry.jsonl, removes entries whose .lingtai/ no longer
// exists, rewrites the file, and returns the surviving paths.
func LoadAndPrune(globalDir string) []string {
	regPath := filepath.Join(globalDir, "registry.jsonl")
	entries := readRegistry(regPath)
	if len(entries) == 0 {
		return nil
	}

	var alive []string
	for _, e := range entries {
		lingtaiDir := filepath.Join(e.Path, ".lingtai")
		if info, err := os.Stat(lingtaiDir); err == nil && info.IsDir() {
			alive = append(alive, e.Path)
		}
	}

	// Rewrite if anything was pruned
	if len(alive) < len(entries) {
		rewriteRegistry(regPath, alive)
	}

	return alive
}

func readRegistry(path string) []registryEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []registryEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e registryEntry
		if json.Unmarshal(line, &e) == nil && e.Path != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

func rewriteRegistry(path string, paths []string) {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	for _, p := range paths {
		line, _ := json.Marshal(registryEntry{Path: p, BriefHash: fs.ProjectHash(p)})
		f.Write(append(line, '\n'))
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return
	}
	os.Rename(tmp, path)
}

// rewriteRegistryEntries rewrites registry.jsonl, backfilling brief_hash for
// the given projectDir.
func rewriteRegistryEntries(path string, entries []registryEntry, projectDir, hash string) {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Path == projectDir {
			e.BriefHash = hash
		}
		if e.BriefHash == "" {
			e.BriefHash = fs.ProjectHash(e.Path)
		}
		line, _ := json.Marshal(e)
		f.Write(append(line, '\n'))
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return
	}
	os.Rename(tmp, path)
}

// briefMeta is written to ~/.lingtai-tui/brief/projects/<hash>/meta.json
// so the brief folder can be traced back to its source project.
type briefMeta struct {
	ProjectPath string `json:"project_path"`
}

// ensureBriefMeta writes meta.json into the brief project directory if it
// doesn't already exist.
func ensureBriefMeta(globalDir, hash, projectDir string) {
	briefDir := filepath.Join(globalDir, "brief", "projects", hash)
	metaPath := filepath.Join(briefDir, "meta.json")
	if _, err := os.Stat(metaPath); err == nil {
		return // already exists
	}
	os.MkdirAll(briefDir, 0o755)
	data, _ := json.Marshal(briefMeta{ProjectPath: projectDir})
	os.WriteFile(metaPath, data, 0o644)
}
