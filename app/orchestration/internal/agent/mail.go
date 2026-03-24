package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MailWriter writes messages to a recipient's mailbox inbox on the filesystem.
type MailWriter struct {
	recipientDir string // recipient's working dir (absolute path)
	mailboxRel   string // relative mailbox path, e.g. "mailbox" or "email"
}

// NewMailWriter creates a MailWriter that delivers to recipientDir/mailboxRel/inbox/.
func NewMailWriter(recipientDir, mailboxRel string) *MailWriter {
	return &MailWriter{recipientDir: recipientDir, mailboxRel: mailboxRel}
}

// Send writes a message to the recipient's inbox.
//
// Handshake:
//  1. Check {recipientDir}/.agent.json exists and is valid JSON.
//  2. Check {recipientDir}/.agent.heartbeat is fresh (< 2 seconds).
//  3. Generate a UUID directory under {recipientDir}/{mailboxRel}/inbox/{uuid}/.
//  4. Write message.json atomically (tmp file + rename).
func (w *MailWriter) Send(payload map[string]interface{}) error {
	// 1. Verify .agent.json exists
	agentJSON := filepath.Join(w.recipientDir, ".agent.json")
	data, err := os.ReadFile(agentJSON)
	if err != nil {
		return fmt.Errorf("no agent at %s: %w", w.recipientDir, err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("invalid .agent.json at %s: %w", w.recipientDir, err)
	}

	// 2. Verify heartbeat is fresh
	heartbeatPath := filepath.Join(w.recipientDir, ".agent.heartbeat")
	hbData, err := os.ReadFile(heartbeatPath)
	if err != nil {
		return fmt.Errorf("agent at %s is not running (no heartbeat): %w", w.recipientDir, err)
	}
	var ts float64
	if _, err := fmt.Sscanf(string(hbData), "%f", &ts); err != nil {
		return fmt.Errorf("agent at %s has invalid heartbeat", w.recipientDir)
	}
	now := float64(time.Now().UnixNano()) / 1e9
	age := now - ts
	if age >= 2.0 || age < 0 {
		return fmt.Errorf("agent at %s is not running (heartbeat stale by %.1fs)", w.recipientDir, age)
	}

	// 3. Create inbox entry with mailbox metadata
	msgID := generateUUID()
	payload["_mailbox_id"] = msgID
	payload["received_at"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")

	inboxDir := filepath.Join(w.recipientDir, w.mailboxRel, "inbox", msgID)
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return fmt.Errorf("create inbox dir: %w", err)
	}

	// 4. Atomic write: tmp → rename
	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	tmpPath := filepath.Join(inboxDir, "message.json.tmp")
	finalPath := filepath.Join(inboxDir, "message.json")

	if err := os.WriteFile(tmpPath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("write tmp message: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("rename message: %w", err)
	}

	return nil
}

// MailPoller polls a mailbox inbox directory for new messages.
type MailPoller struct {
	inboxDir string
	seen     map[string]bool
	handler  func(map[string]interface{})
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewMailPoller creates a poller that watches inboxDir for new message directories.
func NewMailPoller(inboxDir string, handler func(map[string]interface{})) *MailPoller {
	return &MailPoller{
		inboxDir: inboxDir,
		seen:     make(map[string]bool),
		handler:  handler,
		done:     make(chan struct{}),
	}
}

// Start begins polling the inbox every 500ms. Existing entries are recorded
// as "seen" so they are not re-delivered.
func (p *MailPoller) Start() {
	// Snapshot existing inbox entries
	entries, err := os.ReadDir(p.inboxDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				p.seen[e.Name()] = true
			}
		}
	}

	p.wg.Add(1)
	go p.pollLoop()
}

// Stop signals the poller to stop and waits for it to finish.
func (p *MailPoller) Stop() {
	close(p.done)
	p.wg.Wait()
}

func (p *MailPoller) pollLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.scan()
		}
	}
}

func (p *MailPoller) scan() {
	entries, err := os.ReadDir(p.inboxDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || p.seen[e.Name()] {
			continue
		}
		msgPath := filepath.Join(p.inboxDir, e.Name(), "message.json")
		data, err := os.ReadFile(msgPath)
		if err != nil {
			continue // message.json not yet written (or error)
		}
		p.seen[e.Name()] = true
		var msg map[string]interface{}
		if json.Unmarshal(data, &msg) == nil {
			p.handler(msg)
		}
	}
}

// SetupHumanWorkdir creates a working directory for the human TUI user.
// Returns the absolute path to the created directory.
func SetupHumanWorkdir(baseDir, humanID, humanName, language string) (string, error) {
	workdir := filepath.Join(baseDir, humanID)
	inboxDir := filepath.Join(workdir, "mailbox", "inbox")
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return "", fmt.Errorf("create human workdir: %w", err)
	}

	// Write .agent.json manifest (admin: null marks this as a human)
	manifest := map[string]interface{}{
		"address":    workdir,
		"agent_name": humanName,
		"admin":      nil,
		"language":   language,
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(workdir, ".agent.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return "", fmt.Errorf("write human manifest: %w", err)
	}

	return workdir, nil
}

// StartHumanHeartbeat writes the current Unix timestamp to .agent.heartbeat
// every second until the done channel is closed.
func StartHumanHeartbeat(workdir string, done <-chan struct{}) {
	heartbeatPath := filepath.Join(workdir, ".agent.heartbeat")
	// Write initial heartbeat immediately
	writeHeartbeat(heartbeatPath)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				os.Remove(heartbeatPath)
				return
			case <-ticker.C:
				writeHeartbeat(heartbeatPath)
			}
		}
	}()
}

func writeHeartbeat(path string) {
	ts := fmt.Sprintf("%.6f", float64(time.Now().UnixNano())/1e9)
	os.WriteFile(path, []byte(ts), 0644)
}

// WriteContacts writes contact entries to the contacts.json file in the given
// mailbox directory.
func WriteContacts(mailboxDir string, contacts []map[string]interface{}) error {
	os.MkdirAll(mailboxDir, 0755)
	data, err := json.MarshalIndent(contacts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal contacts: %w", err)
	}
	path := filepath.Join(mailboxDir, "contacts.json")
	return os.WriteFile(path, data, 0644)
}

// WaitForAgentJSON waits until .agent.json appears in the given directory.
func WaitForAgentJSON(workdir string, timeout time.Duration) error {
	agentJSON := filepath.Join(workdir, ".agent.json")
	deadline := time.Now().Add(timeout)
	backoff := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		if _, err := os.Stat(agentJSON); err == nil {
			return nil
		}
		time.Sleep(backoff)
		backoff *= 2
		if backoff > 2*time.Second {
			backoff = 2 * time.Second
		}
	}
	return fmt.Errorf(".agent.json not found in %s after %s", workdir, timeout)
}

// generateUUID returns a UUID v4 string (hex, no dashes) using crypto/rand.
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	// Set version (4) and variant bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}
