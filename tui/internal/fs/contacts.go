package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type contactRecord struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Note    string `json:"note"`
}

func ReadContacts(dir string) []ContactEdge {
	path := filepath.Join(dir, "mailbox", "contacts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var records []contactRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil
	}
	baseDir := filepath.Dir(dir) // .lingtai/ directory
	var edges []ContactEdge
	for _, r := range records {
		edges = append(edges, ContactEdge{
			Owner:  dir,
			Target: ResolveAddress(r.Address, baseDir),
			Name:   r.Name,
		})
	}
	return edges
}
