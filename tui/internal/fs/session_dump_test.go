// tui/internal/fs/session_dump_test.go
package fs

import (
	"os"
	"path/filepath"
	"strings"
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
