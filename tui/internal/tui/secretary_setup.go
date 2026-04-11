package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/secretary"
)

// setupSecretary creates the secretary agent by cloning the orchestrator's
// init.json with modifications: no avatar capability, soul delay 9999999,
// secretary recipe files (covenant, procedures, comment), and the briefing
// skill symlinked.
func setupSecretary(baseDir, globalDir, orchDirName string) error {
	// Read orchestrator's init.json
	orchInitPath := filepath.Join(baseDir, orchDirName, "init.json")
	data, err := os.ReadFile(orchInitPath)
	if err != nil {
		return fmt.Errorf("read orchestrator init.json: %w", err)
	}
	var initJSON map[string]interface{}
	if err := json.Unmarshal(data, &initJSON); err != nil {
		return fmt.Errorf("parse orchestrator init.json: %w", err)
	}

	// Populate secretary recipe assets on disk
	recipeDir, err := secretary.RecipeDir(globalDir)
	if err != nil {
		return fmt.Errorf("populate secretary recipe: %w", err)
	}

	// Modify manifest
	manifest, _ := initJSON["manifest"].(map[string]interface{})
	if manifest == nil {
		return fmt.Errorf("orchestrator init.json has no manifest")
	}

	manifest["agent_name"] = "secretary"
	manifest["soul"] = map[string]interface{}{"delay": 9999999}
	manifest["admin"] = map[string]interface{}{"karma": false, "nirvana": false}

	// Build secretary capabilities from scratch. The secretary has a fixed set
	// of capabilities regardless of what the orchestrator has. We only inherit
	// per-capability config (e.g. provider overrides) from the orchestrator
	// where the capability exists in both.
	secretaryCaps := map[string]interface{}{
		"file": map[string]interface{}{}, "bash": map[string]interface{}{},
		"email": map[string]interface{}{}, "psyche": map[string]interface{}{},
		"library": map[string]interface{}{"library_limit": 100},
		"skills": map[string]interface{}{},
		"web_search": map[string]interface{}{}, "web_read": map[string]interface{}{},
		"daemon": map[string]interface{}{},
	}
	// Inherit per-capability config from orchestrator where applicable
	if orchCaps, ok := manifest["capabilities"].(map[string]interface{}); ok {
		for name, cfg := range orchCaps {
			if _, needed := secretaryCaps[name]; needed {
				secretaryCaps[name] = cfg
			}
		}
		// Ensure library_limit is always raised for secretary
		if lib, ok := secretaryCaps["library"].(map[string]interface{}); ok {
			lib["library_limit"] = 100
		}
	}
	manifest["capabilities"] = secretaryCaps

	// Set secretary recipe files (no procedures override — inherits system-wide)
	initJSON["covenant_file"] = filepath.Join(recipeDir, "covenant.md")
	initJSON["comment_file"] = filepath.Join(recipeDir, "comment.md")

	// No brief for the secretary itself
	delete(initJSON, "brief_file")

	// No addons — secretary doesn't need external email
	delete(initJSON, "addons")

	// Clear init.json prompt — the greet is delivered via .prompt file only
	// (setting both would cause the kernel to deliver it twice)
	initJSON["prompt"] = ""

	// Create the standard lingtai project structure:
	// ~/.lingtai-tui/secretary/.lingtai/secretary/  (agent working dir)
	// ~/.lingtai-tui/secretary/.lingtai/human/      (human mailbox for TUI)
	agentDir := secretary.AgentDir(globalDir)
	lingtaiDir := secretary.LingtaiDir(globalDir)
	humanDir := filepath.Join(lingtaiDir, "human")

	for _, sub := range []string{
		"system",
		"logs",
		"mailbox/inbox",
		"mailbox/sent",
		"mailbox/archive",
	} {
		if err := os.MkdirAll(filepath.Join(agentDir, sub), 0o755); err != nil {
			return fmt.Errorf("create secretary %s dir: %w", sub, err)
		}
	}
	// Create human mailbox
	for _, sub := range []string{"mailbox/inbox", "mailbox/sent", "mailbox/archive"} {
		if err := os.MkdirAll(filepath.Join(humanDir, sub), 0o755); err != nil {
			return fmt.Errorf("create human %s dir: %w", sub, err)
		}
	}

	// Write .agent.json manifest (required by DiscoverAgents and mail system)
	agentManifest := map[string]interface{}{
		"agent_name": "secretary",
		"address":    "secretary",
		"state":      "",
		"admin":      map[string]interface{}{"karma": false, "nirvana": false},
	}
	mdata, err := json.MarshalIndent(agentManifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal secretary .agent.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, ".agent.json"), mdata, 0o644); err != nil {
		return fmt.Errorf("write secretary .agent.json: %w", err)
	}

	// Write human .agent.json
	humanManifest := map[string]interface{}{
		"agent_name": "human",
		"address":    "human",
	}
	hmdata, err := json.MarshalIndent(humanManifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal human .agent.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(humanDir, ".agent.json"), hmdata, 0o644); err != nil {
		return fmt.Errorf("write human .agent.json: %w", err)
	}

	out, err := json.MarshalIndent(initJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal secretary init.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "init.json"), out, 0o644); err != nil {
		return fmt.Errorf("write secretary init.json: %w", err)
	}

	// Symlink briefing skill into the network-level .skills/ dir
	// (.lingtai/.skills/ — sibling to agent dirs, not inside the agent dir)
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create secretary skills dir: %w", err)
	}
	linkName := filepath.Join(skillsDir, "secretary-briefing")
	// Remove old symlink if exists (idempotent)
	os.Remove(linkName)
	skillSrc := secretary.SkillDir(globalDir)
	if err := os.Symlink(skillSrc, linkName); err != nil {
		return fmt.Errorf("symlink briefing skill: %w", err)
	}

	// Write .prompt file (greet content) — this is what the kernel reads as the first message
	if err := fs.WritePrompt(agentDir, secretary.GreetContent()); err != nil {
		return fmt.Errorf("write secretary .prompt: %w", err)
	}

	return nil
}
