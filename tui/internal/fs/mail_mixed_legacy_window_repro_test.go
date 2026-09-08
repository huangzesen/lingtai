package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeMixedIDWindowMessage writes one mailbox leaf under folder with an
// arbitrary id and pins the leaf's mtime to the message's ReceivedAt, the way a
// leaf written at delivery time looks on disk. Unlike writeRecentWindowMessage
// the id is caller-chosen, so a test can mix canonical `YYYYMMDDTHHMMSS-xxxx`
// ids with the `hb-*` and hash-style names older producers left behind.
func writeMixedIDWindowMessage(t *testing.T, humanDir, folder, id string, received time.Time) {
	t.Helper()
	dir := filepath.Join(humanDir, "mailbox", folder, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	msg := MailMessage{
		ID:         id,
		MailboxID:  id,
		From:       "human",
		To:         []string{"agent"},
		Message:    id,
		Type:       "normal",
		ReceivedAt: received.UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "message.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, received, received); err != nil {
		t.Fatal(err)
	}
}

// countMailboxBodyReads swaps the body-read seam for the rest of the test and
// returns a counter of message.json opens, so a test can pin the bounded-read
// promise itself rather than infer it from timing.
func countMailboxBodyReads(t *testing.T) *int {
	t.Helper()
	previous := readMailboxFile
	reads := 0
	readMailboxFile = func(path string) ([]byte, error) {
		reads++
		return previous(path)
	}
	t.Cleanup(func() { readMailboxFile = previous })
	return &reads
}

func assertChronological(t *testing.T, messages []MailMessage) {
	t.Helper()
	for i := 1; i < len(messages); i++ {
		if !mailMessageBefore(messages[i-1], messages[i]) {
			t.Fatalf("window is not chronological at index %d: %q then %q",
				i, messages[i-1].MailboxID, messages[i].MailboxID)
		}
	}
}

// TestReproRefreshRecentMixedLegacyIDsHidesNewestTimestampMessage is the
// regression reproduced from a real project: more than `limit` legacy `hb-*`
// leaves sort lexically after every `2026…` canonical id, so a single lexical
// window over directory names dropped the newest delivered message before any
// body was read. The newest canonical message must survive, be last, and the
// window must still hold exactly `limit` chronologically ordered entries.
func TestReproRefreshRecentMixedLegacyIDsHidesNewestTimestampMessage(t *testing.T) {
	humanDir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const limit = 1000
	for i := 0; i < limit; i++ {
		writeMixedIDWindowMessage(t, humanDir, "sent", fmt.Sprintf("hb-ff%04x", i), base.Add(time.Duration(i)*time.Minute))
	}
	newestID := "20260908T030712-df00"
	writeMixedIDWindowMessage(t, humanDir, "sent", newestID, base.Add(2000*time.Minute))

	cache := NewMailCache(humanDir).RefreshRecent(limit)

	if len(cache.Messages) != limit {
		t.Fatalf("loaded %d messages, want exactly the window of %d", len(cache.Messages), limit)
	}
	if _, ok := cache.seen[newestID]; !ok {
		t.Fatalf("newest timestamp message %q missing from bounded cache; lexicographic legacy ids displaced it", newestID)
	}
	if got := cache.Messages[len(cache.Messages)-1].MailboxID; got != newestID {
		t.Errorf("last entry = %q, want the newest message %q", got, newestID)
	}
	if _, stillThere := cache.seen["hb-ff0000"]; stillThere {
		t.Error("the oldest legacy message should have been trimmed to make room for the newest canonical one")
	}
	assertChronological(t, cache.Messages)
}

// TestRefreshRecentIgnoresStagingDirectories pins that dot-prefixed staging
// leaves (`.<id>.staging` and `.<name>.tmp-<hex>`) never occupy a candidate
// slot or surface as messages, on
// the bounded path, the unbounded path, and the listing primitive alike — even
// when an abandoned staging leaf still holds a decodable message.json.
func TestRefreshRecentIgnoresStagingDirectories(t *testing.T) {
	humanDir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const limit = 3
	ids := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		stamp := base.Add(time.Duration(i) * time.Minute)
		id := fmt.Sprintf("%s-%04x", stamp.Format("20060102T150405"), i)
		writeMixedIDWindowMessage(t, humanDir, "inbox", id, stamp)
		ids = append(ids, id)
	}
	// Staging names sort after every canonical id and would each have taken a
	// slot under a naive lexical window. One is empty, one is an abandoned
	// leaf with a full body newer than every real message.
	for _, folder := range []string{"inbox", "outbox"} {
		if err := os.MkdirAll(filepath.Join(humanDir, "mailbox", folder, ".message.staging"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeMixedIDWindowMessage(t, humanDir, "inbox", ".message.tmp-abcd", base.Add(time.Hour))

	for name, cache := range map[string]MailCache{
		"bounded":   NewMailCache(humanDir).RefreshRecent(limit),
		"unbounded": NewMailCache(humanDir).Refresh(),
	} {
		if len(cache.Messages) != limit {
			t.Fatalf("%s: loaded %d messages, want all %d real ones", name, len(cache.Messages), limit)
		}
		for _, id := range ids {
			if _, ok := cache.seen[id]; !ok {
				t.Errorf("%s: real message %q was crowded out of the window", name, id)
			}
		}
		for _, msg := range cache.Messages {
			if strings.HasPrefix(msg.MailboxID, ".") {
				t.Errorf("%s: staging leaf %q surfaced as a message", name, msg.MailboxID)
			}
		}
	}
	for _, id := range RecentMailboxIDs(filepath.Join(humanDir, "mailbox", "inbox"), 0) {
		if strings.HasPrefix(id, ".") {
			t.Errorf("RecentMailboxIDs listed staging leaf %q", id)
		}
	}
}

// TestRefreshRecentWindowsLegacyIDsByModTime pins the legacy proxy: ids whose
// names carry no chronology are windowed by leaf mtime, so a legacy-only
// mailbox still shows its newest messages even when name order disagrees with
// age.
func TestRefreshRecentWindowsLegacyIDsByModTime(t *testing.T) {
	humanDir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const total, limit = 10, 4
	// Hash-style names chosen so lexical order runs opposite to age: the
	// oldest message has the lexically greatest name.
	ids := make([]string, 0, total)
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("%02x%s", total-i, "c0ffee")
		writeMixedIDWindowMessage(t, humanDir, "inbox", id, base.Add(time.Duration(i)*time.Minute))
		ids = append(ids, id)
	}

	cache := NewMailCache(humanDir).RefreshRecent(limit)

	if len(cache.Messages) != limit {
		t.Fatalf("loaded %d messages, want the newest %d", len(cache.Messages), limit)
	}
	for i, want := range ids[total-limit:] {
		if got := cache.Messages[i].MailboxID; got != want {
			t.Errorf("entry %d = %q, want %q (legacy leaves must be windowed by mtime, not name)", i, got, want)
		}
	}
}

// TestRefreshRecentBoundsBodyReadsAcrossMixedShapes pins the cost promise for
// a mixed mailbox: a cold refresh opens at most one window of bodies per id
// shape (2*limit), a quiet tick opens none, and a tick after two arrivals — one
// of each shape — opens exactly those two, while the snapshot stays capped and
// chronological.
func TestRefreshRecentBoundsBodyReadsAcrossMixedShapes(t *testing.T) {
	humanDir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const limit = 10
	const perShape = 3 * limit
	for i := 0; i < perShape; i++ {
		stamp := base.Add(time.Duration(i) * time.Minute)
		writeMixedIDWindowMessage(t, humanDir, "inbox", fmt.Sprintf("%s-%04x", stamp.Format("20060102T150405"), i), stamp)
		writeMixedIDWindowMessage(t, humanDir, "sent", fmt.Sprintf("hb-%04x", i), stamp.Add(30*time.Second))
	}
	reads := countMailboxBodyReads(t)

	cache := NewMailCache(humanDir).RefreshRecent(limit)
	if *reads > 2*limit {
		t.Fatalf("cold refresh opened %d bodies, want at most 2*limit = %d", *reads, 2*limit)
	}
	if len(cache.Messages) != limit {
		t.Fatalf("cold refresh loaded %d messages, want the cap %d", len(cache.Messages), limit)
	}
	assertChronological(t, cache.Messages)

	*reads = 0
	cache = cache.RefreshRecent(limit)
	if *reads != 0 {
		t.Errorf("quiet tick opened %d bodies, want 0 (retained entries must not be re-read)", *reads)
	}

	newestStamp := base.Add(time.Duration(perShape+1) * time.Hour)
	newestCanonical := fmt.Sprintf("%s-beef", newestStamp.Format("20060102T150405"))
	writeMixedIDWindowMessage(t, humanDir, "inbox", newestCanonical, newestStamp)
	writeMixedIDWindowMessage(t, humanDir, "sent", "hb-late", newestStamp.Add(-time.Minute))
	*reads = 0
	cache = cache.RefreshRecent(limit)
	if *reads != 2 {
		t.Errorf("tick after two arrivals opened %d bodies, want exactly 2", *reads)
	}
	if len(cache.Messages) != limit {
		t.Fatalf("after arrivals the snapshot holds %d, want the cap %d", len(cache.Messages), limit)
	}
	if got := cache.Messages[len(cache.Messages)-1].MailboxID; got != newestCanonical {
		t.Errorf("last entry = %q, want the newest canonical arrival %q", got, newestCanonical)
	}
	if _, ok := cache.seen["hb-late"]; !ok {
		t.Error("the newest legacy arrival did not land in the window")
	}
	assertChronological(t, cache.Messages)
}

// TestRefreshRecentFlipsDeliveredForLegacyIDs keeps the outbox→sent transition
// intact for a legacy-shaped id: one entry, no duplicate, Delivered flipped in
// place, exactly as for canonical ids.
func TestRefreshRecentFlipsDeliveredForLegacyIDs(t *testing.T) {
	humanDir := t.TempDir()
	const id = "hb-0001"
	writeMixedIDWindowMessage(t, humanDir, "outbox", id, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	cache := NewMailCache(humanDir).RefreshRecent(5)
	if len(cache.Messages) != 1 || cache.Messages[0].Delivered {
		t.Fatalf("outbox message = %+v, want exactly one undelivered entry", cache.Messages)
	}

	sentParent := filepath.Join(humanDir, "mailbox", "sent")
	if err := os.MkdirAll(sentParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(humanDir, "mailbox", "outbox", id), filepath.Join(sentParent, id)); err != nil {
		t.Fatal(err)
	}

	cache = cache.RefreshRecent(5)
	if len(cache.Messages) != 1 {
		t.Fatalf("after pickup: %d entries, want 1 (no duplicate across folders)", len(cache.Messages))
	}
	if !cache.Messages[0].Delivered {
		t.Error("after pickup: Delivered = false, want true")
	}
}

// TestParseCanonicalMailboxID pins the shape gate that decides which window an
// id falls into, and that the stamp it yields is the id's own instant.
func TestParseCanonicalMailboxID(t *testing.T) {
	for id, want := range map[string]bool{
		"20260908T030712-df00": true,
		"20260908T030712-1":    true,
		"20260908T030712":      false, // no suffix
		"20260908T030712-":     false, // empty suffix
		"20261308T030712-df00": false, // month 13
		"hb-ff0000":            false,
		"0ac0ffee":             false,
		".message.staging":     false,
		"":                     false,
	} {
		if _, got := parseCanonicalMailboxID(id); got != want {
			t.Errorf("parseCanonicalMailboxID(%q) ok = %v, want %v", id, got, want)
		}
	}
	stamp, ok := parseCanonicalMailboxID("20260908T030712-df00")
	if want := time.Date(2026, 9, 8, 3, 7, 12, 0, time.UTC); !ok || !stamp.Equal(want) {
		t.Errorf("stamp = %v (ok=%v), want %v", stamp, ok, want)
	}
}

// TestRefreshRecentReadsNoBodyForCandidatesOlderThanTheWindow pins the
// steady-state gate on the canonical-only shape too: with a full retained
// window, a canonical leaf that is older than everything retained (a late
// backfill, or an id that was trimmed last tick) is never opened, while an
// arrival newer than the window still is.
func TestRefreshRecentReadsNoBodyForCandidatesOlderThanTheWindow(t *testing.T) {
	humanDir := t.TempDir()
	const limit = 4
	for i := 1; i <= limit; i++ {
		writeRecentWindowMessage(t, humanDir, "inbox", 10+i)
	}
	cache := NewMailCache(humanDir).RefreshRecent(limit)
	if len(cache.Messages) != limit {
		t.Fatalf("baseline loaded %d, want %d", len(cache.Messages), limit)
	}
	reads := countMailboxBodyReads(t)

	// Older than every retained entry: sorts into the window by name only if
	// the window were recomputed from disk, and can never survive the trim.
	writeRecentWindowMessage(t, humanDir, "inbox", 1)
	cache = cache.RefreshRecent(limit)
	if *reads != 0 {
		t.Errorf("a backfilled leaf older than the window cost %d body reads, want 0", *reads)
	}

	newest := writeRecentWindowMessage(t, humanDir, "inbox", 100)
	cache = cache.RefreshRecent(limit)
	if *reads != 1 {
		t.Errorf("one new arrival cost %d body reads, want exactly 1", *reads)
	}
	if got := cache.Messages[len(cache.Messages)-1].MailboxID; got != newest {
		t.Errorf("last entry = %q, want the arrival %q", got, newest)
	}
	if len(cache.Messages) != limit {
		t.Errorf("snapshot holds %d, want the cap %d", len(cache.Messages), limit)
	}
}
