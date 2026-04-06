package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anthropics/lingtai-portal/i18n"
	"github.com/anthropics/lingtai-portal/internal/fs"
)

var TopologyMu sync.Mutex

func NewNetworkHandler(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		network, err := fs.BuildNetwork(baseDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if network.Nodes == nil {
			network.Nodes = []fs.AgentNode{}
		}
		if network.AvatarEdges == nil {
			network.AvatarEdges = []fs.AvatarEdge{}
		}
		if network.ContactEdges == nil {
			network.ContactEdges = []fs.ContactEdge{}
		}
		if network.MailEdges == nil {
			network.MailEdges = []fs.MailEdge{}
		}
		network.Lang = i18n.Lang()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(network)
	}
}

// NewTopologyHandler serves the full topology tape as a JSON array.
func NewTopologyHandler(baseDir string) http.HandlerFunc {
	topologyPath := filepath.Join(baseDir, ".portal", "topology.jsonl")

	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(topologyPath)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Write([]byte("[]"))
			return
		}

		// Parse JSONL → JSON array
		var entries []json.RawMessage
		for _, line := range splitLines(data) {
			if len(line) > 0 {
				entries = append(entries, json.RawMessage(line))
			}
		}
		if entries == nil {
			entries = []json.RawMessage{}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(entries)
	}
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// AppendTopology writes one JSONL line: {"t": <unix_ms>, "net": <network>}
// using the current wall-clock time.
func AppendTopology(path string, network fs.Network) {
	AppendTopologyAt(path, network, time.Now().UnixMilli())
}

// AppendTopologyAt writes one JSONL line with an explicit timestamp.
func AppendTopologyAt(path string, network fs.Network, unixMs int64) {
	TopologyMu.Lock()
	defer TopologyMu.Unlock()

	// Normalize nil slices so JSON encodes [] instead of null
	if network.Nodes == nil {
		network.Nodes = []fs.AgentNode{}
	}
	if network.AvatarEdges == nil {
		network.AvatarEdges = []fs.AvatarEdge{}
	}
	if network.ContactEdges == nil {
		network.ContactEdges = []fs.ContactEdge{}
	}
	if network.MailEdges == nil {
		network.MailEdges = []fs.MailEdge{}
	}

	entry := fs.TapeFrame{
		T:   unixMs,
		Net: network,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	line = append(line, '\n')

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		os.MkdirAll(filepath.Dir(path), 0o755)
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
	}
	defer f.Close()
	f.Write(line)
}

// NewProgressHandler serves GET /api/topology/progress.
// Returns reconstruction progress as {"current":N,"total":M} or {} if not rebuilding.
func NewProgressHandler(baseDir string) http.HandlerFunc {
	progressPath := filepath.Join(baseDir, ".portal", "reconstruct.progress")

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		data, err := os.ReadFile(progressPath)
		if err != nil {
			w.Write([]byte("{}"))
			return
		}
		parts := splitProgress(string(data))
		if parts == nil {
			w.Write([]byte("{}"))
			return
		}
		json.NewEncoder(w).Encode(map[string]int{"current": parts[0], "total": parts[1]})
	}
}

func splitProgress(s string) []int {
	for i, c := range s {
		if c == '/' {
			var cur, tot int
			if _, err := fmt.Sscanf(s[:i], "%d", &cur); err != nil {
				return nil
			}
			if _, err := fmt.Sscanf(s[i+1:], "%d", &tot); err != nil {
				return nil
			}
			return []int{cur, tot}
		}
	}
	return nil
}
