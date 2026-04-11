package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DetectOrchestrators scans baseDir for .agent.json files with admin privileges.
// Returns the directory names (not full paths) of orchestrators found.
func DetectOrchestrators(baseDir string) []string {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}
	var orchestrators []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(baseDir, entry.Name(), ".agent.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest map[string]interface{}
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		if IsOrchestrator(manifest) {
			orchestrators = append(orchestrators, entry.Name())
		}
	}
	return orchestrators
}

// PropagateOrchestratorConfig reads the orchestrator's init.json and copies
// its LLM config and capabilities to every other agent in the .lingtai/
// network. Admin privileges and addons are stripped from non-orchestrators.
// Skips directories that are not agents (no init.json) and "human".
func PropagateOrchestratorConfig(baseDir, orchDir string) error {
	orchInitPath := filepath.Join(orchDir, "init.json")
	orchData, err := os.ReadFile(orchInitPath)
	if err != nil {
		return err
	}
	var orchInit map[string]interface{}
	if err := json.Unmarshal(orchData, &orchInit); err != nil {
		return err
	}
	orchManifest, _ := orchInit["manifest"].(map[string]interface{})
	if orchManifest == nil {
		return nil
	}
	orchLLM, _ := orchManifest["llm"].(map[string]interface{})
	if orchLLM == nil {
		return nil
	}
	orchCaps, _ := orchManifest["capabilities"].(map[string]interface{})
	orchEnvFile, _ := orchInit["env_file"].(string)

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "human" {
			continue
		}
		agentDir := filepath.Join(baseDir, entry.Name())
		if agentDir == orchDir {
			continue
		}
		initPath := filepath.Join(agentDir, "init.json")
		data, err := os.ReadFile(initPath)
		if err != nil {
			continue
		}
		var initJSON map[string]interface{}
		if err := json.Unmarshal(data, &initJSON); err != nil {
			continue
		}
		manifest, _ := initJSON["manifest"].(map[string]interface{})
		if manifest == nil {
			continue
		}

		// Replace LLM config
		manifest["llm"] = orchLLM

		// Replace capabilities — strip admin and addons
		if orchCaps != nil {
			capsCopy := make(map[string]interface{}, len(orchCaps))
			for k, v := range orchCaps {
				capsCopy[k] = v
			}
			manifest["capabilities"] = capsCopy
		}
		manifest["admin"] = map[string]interface{}{"karma": false, "nirvana": false}
		delete(initJSON, "addons")

		if orchEnvFile != "" {
			initJSON["env_file"] = orchEnvFile
		}

		out, err := json.MarshalIndent(initJSON, "", "  ")
		if err != nil {
			continue
		}
		os.WriteFile(initPath, out, 0o644)
	}
	return nil
}

// IsOrchestrator checks if a manifest has admin with at least one truthy value.
// admin must be a map[string]interface{} (not nil, not absent) with at least one
// value that is true (bool).
func IsOrchestrator(manifest map[string]interface{}) bool {
	adminRaw, ok := manifest["admin"]
	if !ok || adminRaw == nil {
		return false
	}
	adminMap, ok := adminRaw.(map[string]interface{})
	if !ok {
		return false
	}
	for _, v := range adminMap {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	return false
}
