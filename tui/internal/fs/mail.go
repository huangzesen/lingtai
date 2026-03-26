package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

func ReadInbox(dir string) ([]MailMessage, error) {
	return readMailFolder(filepath.Join(dir, "mailbox", "inbox"))
}

func ReadSent(dir string) ([]MailMessage, error) {
	return readMailFolder(filepath.Join(dir, "mailbox", "sent"))
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
