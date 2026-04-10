// tui/internal/fs/session_dump_test.go
package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRenderMailEntry(t *testing.T) {
	e := SessionEntry{
		Ts:      "2026-04-10T14:02:00Z",
		Type:    "mail",
		From:    "human",
		To:      "agent",
		Subject: "hello",
		Body:    "Hi there",
	}
	got := renderMailEntry(e)
	want := "**human** 14:02 → agent │ Re: hello\nHi there\n"
	if got != want {
		t.Fatalf("renderMailEntry =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderMailEntryNoSubject(t *testing.T) {
	e := SessionEntry{
		Ts:   "2026-04-10T14:02:00Z",
		Type: "mail",
		From: "human",
		To:   "agent",
		Body: "Hi there",
	}
	got := renderMailEntry(e)
	want := "**human** 14:02 → agent\nHi there\n"
	if got != want {
		t.Fatalf("renderMailEntry =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderMailEntryWithAttachments(t *testing.T) {
	e := SessionEntry{
		Ts:          "2026-04-10T14:02:00Z",
		Type:        "mail",
		From:        "human",
		To:          "agent",
		Body:        "See attached",
		Attachments: []string{"file.txt", "img.png"},
	}
	got := renderMailEntry(e)
	if !strings.Contains(got, "Attachments:") {
		t.Fatal("missing Attachments header")
	}
	if !strings.Contains(got, "[1] file.txt") {
		t.Fatal("missing attachment 1")
	}
	if !strings.Contains(got, "[2] img.png") {
		t.Fatal("missing attachment 2")
	}
}

func TestRenderEventEntry(t *testing.T) {
	cases := []struct {
		typ  string
		body string
		want string
	}{
		{"thinking", "Let me consider...", "[thinking] Let me consider...\n"},
		{"tool_call", "email({action: check})", "[tool_call] email({action: check})\n"},
		{"tool_result", "email → ok 250ms", "[tool_result] email → ok 250ms\n"},
		{"diary", "I notice the user...", "[diary] I notice the user...\n"},
		{"text_input", "some input", "[text_input] some input\n"},
		{"text_output", "some output", "[text_output] some output\n"},
	}
	for _, tc := range cases {
		e := SessionEntry{Ts: "2026-04-10T14:03:00Z", Type: tc.typ, Body: tc.body}
		got := renderEventEntry(e)
		if got != tc.want {
			t.Errorf("renderEventEntry(%s) = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

func TestRenderInsightEntry(t *testing.T) {
	// Auto-insight (no question).
	e := SessionEntry{Ts: "2026-04-10T14:05:00Z", Type: "insight", Body: "The user prefers..."}
	got := renderInsightEntry(e)
	if !strings.Contains(got, "★ insight") {
		t.Fatal("missing ★ insight marker")
	}
	if !strings.Contains(got, "The user prefers...") {
		t.Fatal("missing body")
	}

	// Human /btw inquiry.
	e2 := SessionEntry{
		Ts:       "2026-04-10T14:06:00Z",
		Type:     "insight",
		Body:     "The user seems to value...",
		Question: "What does the user think?",
		Source:   "human",
	}
	got2 := renderInsightEntry(e2)
	if !strings.Contains(got2, "/btw › What does the user think?") {
		t.Fatalf("missing /btw question, got:\n%s", got2)
	}
	if !strings.Contains(got2, "The user seems to value...") {
		t.Fatal("missing body")
	}
}

func TestRenderHourMarkdown(t *testing.T) {
	hour, _ := time.Parse(time.RFC3339, "2026-04-10T14:00:00Z")
	entries := []SessionEntry{
		{Ts: "2026-04-10T14:02:00Z", Type: "mail", From: "human", To: "agent", Subject: "hello", Body: "Hi there"},
		{Ts: "2026-04-10T14:03:00Z", Type: "thinking", Body: "Let me consider..."},
		{Ts: "2026-04-10T14:05:00Z", Type: "insight", Body: "The user prefers..."},
	}
	got := renderHourMarkdown(entries, hour)
	if !strings.HasPrefix(got, "# Session — 2026-04-10 14:00–15:00 UTC\n") {
		t.Fatalf("bad header, got:\n%s", got)
	}
	if !strings.Contains(got, "**human** 14:02 → agent │ Re: hello") {
		t.Fatal("missing mail entry")
	}
	if !strings.Contains(got, "[thinking] Let me consider...") {
		t.Fatal("missing thinking entry")
	}
	if !strings.Contains(got, "★ insight") {
		t.Fatal("missing insight entry")
	}
}

func TestDumpCompletedHour(t *testing.T) {
	dir := t.TempDir()
	hour, _ := time.Parse(time.RFC3339, "2026-04-10T14:00:00Z")
	entries := []SessionEntry{
		{Ts: "2026-04-10T14:02:00Z", Type: "mail", From: "human", To: "agent", Body: "Hi"},
	}

	// First dump — file should be created.
	dumpCompletedHour(entries, hour, dir)
	path := filepath.Join(dir, "2026-04-10-14.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(data), "Hi") {
		t.Fatal("missing content")
	}

	// Second dump with same content — file should not be rewritten.
	info1, _ := os.Stat(path)
	modTime1 := info1.ModTime()
	dumpCompletedHour(entries, hour, dir)
	info2, _ := os.Stat(path)
	if info2.ModTime() != modTime1 {
		t.Fatal("identical content should not rewrite file")
	}

	// Third dump with different content — file should be rewritten.
	entries2 := []SessionEntry{
		{Ts: "2026-04-10T14:02:00Z", Type: "mail", From: "human", To: "agent", Body: "Changed"},
	}
	dumpCompletedHour(entries2, hour, dir)
	data2, _ := os.ReadFile(path)
	if !strings.Contains(string(data2), "Changed") {
		t.Fatal("content should have been updated")
	}
}

func TestDumpCompletedHourEmpty(t *testing.T) {
	dir := t.TempDir()
	hour, _ := time.Parse(time.RFC3339, "2026-04-10T14:00:00Z")

	// Empty entries — no file created.
	dumpCompletedHour(nil, hour, dir)
	path := filepath.Join(dir, "2026-04-10-14.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("empty hour should not produce a file")
	}
}

func TestSessionCacheHourBoundaryDump(t *testing.T) {
	dir := t.TempDir()
	humanDir := filepath.Join(dir, "human")
	os.MkdirAll(filepath.Join(humanDir, "logs"), 0o755)

	// Use a known project path so we can predict the dump directory.
	projectPath := "/test/project"
	hash := projectHash(projectPath)
	histDir := filepath.Join(dir, "brief", hash, "history")

	// Create cache with overridden briefBase for testing.
	sc := NewSessionCache(humanDir, projectPath)
	sc.briefBase = dir // override ~/.lingtai-tui to temp dir for testing

	// Append entries in hour 14.
	sc.append(SessionEntry{Ts: "2026-04-10T14:02:00Z", Type: "mail", From: "human", To: "agent", Body: "Hi"})
	sc.append(SessionEntry{Ts: "2026-04-10T14:30:00Z", Type: "thinking", Body: "Hmm..."})

	// No dump yet — still in hour 14.
	if _, err := os.Stat(filepath.Join(histDir, "2026-04-10-14.md")); !os.IsNotExist(err) {
		t.Fatal("should not dump before hour boundary")
	}

	// Append entry in hour 15 — should trigger dump of hour 14.
	sc.append(SessionEntry{Ts: "2026-04-10T15:01:00Z", Type: "mail", From: "agent", To: "human", Body: "Hello"})

	data, err := os.ReadFile(filepath.Join(histDir, "2026-04-10-14.md"))
	if err != nil {
		t.Fatalf("hour 14 markdown not created: %v", err)
	}
	if !strings.Contains(string(data), "Hi") {
		t.Fatal("missing mail entry in dump")
	}
	if !strings.Contains(string(data), "[thinking] Hmm...") {
		t.Fatal("missing thinking entry in dump")
	}
	// The hour-15 entry should NOT be in the hour-14 dump.
	if strings.Contains(string(data), "Hello") {
		t.Fatal("hour-15 entry should not be in hour-14 dump")
	}
}
