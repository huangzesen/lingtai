package api

import (
	"encoding/json"
	"net/http"

	"github.com/anthropics/lingtai-tui/internal/fs"
)

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
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(network)
	}
}
