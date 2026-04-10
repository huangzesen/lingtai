// internal/fs/session.go — append-only session log and in-memory cache.
package fs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// SessionEntry is the JSON-serializable entry stored in session.jsonl.
type SessionEntry struct {
	Ts          string   `json:"ts"`
	Type        string   `json:"type"`
	From        string   `json:"from,omitempty"`
	To          string   `json:"to,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	Body        string   `json:"body"`
	Question    string   `json:"question,omitempty"`
	Attachments []string `json:"attachments,omitempty"`
	Source      string   `json:"source,omitempty"` // "human", "insight" — for inquiry entries
}

// SessionCache is an append-only cache backed by session.jsonl.
// It incrementally tails three data sources and appends new entries.
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

// NewSessionCache opens (or creates) session.jsonl and loads existing entries
// into memory. Source file offsets are set to end-of-file so only new entries
// are appended going forward.
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

func (sc *SessionCache) loadExisting() {
	f, err := os.Open(sc.path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e SessionEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		sc.entries = append(sc.entries, e)
		// Rebuild mailSeen from existing mail entries.
		if e.Type == "mail" {
			sc.mailSeen[e.From+"|"+e.Ts] = true
		}
	}

	// Set lastHour from the final entry.
	if len(sc.entries) > 0 {
		if t, err := time.Parse(time.RFC3339, sc.entries[len(sc.entries)-1].Ts); err == nil {
			sc.lastHour = t.Truncate(time.Hour)
		}
	}
}

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

// Entries returns all entries in the cache.
func (sc *SessionCache) Entries() []SessionEntry {
	return sc.entries
}

// Len returns the total number of entries.
func (sc *SessionCache) Len() int {
	return len(sc.entries)
}

// ---------------------------------------------------------------------------
// Mail ingestion
// ---------------------------------------------------------------------------

// IngestMail appends new mail messages to the session log.
// humanAddr is the human's mail address (to determine IsFromMe).
// orchName is the orchestrator's display name.
func (sc *SessionCache) IngestMail(cache MailCache, humanAddr, orchDir, orchName string) {
	var newEntries []SessionEntry
	for _, msg := range cache.Messages {
		key := msg.From + "|" + msg.ReceivedAt
		if sc.mailSeen[key] {
			continue
		}
		sc.mailSeen[key] = true

		from := resolveMailFrom(msg, humanAddr)
		to := resolveMailTo(msg, humanAddr, orchName)

		newEntries = append(newEntries, SessionEntry{
			Ts:          msg.ReceivedAt,
			Type:        "mail",
			From:        from,
			To:          to,
			Subject:     msg.Subject,
			Body:        msg.Message,
			Attachments: msg.Attachments,
		})
	}
	sc.append(newEntries...)
}

func resolveMailFrom(msg MailMessage, humanAddr string) string {
	parts := splitLast(msg.From, "/")
	if msg.From == humanAddr || parts == "human" {
		return "human"
	}
	if nick, ok := msg.Identity["nickname"].(string); ok && nick != "" {
		return nick
	}
	if name, ok := msg.Identity["agent_name"].(string); ok && name != "" {
		return name
	}
	return parts
}

func resolveMailTo(msg MailMessage, humanAddr, orchName string) string {
	to := fmt.Sprintf("%v", msg.To)
	if to == humanAddr {
		return "human"
	}
	return orchName
}

func splitLast(s, sep string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if string(s[i]) == sep {
			return s[i+1:]
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// Events ingestion
// ---------------------------------------------------------------------------

// IngestEvents tails the orchestrator's events.jsonl from the last-read offset,
// converting new entries to SessionEntry. ALL event types are ingested — verbose
// filtering happens at render time.
func (sc *SessionCache) IngestEvents(orchDir string) {
	if orchDir == "" {
		return
	}
	eventsPath := filepath.Join(orchDir, "logs", "events.jsonl")
	newEntries, newOff := sc.tailJSONL(eventsPath, sc.eventsOff, parseEvent)
	sc.eventsOff = newOff
	sc.append(newEntries...)
}

// tailJSONL reads a JSONL file from the given byte offset, calls parseFn on each
// line, and returns new SessionEntry values plus the updated offset.
func (sc *SessionCache) tailJSONL(path string, offset int64, parseFn func([]byte) *SessionEntry) ([]SessionEntry, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer f.Close()

	// Check if file was truncated (e.g. agent molt reset the log).
	info, err := f.Stat()
	if err != nil {
		return nil, offset
	}
	if info.Size() < offset {
		offset = 0 // file was truncated, restart from beginning
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset
	}

	var entries []SessionEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if e := parseFn(scanner.Bytes()); e != nil {
			entries = append(entries, *e)
		}
	}

	newOff, _ := f.Seek(0, io.SeekCurrent)
	return entries, newOff
}

func parseEvent(line []byte) *SessionEntry {
	var raw map[string]interface{}
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil
	}
	eventType, _ := raw["type"].(string)

	switch eventType {
	case "thinking", "diary", "text_input", "text_output", "tool_call", "tool_result", "insight":
		// ok
	default:
		return nil
	}

	text := extractSessionEventText(raw, eventType)
	if text == "" {
		return nil
	}

	ts := ""
	if tsFloat, ok := raw["ts"].(float64); ok {
		ts = time.Unix(int64(tsFloat), 0).UTC().Format(time.RFC3339)
	}

	e := &SessionEntry{
		Ts:   ts,
		Type: eventType,
		Body: text,
	}

	if eventType == "insight" {
		if q, ok := raw["question"].(string); ok {
			e.Question = q
		}
	}

	return e
}

func extractSessionEventText(entry map[string]interface{}, eventType string) string {
	switch eventType {
	case "thinking", "diary", "text_output", "text_input", "insight":
		text, _ := entry["text"].(string)
		return text
	case "tool_call":
		name, _ := entry["tool_name"].(string)
		args, _ := entry["tool_args"].(string)
		if args == "" {
			if argsMap, ok := entry["tool_args"].(map[string]interface{}); ok {
				data, _ := json.Marshal(argsMap)
				args = string(data)
			}
		}
		if len(args) > 200 {
			args = args[:200] + "..."
		}
		return fmt.Sprintf("%s(%s)", name, args)
	case "tool_result":
		name, _ := entry["tool_name"].(string)
		status, _ := entry["status"].(string)
		elapsed := ""
		if ms, ok := entry["elapsed_ms"].(float64); ok {
			elapsed = fmt.Sprintf(" %dms", int(ms))
		}
		return fmt.Sprintf("%s → %s%s", name, status, elapsed)
	}
	return ""
}

// ---------------------------------------------------------------------------
// Inquiry ingestion
// ---------------------------------------------------------------------------

// IngestInquiries tails the orchestrator's soul_inquiry.jsonl from the last-read
// offset. Only human and insight-sourced inquiries are ingested.
func (sc *SessionCache) IngestInquiries(orchDir string) {
	if orchDir == "" {
		return
	}
	inquiryPath := filepath.Join(orchDir, "logs", "soul_inquiry.jsonl")
	newEntries, newOff := sc.tailJSONL(inquiryPath, sc.inquiryOff, parseInquiry)
	sc.inquiryOff = newOff
	sc.append(newEntries...)
}

func parseInquiry(line []byte) *SessionEntry {
	var raw map[string]interface{}
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil
	}
	source, _ := raw["source"].(string)
	if source != "human" && source != "insight" {
		return nil
	}
	voice, _ := raw["voice"].(string)
	if voice == "" {
		return nil
	}
	ts, _ := raw["ts"].(string)

	e := &SessionEntry{
		Ts:     ts,
		Type:   "insight",
		Body:   voice,
		Source: source,
	}
	if source == "human" {
		e.Question, _ = raw["prompt"].(string)
	}
	return e
}

// ---------------------------------------------------------------------------
// Refresh + offset helpers
// ---------------------------------------------------------------------------

// Refresh polls all three data sources and appends new entries to the session log.
func (sc *SessionCache) Refresh(cache MailCache, humanAddr, orchDir, orchName string) {
	sc.IngestMail(cache, humanAddr, orchDir, orchName)
	sc.IngestEvents(orchDir)
	sc.IngestInquiries(orchDir)
}

// SetSourceOffsets seeks source file offsets to end-of-file.
// Call this after loading an existing session.jsonl on startup so we only
// read new entries going forward.
func (sc *SessionCache) SetSourceOffsets(orchDir string) {
	if orchDir == "" {
		return
	}
	sc.eventsOff = fileSize(filepath.Join(orchDir, "logs", "events.jsonl"))
	sc.inquiryOff = fileSize(filepath.Join(orchDir, "logs", "soul_inquiry.jsonl"))
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
