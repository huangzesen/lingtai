package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadInbox(t *testing.T) {
	dir := t.TempDir()
	inbox := filepath.Join(dir, "mailbox", "inbox", "msg-001")
	os.MkdirAll(inbox, 0o755)

	msg := MailMessage{
		ID: "msg-001", MailboxID: "msg-001", From: "/agents/alice",
		To: "/agents/human", Subject: "hello", Message: "hi there",
		Type: "normal", ReceivedAt: "2026-03-25T12:00:00.000Z",
	}
	data, _ := json.Marshal(msg)
	os.WriteFile(filepath.Join(inbox, "message.json"), data, 0o644)

	messages, err := ReadInbox(dir)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("inbox len = %d, want 1", len(messages))
	}
	if messages[0].Subject != "hello" {
		t.Errorf("subject = %q, want %q", messages[0].Subject, "hello")
	}
}

func TestWriteMail(t *testing.T) {
	recipientDir := t.TempDir()
	os.MkdirAll(filepath.Join(recipientDir, "mailbox", "inbox"), 0o755)
	senderDir := t.TempDir()
	os.MkdirAll(filepath.Join(senderDir, "mailbox", "sent"), 0o755)

	err := WriteMail(recipientDir, senderDir, "/sender/human", "/recipient/alice", "test subject", "test body")
	if err != nil {
		t.Fatalf("write mail: %v", err)
	}

	messages, err := ReadInbox(recipientDir)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("inbox len = %d, want 1", len(messages))
	}
	if messages[0].Message != "test body" {
		t.Errorf("message = %q, want %q", messages[0].Message, "test body")
	}
	if messages[0].From != "/sender/human" {
		t.Errorf("from = %q, want %q", messages[0].From, "/sender/human")
	}

	sent, err := ReadSent(senderDir)
	if err != nil {
		t.Fatalf("read sent: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("sent len = %d, want 1", len(sent))
	}
}

func TestReadInbox_Empty(t *testing.T) {
	dir := t.TempDir()
	messages, err := ReadInbox(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected empty inbox, got %d", len(messages))
	}
}
