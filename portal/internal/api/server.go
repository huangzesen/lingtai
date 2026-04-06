package api

import (
	"bytes"
	"context"
	"encoding/json"
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
	mux.Handle("/api/topology/manifest", NewManifestHandler(baseDir))
	mux.Handle("/api/topology/chunk", NewChunkHandler(baseDir))
	mux.Handle("/api/topology/rebuild", NewRebuildHandler(baseDir))
	mux.Handle("/api/topology/progress", NewProgressHandler(baseDir))
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

		// Check if tape needs reconstruction
		if needsReconstruction(topologyPath) {
			replayDir := filepath.Join(baseDir, ".portal", "replay", "chunks")
			progressPath := filepath.Join(baseDir, ".portal", "reconstruct.progress")
			os.WriteFile(progressPath, []byte("0/0"), 0o644)

			frames, err := agentfs.ReconstructTape(baseDir)
			if err == nil && len(frames) > 0 {
				os.MkdirAll(filepath.Dir(topologyPath), 0o755)
				os.Remove(topologyPath)
				os.RemoveAll(filepath.Join(baseDir, ".portal", "replay"))
				os.MkdirAll(replayDir, 0o755)

				// Stream frames directly into hourly compressed chunks
				total := len(frames)
				var currentHour int64 = -1
				var hourFrames []agentfs.TapeFrame
				var chunks []ChunkInfo

				flushHour := func() {
					if len(hourFrames) == 0 {
						return
					}
					info := ChunkInfo{
						Start:  currentHour,
						End:    hourFrames[len(hourFrames)-1].T,
						Frames: len(hourFrames),
					}
					chunks = append(chunks, info)
					cachePath := filepath.Join(replayDir, fmt.Sprintf("%d.json.gz", currentHour))
					chunk := deltaEncode(hourFrames, defaultKeyframeInterval)
					writeChunkCache(cachePath, chunk)
					hourFrames = nil
				}

				for i, f := range frames {
					bucket := hourBucket(f.T)
					if bucket != currentHour {
						flushHour()
						currentHour = bucket
					}
					hourFrames = append(hourFrames, f)
					if i%1000 == 0 || i == total-1 {
						os.WriteFile(progressPath, []byte(fmt.Sprintf("%d/%d", i+1, total)), 0o644)
					}
				}
				flushHour()

				// Write minimal topology.jsonl with just the last frame (for live recording to append to)
				if len(frames) > 0 {
					lastFrame := frames[len(frames)-1]
					line, _ := json.Marshal(lastFrame)
					os.WriteFile(topologyPath, append(line, '\n'), 0o644)
				}

				// Write manifest cache
				if len(chunks) > 0 {
					manifest := ReplayManifest{
						TapeStart: chunks[0].Start,
						TapeEnd:   chunks[len(chunks)-1].End,
						Chunks:    chunks,
					}
					mdata, _ := json.Marshal(manifest)
					os.WriteFile(filepath.Join(replayDir, "manifest.json"), mdata, 0o644)
				}
			}
			os.Remove(progressPath)
		}

		// Record current state immediately
		if network, err := agentfs.BuildNetwork(baseDir); err == nil {
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

// needsReconstruction checks if topology.jsonl is missing, empty,
// or uses the old format (missing direct/cc/bcc on mail edges).
func needsReconstruction(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return true
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) == 0 {
		return true
	}
	lastLine := lines[len(lines)-1]
	var frame struct {
		Net struct {
			MailEdges []struct {
				Direct *int `json:"direct"`
			} `json:"mail_edges"`
		} `json:"net"`
	}
	if json.Unmarshal(lastLine, &frame) != nil {
		return true
	}
	if len(frame.Net.MailEdges) == 0 {
		return false
	}
	return frame.Net.MailEdges[0].Direct == nil
}
