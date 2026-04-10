// tui/internal/fs/session_dump_test.go
package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectHash(t *testing.T) {
	hash := projectHash("/home/user/myproject")
	if len(hash) != 12 {
		t.Fatalf("hash length = %d, want 12", len(hash))
	}
	if hash != projectHash("/home/user/myproject") {
		t.Fatal("hash is not deterministic")
	}
	if hash == projectHash("/home/user/other") {
		t.Fatal("different paths should produce different hashes")
	}
}

func TestBriefHistoryDir(t *testing.T) {
	hash := "abcdef012345"
	dir := briefHistoryDir(hash)
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".lingtai-tui", "brief", hash, "history")
	if dir != want {
		t.Fatalf("briefHistoryDir = %q, want %q", dir, want)
	}
}
