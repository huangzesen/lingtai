package tui

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// LogEvent is a parsed JSONL event from the agent's log.
type LogEvent struct {
	Type      string      `json:"type"`
	Text      string      `json:"text,omitempty"`
	Sender    string      `json:"sender,omitempty"`
	Subject   string      `json:"subject,omitempty"`
	To        interface{} `json:"to,omitempty"`
	ToolName  string      `json:"tool_name,omitempty"`
	Name      string      `json:"name,omitempty"`
	Timestamp string      `json:"timestamp,omitempty"`
}

// GetToolName returns the tool name, checking both field names.
func (e LogEvent) GetToolName() string {
	if e.ToolName != "" {
		return e.ToolName
	}
	return e.Name
}

// LogTailer tails a JSONL file and sends events to a channel.
type LogTailer struct {
	path   string
	events chan LogEvent
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewLogTailer starts tailing the given JSONL file.
func NewLogTailer(path string) *LogTailer {
	lt := &LogTailer{
		path:   path,
		events: make(chan LogEvent, 100),
		done:   make(chan struct{}),
	}
	lt.wg.Add(1)
	go lt.tailLoop()
	return lt
}

// Events returns the channel of parsed log events.
func (lt *LogTailer) Events() <-chan LogEvent {
	return lt.events
}

// Stop stops the tailer.
func (lt *LogTailer) Stop() {
	close(lt.done)
	lt.wg.Wait()
}

func (lt *LogTailer) tailLoop() {
	defer lt.wg.Done()

	// Wait for file to exist
	for {
		select {
		case <-lt.done:
			return
		default:
		}
		if _, err := os.Stat(lt.path); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	lt.tailFile()
}

func (lt *LogTailer) tailFile() {
	f, err := os.Open(lt.path)
	if err != nil {
		return
	}
	defer f.Close()

	// Seek to end
	f.Seek(0, 2)

	// Use a large scanner buffer — agent log lines can exceed the default 64KB limit.
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for {
		select {
		case <-lt.done:
			return
		default:
		}
		if scanner.Scan() {
			var event LogEvent
			if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Type != "" {
				lt.events <- event
			}
		} else {
			if scanner.Err() != nil {
				// Scanner error (e.g., buffer overflow) — re-open the file and seek to end.
				f.Close()
				f2, err := os.Open(lt.path)
				if err != nil {
					return
				}
				f = f2
				f.Seek(0, 2)
				scanner = bufio.NewScanner(f)
				scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
				continue
			}
			// No new data, wait briefly then reset scanner to pick up new data
			time.Sleep(200 * time.Millisecond)
			scanner = bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		}
	}
}
