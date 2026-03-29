// internal/fs/location_test.go
package fs

import (
	"testing"
	"time"
)

func TestLocationStale_Empty(t *testing.T) {
	loc := Location{}
	if !LocationStale(loc, time.Hour) {
		t.Error("empty Location should be stale")
	}
}

func TestLocationStale_Recent(t *testing.T) {
	loc := Location{
		ResolvedAt: time.Now().Format(time.RFC3339),
	}
	if LocationStale(loc, time.Hour) {
		t.Error("Location resolved just now should NOT be stale")
	}
}

func TestLocationStale_Old(t *testing.T) {
	loc := Location{
		ResolvedAt: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
	}
	if !LocationStale(loc, time.Hour) {
		t.Error("Location resolved 2h ago should be stale with 1h maxAge")
	}
}
