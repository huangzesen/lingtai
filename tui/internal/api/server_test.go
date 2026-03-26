package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/lingtai-tui/internal/fs"
)

func TestNetworkHandler(t *testing.T) {
	base := t.TempDir()
	handler := NewNetworkHandler(base)

	req := httptest.NewRequest("GET", "/api/network", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var net fs.Network
	if err := json.Unmarshal(w.Body.Bytes(), &net); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}
