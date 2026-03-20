package agent

import (
	"net"
	"testing"
	"time"
)

func TestWaitForPort_AlreadyOpen(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:19910")
	defer ln.Close()

	err := WaitForPort(19910, 2*time.Second)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestWaitForPort_Timeout(t *testing.T) {
	err := WaitForPort(19911, 500*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}
