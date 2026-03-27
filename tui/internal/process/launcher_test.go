package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitProject(t *testing.T) {
	dir := t.TempDir()
	lingtaiDir := filepath.Join(dir, ".lingtai")

	if err := InitProject(lingtaiDir); err != nil {
		t.Fatalf("init: %v", err)
	}

	humanManifest := filepath.Join(lingtaiDir, "human", ".agent.json")
	if _, err := os.Stat(humanManifest); os.IsNotExist(err) {
		t.Error("human/.agent.json not created")
	}

	for _, sub := range []string{"mailbox/inbox", "mailbox/sent", "mailbox/archive"} {
		path := filepath.Join(lingtaiDir, "human", sub)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("%s not created", sub)
		}
	}

	contactsPath := filepath.Join(lingtaiDir, "human", "mailbox", "contacts.json")
	data, err := os.ReadFile(contactsPath)
	if err != nil {
		t.Fatalf("read contacts: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("contacts = %q, want %q", string(data), "[]")
	}
}

func TestProviderToEnvKey(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"minimax", "MINIMAX_API_KEY"},
		{"gemini", "GEMINI_API_KEY"},
		{"custom", "LLM_API_KEY"},
		{"unknown", "LLM_API_KEY"},
		{"openai", "LLM_API_KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := providerToEnvKey(tt.provider)
			if got != tt.want {
				t.Errorf("providerToEnvKey(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}
