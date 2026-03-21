package combo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Combo is a named snapshot of provider/model/config settings.
type Combo struct {
	Name   string                 `json:"name"`
	Model  map[string]interface{} `json:"model"`
	Config map[string]interface{} `json:"config"`
	Env    map[string]string      `json:"env"`
}

// Dir returns the combos directory (~/.lingtai/combos/).
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lingtai", "combos")
}

// List returns all saved combos, sorted by name.
func List() ([]Combo, error) {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var combos []Combo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var c Combo
		if json.Unmarshal(data, &c) == nil && c.Name != "" {
			combos = append(combos, c)
		}
	}
	sort.Slice(combos, func(i, j int) bool { return combos[i].Name < combos[j].Name })
	return combos, nil
}

// Save writes a combo to ~/.lingtai/combos/<name>.json with mode 0600.
func Save(c Combo) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create combos dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, c.Name+".json")
	return os.WriteFile(path, data, 0600)
}

// Load reads a combo by name.
func Load(name string) (*Combo, error) {
	path := filepath.Join(Dir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Combo
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
