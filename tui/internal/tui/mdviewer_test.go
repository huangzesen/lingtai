package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownViewer_EmptyEntries(t *testing.T) {
	m := NewMarkdownViewer(nil, "Test")
	if len(m.entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(m.entries))
	}
}

func TestMarkdownViewer_CursorBounds(t *testing.T) {
	entries := []MarkdownEntry{
		{Label: "a", Group: "G", Content: "hello"},
		{Label: "b", Group: "G", Content: "world"},
	}
	m := NewMarkdownViewer(entries, "Test")
	if m.cursor != 0 {
		t.Errorf("initial cursor = %d, want 0", m.cursor)
	}
}

func TestMarkdownViewer_ContentEntry(t *testing.T) {
	entries := []MarkdownEntry{
		{Label: "test", Group: "G", Content: "# Hello\n\nThis is content."},
	}
	m := NewMarkdownViewer(entries, "Test")
	m.width = 80
	m.height = 24
	right := m.renderRight(60)
	if right == "" {
		t.Error("renderRight returned empty for content entry")
	}
}

func TestMarkdownViewer_PathEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("# Test File\n\nContent here."), 0o644)

	entries := []MarkdownEntry{
		{Label: "test.md", Group: "G", Path: path},
	}
	m := NewMarkdownViewer(entries, "Test")
	m.width = 80
	m.height = 24
	right := m.renderRight(60)
	if right == "" {
		t.Error("renderRight returned empty for path entry")
	}
}

func TestMarkdownViewer_FrontmatterStripped(t *testing.T) {
	entries := []MarkdownEntry{
		{Label: "skill", Group: "G", Content: "---\nname: test\n---\n# Real Content"},
	}
	m := NewMarkdownViewer(entries, "Test")
	m.width = 80
	m.height = 24
	right := m.renderRight(60)
	if right == "" {
		t.Error("renderRight returned empty")
	}
	if strings.Contains(right, "name: test") {
		t.Error("frontmatter was not stripped")
	}
}

func TestMarkdownViewer_GroupRendering(t *testing.T) {
	entries := []MarkdownEntry{
		{Label: "a", Group: "Skills", Content: "x"},
		{Label: "b", Group: "Skills", Content: "y"},
		{Label: "c", Group: "Imported", Content: "z"},
	}
	m := NewMarkdownViewer(entries, "Test")
	m.width = 80
	m.height = 24
	left := m.renderLeft(30)
	if left == "" {
		t.Error("renderLeft returned empty")
	}
	if !strings.Contains(left, "Skills") {
		t.Error("missing Skills group header")
	}
	if !strings.Contains(left, "Imported") {
		t.Error("missing Imported group header")
	}
}
