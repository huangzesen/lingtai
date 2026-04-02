package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

func ReadInbox(dir string) ([]MailMessage, error) {
	return readMailFolder(filepath.Join(dir, "mailbox", "inbox"))
}

func ReadSent(dir string) ([]MailMessage, error) {
	return readMailFolder(filepath.Join(dir, "mailbox", "sent"))
}

// MailCache tracks already-loaded messages for incremental refresh.
// Each Refresh call reads only new messages from disk.
type MailCache struct {
	inboxSeen map[string]struct{} // UUID dirs already loaded from inbox
	sentSeen  map[string]struct{} // UUID dirs already loaded from sent
	Messages  []MailMessage       // full sorted merged slice (inbox + sent)
	inboxDir  string
	sentDir   string
}

// NewMailCache creates an empty cache for the given human directory.
func NewMailCache(humanDir string) MailCache {
	return MailCache{
		inboxSeen: make(map[string]struct{}),
		sentSeen:  make(map[string]struct{}),
		inboxDir:  filepath.Join(humanDir, "mailbox", "inbox"),
		sentDir:   filepath.Join(humanDir, "mailbox", "sent"),
	}
}

// Refresh scans inbox and sent folders for new messages, returning an updated
// cache. The receiver is not mutated — safe to call from a goroutine.
func (c MailCache) Refresh() MailCache {
	out := MailCache{
		inboxSeen: make(map[string]struct{}, len(c.inboxSeen)+16),
		sentSeen:  make(map[string]struct{}, len(c.sentSeen)+16),
		Messages:  make([]MailMessage, len(c.Messages)),
		inboxDir:  c.inboxDir,
		sentDir:   c.sentDir,
	}
	copy(out.Messages, c.Messages)
	for k := range c.inboxSeen {
		out.inboxSeen[k] = struct{}{}
	}
	for k := range c.sentSeen {
		out.sentSeen[k] = struct{}{}
	}

	// Scan inbox for new entries
	out.scanFolder(out.inboxDir, out.inboxSeen)
	// Scan sent for new entries
	out.scanFolder(out.sentDir, out.sentSeen)

	// Sort by ReceivedAt (RFC3339 strings sort lexicographically)
	sort.Slice(out.Messages, func(i, j int) bool {
		return out.Messages[i].ReceivedAt < out.Messages[j].ReceivedAt
	})
	return out
}

// scanFolder reads UUID directories not yet in seen, loads their message.json,
// and appends to Messages.
func (c *MailCache) scanFolder(folder string, seen map[string]struct{}) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := seen[name]; ok {
			continue
		}
		msgPath := filepath.Join(folder, name, "message.json")
		data, err := os.ReadFile(msgPath)
		if err != nil {
			continue
		}
		var msg MailMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		seen[name] = struct{}{}
		c.Messages = append(c.Messages, msg)
	}
}

func readMailFolder(folder string) ([]MailMessage, error) {
	entries, err := os.ReadDir(folder)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read folder: %w", err)
	}
	var messages []MailMessage
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		msgPath := filepath.Join(folder, entry.Name(), "message.json")
		data, err := os.ReadFile(msgPath)
		if err != nil {
			continue
		}
		var msg MailMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// readManifestAsIdentity reads .agent.json from dir and returns it as the identity card.
func readManifestAsIdentity(dir string) map[string]interface{} {
	data, err := os.ReadFile(filepath.Join(dir, ".agent.json"))
	if err != nil {
		return map[string]interface{}{"agent_name": "human", "admin": nil}
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return map[string]interface{}{"agent_name": "human", "admin": nil}
	}
	return manifest
}

func WriteMail(recipientDir, senderDir, fromAddr, toAddr, subject, body string) error {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Read sender's manifest as identity card (same as Python agents do)
	identity := readManifestAsIdentity(senderDir)

	msg := MailMessage{
		ID:         id,
		MailboxID:  id,
		From:       fromAddr,
		To:         toAddr,
		CC:         []string{},
		Subject:    subject,
		Message:    body,
		Type:       "normal",
		ReceivedAt: now,
		Identity:   identity,
	}

	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Write to recipient inbox
	inboxDir := filepath.Join(recipientDir, "mailbox", "inbox", id)
	if err := os.MkdirAll(inboxDir, 0o755); err != nil {
		return fmt.Errorf("create inbox dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(inboxDir, "message.json"), data, 0o644); err != nil {
		return fmt.Errorf("write inbox message: %w", err)
	}

	// Write copy to sender's sent folder
	sentDir := filepath.Join(senderDir, "mailbox", "sent", id)
	if err := os.MkdirAll(sentDir, 0o755); err != nil {
		return fmt.Errorf("create sent dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(sentDir, "message.json"), data, 0o644); err != nil {
		return fmt.Errorf("write sent message: %w", err)
	}

	return nil
}
