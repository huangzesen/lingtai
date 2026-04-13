package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitProject(t *testing.T) {
	dir := t.TempDir()
	lingtaiDir := filepath.Join(dir, ".lingtai")
	globalDir := filepath.Join(dir, "global")

	if err := InitProject(lingtaiDir, globalDir); err != nil {
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
