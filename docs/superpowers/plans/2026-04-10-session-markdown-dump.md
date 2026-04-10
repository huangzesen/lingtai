# Session.jsonl Hourly Markdown Dump — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce hourly markdown transcripts of the TUI chat view at `~/.lingtai-tui/brief/<project-hash>/history/` for the future secretary agent.

**Architecture:** When `SessionCache.append()` detects an hour boundary crossing, it slices entries for the completed hour, renders them to markdown mirroring the full verbose TUI view, compares with any existing file, and writes only if different. All dump logic lives in a new `session_dump.go` file. The project hash (SHA-256 first 12 hex chars of the `.lingtai/` parent directory's absolute path) is passed to `SessionCache` at construction time.

**Tech Stack:** Go, crypto/sha256, encoding/hex, os, time, strings, fmt

---

## File Structure

| File | Role |
|------|------|
| `tui/internal/fs/session_dump.go` | **New.** `projectHash()`, `briefHistoryDir()`, `renderHourMarkdown()`, `renderMailEntry()`, `renderEventEntry()`, `renderInsightEntry()`, `dumpCompletedHour()` |
| `tui/internal/fs/session_dump_test.go` | **New.** Tests for hash, rendering, dump logic |
| `tui/internal/fs/session.go` | **Modify.** Add `projectPath string` and `lastHour time.Time` fields to `SessionCache`. Update `NewSessionCache()` signature. Add hour-boundary check in `append()`. |
| `tui/internal/tui/mail.go` | **Modify.** Pass project path to `NewSessionCache()`. |

---

### Task 1: Project hash and directory helpers

**Files:**
- Create: `tui/internal/fs/session_dump.go`
- Create: `tui/internal/fs/session_dump_test.go`

- [ ] **Step 1: Write the failing test for `projectHash()`**

```go
// tui/internal/fs/session_dump_test.go
package fs

import "testing"

func TestProjectHash(t *testing.T) {
	// SHA-256 of "/home/user/myproject" first 12 hex chars.
	// Precomputed: sha256("/home/user/myproject") = "a1d9c09f3e5d..."
	hash := projectHash("/home/user/myproject")
	if len(hash) != 12 {
		t.Fatalf("hash length = %d, want 12", len(hash))
	}
	// Same input → same output (deterministic).
	if hash != projectHash("/home/user/myproject") {
		t.Fatal("hash is not deterministic")
	}
	// Different input → different output.
	if hash == projectHash("/home/user/other") {
		t.Fatal("different paths should produce different hashes")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd tui && go test ./internal/fs/ -run TestProjectHash -v`
Expected: FAIL — `projectHash` undefined

- [ ] **Step 3: Write the failing test for `briefHistoryDir()`**

```go
func TestBriefHistoryDir(t *testing.T) {
	hash := "abcdef012345"
	dir := briefHistoryDir(hash)
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".lingtai-tui", "brief", hash, "history")
	if dir != want {
		t.Fatalf("briefHistoryDir = %q, want %q", dir, want)
	}
}
```

Add `"os"` and `"path/filepath"` to the test imports.

- [ ] **Step 4: Implement `projectHash()` and `briefHistoryDir()`**

```go
// tui/internal/fs/session_dump.go
package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// projectHash returns the first 12 hex chars of SHA-256(projectPath).
func projectHash(projectPath string) string {
	sum := sha256.Sum256([]byte(projectPath))
	return hex.EncodeToString(sum[:])[:12]
}

// briefHistoryDir returns ~/.lingtai-tui/brief/<hash>/history/.
func briefHistoryDir(hash string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lingtai-tui", "brief", hash, "history")
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd tui && go test ./internal/fs/ -run "TestProjectHash|TestBriefHistoryDir" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add tui/internal/fs/session_dump.go tui/internal/fs/session_dump_test.go
git commit -m "feat(fs): add projectHash and briefHistoryDir helpers"
```

---

### Task 2: Markdown rendering functions

**Files:**
- Modify: `tui/internal/fs/session_dump.go`
- Modify: `tui/internal/fs/session_dump_test.go`

- [ ] **Step 1: Write the failing test for `renderMailEntry()`**

```go
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
```

Add `"strings"` to the test imports.

- [ ] **Step 2: Write the failing test for `renderEventEntry()`**

```go
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
```

- [ ] **Step 3: Write the failing test for `renderInsightEntry()`**

```go
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
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd tui && go test ./internal/fs/ -run "TestRenderMail|TestRenderEvent|TestRenderInsight" -v`
Expected: FAIL — undefined functions

- [ ] **Step 5: Implement the three render functions**

Add to `tui/internal/fs/session_dump.go`:

```go
import (
	"fmt"
	"strings"
	"time"
)
```

(Merge with existing imports.)

```go
// renderMailEntry renders a mail SessionEntry to markdown.
func renderMailEntry(e SessionEntry) string {
	ts := ""
	if t, err := time.Parse(time.RFC3339, e.Ts); err == nil {
		ts = t.UTC().Format("15:04")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s** %s → %s", e.From, ts, e.To))
	if e.Subject != "" {
		b.WriteString(" │ Re: " + e.Subject)
	}
	b.WriteString("\n" + e.Body + "\n")
	if len(e.Attachments) > 0 {
		b.WriteString("Attachments:\n")
		for i, att := range e.Attachments {
			b.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, att))
		}
	}
	return b.String()
}

// renderEventEntry renders a thinking/tool/diary SessionEntry to markdown.
func renderEventEntry(e SessionEntry) string {
	return fmt.Sprintf("[%s] %s\n", e.Type, e.Body)
}

// renderInsightEntry renders an insight SessionEntry to markdown.
func renderInsightEntry(e SessionEntry) string {
	var b strings.Builder
	b.WriteString("---\n")
	if e.Question != "" {
		b.WriteString("/btw › " + e.Question + "\n")
	} else {
		b.WriteString("★ insight\n")
	}
	b.WriteString(e.Body + "\n")
	b.WriteString("---\n")
	return b.String()
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd tui && go test ./internal/fs/ -run "TestRenderMail|TestRenderEvent|TestRenderInsight" -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add tui/internal/fs/session_dump.go tui/internal/fs/session_dump_test.go
git commit -m "feat(fs): add markdown rendering functions for session entries"
```

---

### Task 3: Full hour rendering and idempotent dump

**Files:**
- Modify: `tui/internal/fs/session_dump.go`
- Modify: `tui/internal/fs/session_dump_test.go`

- [ ] **Step 1: Write the failing test for `renderHourMarkdown()`**

```go
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
```

- [ ] **Step 2: Write the failing test for `dumpCompletedHour()`**

```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd tui && go test ./internal/fs/ -run "TestRenderHourMarkdown|TestDumpCompletedHour" -v`
Expected: FAIL — undefined functions

- [ ] **Step 4: Implement `renderHourMarkdown()` and `dumpCompletedHour()`**

Add to `tui/internal/fs/session_dump.go`:

```go
// renderHourMarkdown renders a slice of entries for one hour into a markdown document.
func renderHourMarkdown(entries []SessionEntry, hour time.Time) string {
	var b strings.Builder
	nextHour := hour.Add(time.Hour)
	b.WriteString(fmt.Sprintf("# Session — %s %s–%s UTC\n\n",
		hour.Format("2006-01-02"),
		hour.Format("15:04"),
		nextHour.Format("15:04"),
	))
	for _, e := range entries {
		switch e.Type {
		case "mail":
			b.WriteString(renderMailEntry(e))
		case "insight":
			b.WriteString(renderInsightEntry(e))
		default:
			b.WriteString(renderEventEntry(e))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// dumpCompletedHour renders entries for the given hour to markdown and writes
// to historyDir/YYYY-MM-DD-HH.md — but only if the content differs from the
// existing file (or the file does not exist). Empty entries produce no file.
func dumpCompletedHour(entries []SessionEntry, hour time.Time, historyDir string) {
	if len(entries) == 0 {
		return
	}
	content := renderHourMarkdown(entries, hour)
	filename := hour.Format("2006-01-02-15") + ".md"
	path := filepath.Join(historyDir, filename)

	// Compare with existing file.
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return // identical — no rewrite
	}

	os.MkdirAll(historyDir, 0o755)
	os.WriteFile(path, []byte(content), 0o644)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd tui && go test ./internal/fs/ -run "TestRenderHourMarkdown|TestDumpCompletedHour" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add tui/internal/fs/session_dump.go tui/internal/fs/session_dump_test.go
git commit -m "feat(fs): add renderHourMarkdown and idempotent dumpCompletedHour"
```

---

### Task 4: Wire hour boundary detection into SessionCache

**Files:**
- Modify: `tui/internal/fs/session.go`
- Modify: `tui/internal/fs/session_dump_test.go`

- [ ] **Step 1: Write the failing test for hour boundary detection**

```go
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
```

Add `"os"`, `"path/filepath"`, `"strings"` to the test imports if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd tui && go test ./internal/fs/ -run TestSessionCacheHourBoundaryDump -v`
Expected: FAIL — `NewSessionCache` signature mismatch and `briefBase` undefined

- [ ] **Step 3: Add `projectPath`, `lastHour`, and `briefBase` fields to `SessionCache`**

In `tui/internal/fs/session.go`, modify the `SessionCache` struct:

```go
type SessionCache struct {
	path        string          // human/logs/session.jsonl
	entries     []SessionEntry  // in-memory mirror of all entries
	mailSeen    map[string]bool // mail dedup key (from|ts) already written
	eventsOff   int64           // byte offset in events.jsonl
	inquiryOff  int64           // byte offset in soul_inquiry.jsonl
	projectPath string          // absolute path of the project directory (parent of .lingtai/)
	lastHour    time.Time       // hour (truncated) of the most recent entry
	briefBase   string          // base dir for brief output (default: ~/.lingtai-tui)
}
```

- [ ] **Step 4: Update `NewSessionCache()` to accept `projectPath`**

```go
func NewSessionCache(humanDir string, projectPath string) *SessionCache {
	logsDir := filepath.Join(humanDir, "logs")
	os.MkdirAll(logsDir, 0o755)
	path := filepath.Join(logsDir, "session.jsonl")

	home, _ := os.UserHomeDir()
	sc := &SessionCache{
		path:        path,
		mailSeen:    make(map[string]bool),
		projectPath: projectPath,
		briefBase:   filepath.Join(home, ".lingtai-tui"),
	}

	sc.loadExisting()
	return sc
}
```

- [ ] **Step 5: Update `loadExisting()` to set `lastHour` from the last entry**

At the end of `loadExisting()`, after the scan loop:

```go
	// Set lastHour from the final entry.
	if len(sc.entries) > 0 {
		if t, err := time.Parse(time.RFC3339, sc.entries[len(sc.entries)-1].Ts); err == nil {
			sc.lastHour = t.Truncate(time.Hour)
		}
	}
```

- [ ] **Step 6: Add hour boundary check to `append()`**

Replace the existing `append()` method with:

```go
func (sc *SessionCache) append(entries ...SessionEntry) {
	if len(entries) == 0 {
		return
	}

	// Check for hour boundary crossings before appending.
	for _, e := range entries {
		t, err := time.Parse(time.RFC3339, e.Ts)
		if err != nil {
			continue
		}
		entryHour := t.Truncate(time.Hour)
		if !sc.lastHour.IsZero() && entryHour.After(sc.lastHour) {
			// Hour boundary crossed — dump all completed hours.
			sc.dumpHours(sc.lastHour, entryHour)
		}
		sc.lastHour = entryHour
	}

	sc.entries = append(sc.entries, entries...)

	// Append to file.
	f, err := os.OpenFile(sc.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, e := range entries {
		_ = enc.Encode(e)
	}
}
```

- [ ] **Step 7: Add `dumpHours()` helper**

Add to `tui/internal/fs/session.go`:

```go
// dumpHours dumps all completed hours from fromHour up to (but not including) toHour.
func (sc *SessionCache) dumpHours(fromHour, toHour time.Time) {
	if sc.projectPath == "" {
		return
	}
	hash := projectHash(sc.projectPath)
	histDir := filepath.Join(sc.briefBase, "brief", hash, "history")

	for h := fromHour; h.Before(toHour); h = h.Add(time.Hour) {
		// Collect entries for this hour.
		var hourEntries []SessionEntry
		for _, e := range sc.entries {
			t, err := time.Parse(time.RFC3339, e.Ts)
			if err != nil {
				continue
			}
			if t.Truncate(time.Hour).Equal(h) {
				hourEntries = append(hourEntries, e)
			}
		}
		dumpCompletedHour(hourEntries, h, histDir)
	}
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd tui && go test ./internal/fs/ -run TestSessionCacheHourBoundaryDump -v`
Expected: PASS

- [ ] **Step 9: Run all fs tests to check nothing is broken**

Run: `cd tui && go test ./internal/fs/ -v`
Expected: All PASS

- [ ] **Step 10: Commit**

```bash
git add tui/internal/fs/session.go tui/internal/fs/session_dump.go tui/internal/fs/session_dump_test.go
git commit -m "feat(fs): wire hour boundary detection into SessionCache.append()"
```

---

### Task 5: Update MailModel to pass project path

**Files:**
- Modify: `tui/internal/tui/mail.go:143-176`

- [ ] **Step 1: Update `NewMailModel()` to pass project path to `NewSessionCache()`**

In `tui/internal/tui/mail.go`, the `baseDir` parameter is the `.lingtai/` directory. The project path is its parent. Change line 173:

From:
```go
		sessionCache:      fs.NewSessionCache(humanDir),
```

To:
```go
		sessionCache:      fs.NewSessionCache(humanDir, filepath.Dir(baseDir)),
```

- [ ] **Step 2: Build to verify compilation**

Run: `cd tui && make build`
Expected: Builds successfully to `tui/bin/lingtai-tui`

- [ ] **Step 3: Commit**

```bash
git add tui/internal/tui/mail.go
git commit -m "feat(tui): pass project path to SessionCache for brief directory"
```

---

### Task 6: Full integration test

**Files:**
- Modify: `tui/internal/fs/session_dump_test.go`

- [ ] **Step 1: Write the integration test**

```go
func TestSessionCacheMultiHourDump(t *testing.T) {
	dir := t.TempDir()
	humanDir := filepath.Join(dir, "human")
	os.MkdirAll(filepath.Join(humanDir, "logs"), 0o755)

	projectPath := "/test/multi"
	hash := projectHash(projectPath)
	histDir := filepath.Join(dir, "brief", hash, "history")

	sc := NewSessionCache(humanDir, projectPath)
	sc.briefBase = dir

	// Hour 10 entries.
	sc.append(SessionEntry{Ts: "2026-04-10T10:05:00Z", Type: "mail", From: "human", To: "agent", Body: "Morning"})

	// Hour 11 entries — triggers dump of hour 10.
	sc.append(SessionEntry{Ts: "2026-04-10T11:00:00Z", Type: "mail", From: "agent", To: "human", Body: "Hi"})

	// Hour 12 entry — triggers dump of hour 11.
	sc.append(SessionEntry{Ts: "2026-04-10T12:00:00Z", Type: "thinking", Body: "thinking..."})

	// Verify hour 10 dump exists.
	data10, err := os.ReadFile(filepath.Join(histDir, "2026-04-10-10.md"))
	if err != nil {
		t.Fatalf("hour 10 dump missing: %v", err)
	}
	if !strings.Contains(string(data10), "Morning") {
		t.Fatal("hour 10 missing content")
	}

	// Verify hour 11 dump exists.
	data11, err := os.ReadFile(filepath.Join(histDir, "2026-04-10-11.md"))
	if err != nil {
		t.Fatalf("hour 11 dump missing: %v", err)
	}
	if !strings.Contains(string(data11), "Hi") {
		t.Fatal("hour 11 missing content")
	}

	// Hour 12 not yet dumped (no boundary crossed).
	if _, err := os.Stat(filepath.Join(histDir, "2026-04-10-12.md")); !os.IsNotExist(err) {
		t.Fatal("hour 12 should not be dumped yet")
	}
}

func TestSessionCacheIdempotentDump(t *testing.T) {
	dir := t.TempDir()
	humanDir := filepath.Join(dir, "human")
	os.MkdirAll(filepath.Join(humanDir, "logs"), 0o755)

	projectPath := "/test/idempotent"
	hash := projectHash(projectPath)
	histDir := filepath.Join(dir, "brief", hash, "history")

	sc := NewSessionCache(humanDir, projectPath)
	sc.briefBase = dir

	sc.append(SessionEntry{Ts: "2026-04-10T14:02:00Z", Type: "mail", From: "human", To: "agent", Body: "Hi"})
	sc.append(SessionEntry{Ts: "2026-04-10T15:00:00Z", Type: "mail", From: "agent", To: "human", Body: "Next"})

	path14 := filepath.Join(histDir, "2026-04-10-14.md")
	info1, _ := os.Stat(path14)
	modTime1 := info1.ModTime()

	// Create a new cache from the same session.jsonl — simulates TUI restart.
	sc2 := NewSessionCache(humanDir, projectPath)
	sc2.briefBase = dir

	// Append another hour-15 entry + hour-16 entry to trigger dump of hour 15.
	sc2.append(SessionEntry{Ts: "2026-04-10T15:30:00Z", Type: "thinking", Body: "hmm"})
	sc2.append(SessionEntry{Ts: "2026-04-10T16:00:00Z", Type: "mail", From: "human", To: "agent", Body: "Later"})

	// Hour 14 file should NOT be rewritten (identical content).
	info2, _ := os.Stat(path14)
	if info2.ModTime() != modTime1 {
		t.Fatal("hour 14 should not have been rewritten")
	}

	// Hour 15 should now exist.
	data15, err := os.ReadFile(filepath.Join(histDir, "2026-04-10-15.md"))
	if err != nil {
		t.Fatalf("hour 15 dump missing: %v", err)
	}
	if !strings.Contains(string(data15), "Next") {
		t.Fatal("hour 15 missing mail entry")
	}
	if !strings.Contains(string(data15), "[thinking] hmm") {
		t.Fatal("hour 15 missing thinking entry")
	}
}
```

- [ ] **Step 2: Run integration tests**

Run: `cd tui && go test ./internal/fs/ -run "TestSessionCacheMultiHour|TestSessionCacheIdempotent" -v`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `cd tui && go test ./... -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add tui/internal/fs/session_dump_test.go
git commit -m "test(fs): add integration tests for multi-hour dump and idempotency"
```
