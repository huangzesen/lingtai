package agent

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Process struct {
	cmd        *exec.Cmd
	configPath string
	agentPort  int
	workingDir string
	pidPath    string
	logFile    *os.File
}

type StartOptions struct {
	ConfigPath string
	AgentPort  int
	WorkingDir string
	Headless   bool
}

func Start(opts StartOptions) (*Process, error) {
	os.MkdirAll(opts.WorkingDir, 0755)

	// Resolve the lingtai project root relative to the daemon binary.
	// The binary lives at <project>/daemon/lingtai, so project root is ../
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	projectRoot := filepath.Dir(exeDir) // daemon/ -> project root

	cmd := exec.Command("python", "-m", "app", opts.ConfigPath)
	cmd.Dir = projectRoot

	p := &Process{
		cmd:        cmd,
		configPath: opts.ConfigPath,
		agentPort:  opts.AgentPort,
		workingDir: opts.WorkingDir,
		pidPath:    filepath.Join(opts.WorkingDir, "agent.pid"),
	}

	if opts.Headless {
		logPath := filepath.Join(opts.WorkingDir, "daemon.log")
		os.MkdirAll(filepath.Dir(logPath), 0755)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		cmd.Stdout = f
		cmd.Stderr = f
		p.logFile = f
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start python: %w", err)
	}

	WritePIDFile(p.pidPath, cmd.Process.Pid, opts.AgentPort, opts.ConfigPath)

	if err := WaitForPort(opts.AgentPort, 30*time.Second); err != nil {
		cmd.Process.Kill()
		RemovePIDFile(p.pidPath)
		return nil, fmt.Errorf("agent failed to start: %w", err)
	}

	return p, nil
}

func (p *Process) Stop() error {
	if p.cmd.Process != nil {
		p.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- p.cmd.Wait() }()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			p.cmd.Process.Kill()
		}
	}
	RemovePIDFile(p.pidPath)
	if p.logFile != nil {
		p.logFile.Close()
	}
	return nil
}

func (p *Process) PID() int {
	if p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

func WaitForPort(port int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	backoff := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(backoff)
		backoff = backoff * 2
		if backoff > 5*time.Second {
			backoff = 5 * time.Second
		}
	}
	return fmt.Errorf("port %d not ready after %s", port, timeout)
}
