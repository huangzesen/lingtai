package migrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anthropics/lingtai-tui/internal/fs"
)

// migrateSessionBackfill rebuilds session.jsonl from existing mail, events,
// and inquiries if it is missing or incomplete, then dumps all completed hours
// into brief history markdown files.
func migrateSessionBackfill(lingtaiDir string) error {
	humanDir := filepath.Join(lingtaiDir, "human")
	if _, err := os.Stat(humanDir); err != nil {
		return nil // no human dir — nothing to backfill
	}

	// Find the orchestrator directory (first agent with admin.karma=true,
	// or just the first non-human agent).
	orchDir := findOrchestrator(lingtaiDir)
	if orchDir == "" {
		return nil // no orchestrator — nothing to backfill
	}

	// Read orchestrator name from .agent.json
	orchName := ""
	if node, err := fs.ReadAgent(orchDir); err == nil {
		orchName = node.AgentName
	}

	// Read human address
	humanAddr := "human"
	if node, err := fs.ReadAgent(humanDir); err == nil && node.Address != "" {
		humanAddr = node.Address
	}

	// Project path is the parent of .lingtai/
	projectPath := filepath.Dir(lingtaiDir)

	// Create a fresh session cache — this will load any existing session.jsonl
	sc := fs.NewSessionCache(humanDir, projectPath)

	// Check if events/inquiries have already been ingested by comparing
	// the session entry count against source sizes. If session.jsonl already
	// has entries and the offsets were set (i.e., TUI already ran with the
	// new code), skip backfill.
	existingCount := sc.Len()

	// Ingest everything from offset 0 (the session cache starts fresh,
	// mailSeen is populated from existing session.jsonl entries, so
	// already-ingested mail is deduplicated automatically).
	cache := fs.NewMailCache(humanDir).Refresh()
	sc.IngestMail(cache, humanAddr, orchDir, orchName)
	sc.IngestEvents(orchDir)
	sc.IngestInquiries(orchDir)

	newCount := sc.Len()
	if newCount == existingCount {
		// Nothing new to backfill — session.jsonl was already complete
		return nil
	}

	// Force dump ALL completed hours (bypass the runtime 24h cap).
	// The session cache's append() already dumped hours for new entries,
	// but that was capped. Do a full dump here.
	globalDir := globalTUIDir()
	if globalDir == "" {
		return nil
	}
	hash := fs.ProjectHash(projectPath)
	histDir := filepath.Join(globalDir, "brief", "projects", hash, "history")
	fs.DumpAllHours(sc.Entries(), histDir)

	fmt.Printf("  session backfill: %d entries (%d new), history dumped to %s\n",
		newCount, newCount-existingCount, histDir)
	return nil
}

// findOrchestrator returns the working directory of the orchestrator agent.
// Looks for the first agent with admin.karma in its init.json, falling back
// to the first non-human agent directory.
func findOrchestrator(lingtaiDir string) string {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return ""
	}
	var fallback string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "human" || e.Name()[0] == '.' {
			continue
		}
		dir := filepath.Join(lingtaiDir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, ".agent.json")); err != nil {
			continue
		}
		if fallback == "" {
			fallback = dir
		}
		// Check init.json for admin.karma
		if manifest, err := fs.ReadInitManifest(dir); err == nil {
			if admin, ok := manifest["admin"].(map[string]interface{}); ok {
				if karma, ok := admin["karma"].(bool); ok && karma {
					return dir
				}
			}
		}
	}
	return fallback
}
