package api

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	agentfs "github.com/anthropics/lingtai-portal/internal/fs"
)

type Server struct {
	httpServer *http.Server
	port       int
	baseDir    string
	cancel     context.CancelFunc
	done       chan struct{}
}

func NewServer(baseDir string, staticFS fs.FS) *Server {
	mux := http.NewServeMux()
	mux.Handle("/api/network", NewNetworkHandler(baseDir))
	mux.Handle("/api/topology", NewTopologyHandler(baseDir))
	if staticFS != nil {
		mux.Handle("/", http.FileServer(http.FS(staticFS)))
	}
	return &Server{
		httpServer: &http.Server{Handler: mux},
		baseDir:    baseDir,
	}
}

func (s *Server) Start(portFile string, fixedPort int) error {
	addr := "0.0.0.0:0"
	if fixedPort > 0 {
		addr = fmt.Sprintf("0.0.0.0:%d", fixedPort)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	if portFile != "" {
		os.WriteFile(portFile, []byte(fmt.Sprintf("%d", s.port)), 0o644)
	}
	go s.httpServer.Serve(ln)
	return nil
}

// StartRecording begins a background goroutine that snapshots the network
// topology every 3 seconds, writing to .portal/topology.jsonl.
func (s *Server) StartRecording(baseDir string) {
	topologyPath := filepath.Join(baseDir, ".portal", "topology.jsonl")
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		// Record immediately on start.
		// If the tape is empty and agents already exist, backdate the first
		// frame to the earliest agent birth so replay covers the full history.
		if network, err := agentfs.BuildNetwork(baseDir); err == nil {
			tapeEmpty := true
			if info, err := os.Stat(topologyPath); err == nil && info.Size() > 0 {
				tapeEmpty = false
			}
			if tapeEmpty {
				if earliest := earliestBirth(baseDir); earliest > 0 {
					AppendTopologyAt(topologyPath, network, earliest)
				}
			}
			AppendTopology(topologyPath, network)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				network, err := agentfs.BuildNetwork(baseDir)
				if err != nil {
					continue
				}
				AppendTopology(topologyPath, network)
			}
		}
	}()
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) URL() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

func (s *Server) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
		<-s.done
	}
	return s.httpServer.Shutdown(ctx)
}

// earliestBirth scans agent directories and returns the oldest birth time
// as unix milliseconds. Returns 0 if no agents have a determinable birth time.
func earliestBirth(baseDir string) int64 {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return 0
	}
	var earliest int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t, err := agentfs.BirthTime(filepath.Join(baseDir, entry.Name()))
		if err != nil {
			continue
		}
		ms := t.UnixMilli()
		if earliest == 0 || ms < earliest {
			earliest = ms
		}
	}
	return earliest
}
