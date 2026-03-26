package api

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
)

type Server struct {
	httpServer *http.Server
	port       int
}

func NewServer(baseDir string, staticFS fs.FS) *Server {
	mux := http.NewServeMux()
	mux.Handle("/api/network", NewNetworkHandler(baseDir))
	if staticFS != nil {
		mux.Handle("/", http.FileServer(http.FS(staticFS)))
	}
	return &Server{
		httpServer: &http.Server{Handler: mux},
	}
}

func (s *Server) Start(portFile string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
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

func (s *Server) Port() int {
	return s.port
}

func (s *Server) URL() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
