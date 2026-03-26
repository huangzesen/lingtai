// internal/fs/agent.go
package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// agentManifest is the raw JSON shape of .agent.json.
type agentManifest struct {
	AgentName    string           `json:"agent_name"`
	Address      string           `json:"address"`
	State        string           `json:"state"`
	Admin        *json.RawMessage `json:"admin,omitempty"`
	Capabilities []string         `json:"capabilities"`
}

// ReadAgent reads .agent.json from dir and returns an AgentNode.
func ReadAgent(dir string) (AgentNode, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".agent.json"))
	if err != nil {
		return AgentNode{}, fmt.Errorf("read manifest: %w", err)
	}

	var m agentManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return AgentNode{}, fmt.Errorf("parse manifest: %w", err)
	}

	// is_human: true when admin is JSON null or key is absent entirely
	isHuman := m.Admin == nil || string(*m.Admin) == "null"

	return AgentNode{
		Address:      m.Address,
		AgentName:    m.AgentName,
		State:        m.State,
		IsHuman:      isHuman,
		Capabilities: m.Capabilities,
		WorkingDir:   dir,
	}, nil
}

// DiscoverAgents scans baseDir for subdirectories with .agent.json manifests.
func DiscoverAgents(baseDir string) ([]AgentNode, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("read base dir: %w", err)
	}

	var nodes []AgentNode
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentDir := filepath.Join(baseDir, entry.Name())
		node, err := ReadAgent(agentDir)
		if err != nil {
			continue // skip non-agent dirs
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}
