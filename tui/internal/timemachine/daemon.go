package timemachine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	pollInterval = 5 * time.Second
	thinInterval = 1 * time.Hour
	maxFileSize  int64 = 10 * 1024 * 1024       // 10MB
	maxRepoSize  int64 = 2 * 1024 * 1024 * 1024 // 2GB
)

// MessageInfo holds sender and subject from a message.json.
type MessageInfo struct {
	From    string `json:"from"`
	Subject string `json:"subject"`
}

// FindOrchestrator returns the directory of the orchestrator agent
// (the one whose .agent.json has admin with at least one truthy value).
func FindOrchestrator(lingtaiDir string) string {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "human" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(lingtaiDir, e.Name(), ".agent.json"))
		if err != nil {
			continue
		}
		var manifest struct {
			Admin map[string]bool `json:"admin"`
		}
		if json.Unmarshal(data, &manifest) != nil || manifest.Admin == nil {
			continue
		}
		for _, v := range manifest.Admin {
			if v {
				return filepath.Join(lingtaiDir, e.Name())
			}
		}
	}
	return ""
}

// isAlive checks if an agent's heartbeat is fresh (< 3 seconds old).
func isAlive(agentDir string) bool {
	data, err := os.ReadFile(filepath.Join(agentDir, ".agent.heartbeat"))
	if err != nil {
		return false
	}
	ts, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return false
	}
	return float64(time.Now().Unix())-ts < 3
}

// ScanInbox checks for new messages in human/mailbox/inbox/.
// Updates the known set and returns info about new messages.
func ScanInbox(lingtaiDir string, known map[string]bool) []MessageInfo {
	inboxDir := filepath.Join(lingtaiDir, "human", "mailbox", "inbox")
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		return nil
	}

	var newMsgs []MessageInfo
	for _, e := range entries {
		if !e.IsDir() || known[e.Name()] {
			continue
		}
		known[e.Name()] = true

		data, err := os.ReadFile(filepath.Join(inboxDir, e.Name(), "message.json"))
		if err != nil {
			continue
		}
		var msg MessageInfo
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		newMsgs = append(newMsgs, msg)
	}
	return newMsgs
}

// IsRunning checks if a time machine daemon is already running for this dir.
func IsRunning(lingtaiDir string) bool {
	data, err := os.ReadFile(filepath.Join(lingtaiDir, ".timemachine.pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// Run is the daemon main loop. Blocks until the orchestrator dies.
func Run(lingtaiDir string) {
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Println("time machine: git not found, exiting")
		return
	}

	orchDir := FindOrchestrator(lingtaiDir)
	if orchDir == "" {
		fmt.Println("time machine: no orchestrator found, exiting")
		return
	}

	if !isAlive(orchDir) {
		fmt.Println("time machine: orchestrator not alive, exiting")
		return
	}

	if err := InitGit(lingtaiDir); err != nil {
		fmt.Printf("time machine: git init failed: %v\n", err)
		return
	}

	writePID(lingtaiDir)
	defer removePID(lingtaiDir)

	// Seed known set with existing inbox messages
	known := make(map[string]bool)
	ScanInbox(lingtaiDir, known)

	lastThin := time.Now()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		if !isAlive(orchDir) {
			return
		}

		newMsgs := ScanInbox(lingtaiDir, known)
		if len(newMsgs) == 0 {
			continue
		}

		msg := newMsgs[0]
		subject := msg.Subject
		if len(subject) > 50 {
			subject = subject[:50]
		}
		commitMsg := fmt.Sprintf("mail: %s → %q", msg.From, subject)

		ScanLargeFiles(lingtaiDir, maxFileSize)
		committed, _ := Commit(lingtaiDir, commitMsg)

		if committed && time.Since(lastThin) >= thinInterval {
			commits, err := ListCommits(lingtaiDir)
			if err == nil && len(commits) > 0 {
				keepers := SelectKeepers(commits, time.Now())
				ThinHistory(lingtaiDir, keepers)
				EnforceSizeCap(lingtaiDir, maxRepoSize)
				lastThin = time.Now()
			}
		}
	}
}

func writePID(lingtaiDir string) {
	os.WriteFile(filepath.Join(lingtaiDir, ".timemachine.pid"), []byte(strconv.Itoa(os.Getpid())), 0o644)
}

func removePID(lingtaiDir string) {
	os.Remove(filepath.Join(lingtaiDir, ".timemachine.pid"))
}
