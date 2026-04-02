// internal/fs/location.go
package fs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ipinfoResponse is the JSON shape returned by http://ipinfo.io/json.
type ipinfoResponse struct {
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Timezone string `json:"timezone"`
	Loc      string `json:"loc"`
}

// ResolveLocation queries ipinfo.io and returns a populated Location.
func ResolveLocation() (Location, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return Location{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Location{}, fmt.Errorf("ipinfo.io returned %d", resp.StatusCode)
	}

	var info ipinfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return Location{}, err
	}

	return Location{
		City:       info.City,
		Region:     info.Region,
		Country:    info.Country,
		Timezone:   info.Timezone,
		Loc:        info.Loc,
		ResolvedAt: time.Now().Format(time.RFC3339),
	}, nil
}

// LocationStale reports whether loc needs to be refreshed.
// It returns true if ResolvedAt is empty, unparseable, or older than maxAge.
func LocationStale(loc Location, maxAge time.Duration) bool {
	if loc.ResolvedAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, loc.ResolvedAt)
	if err != nil {
		return true
	}
	return time.Since(t) > maxAge
}

// UpdateHumanLocation reads the human's .agent.json, resolves location if stale
// (older than 1 hour), and writes it back atomically. It is a no-op on any failure.
func UpdateHumanLocation(humanDir string) {
	raw, err := ReadAgentRaw(humanDir)
	if err != nil {
		return
	}

	// Extract existing location, if any.
	var current Location
	if locRaw, ok := raw["location"]; ok {
		// Re-encode and decode into Location struct for type safety.
		b, err := json.Marshal(locRaw)
		if err == nil {
			_ = json.Unmarshal(b, &current)
		}
	}

	if !LocationStale(current, time.Hour) {
		return
	}

	resolved, err := ResolveLocation()
	if err != nil {
		return
	}

	raw["location"] = resolved

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return
	}

	manifestPath := filepath.Join(humanDir, ".agent.json")
	tmpPath := manifestPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tmpPath, manifestPath)
}
