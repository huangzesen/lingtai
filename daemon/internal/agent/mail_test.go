package agent

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

// mockServer simulates a TCPMailService server.
func mockServer(t *testing.T, port int, banner string, handler func(net.Conn)) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprintf(conn, "LINGTAI %s\n", banner)
		handler(conn)
	}()
	return ln
}

func TestMailClient_Send(t *testing.T) {
	port := 19901
	received := make(chan map[string]interface{}, 1)

	ln := mockServer(t, port, "test-banner", func(conn net.Conn) {
		var length uint32
		binary.Read(conn, binary.BigEndian, &length)
		buf := make([]byte, length)
		conn.Read(buf)
		var msg map[string]interface{}
		json.Unmarshal(buf, &msg)
		received <- msg
	})
	defer ln.Close()

	time.Sleep(50 * time.Millisecond)

	client := NewMailClient(fmt.Sprintf("127.0.0.1:%d", port))
	err := client.Send(map[string]interface{}{
		"from":    "cli@localhost:19902",
		"message": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-received:
		if msg["message"] != "hello" {
			t.Errorf("got message %q, want %q", msg["message"], "hello")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestMailClient_SendBannerRead(t *testing.T) {
	port := 19902
	bannerRead := make(chan bool, 1)

	ln := mockServer(t, port, "my-banner-123", func(conn net.Conn) {
		var length uint32
		err := binary.Read(conn, binary.BigEndian, &length)
		if err == nil && length > 0 && length < 10000 {
			bannerRead <- true
		} else {
			bannerRead <- false
		}
	})
	defer ln.Close()

	time.Sleep(50 * time.Millisecond)

	client := NewMailClient(fmt.Sprintf("127.0.0.1:%d", port))
	client.Send(map[string]interface{}{"message": "test"})

	select {
	case ok := <-bannerRead:
		if !ok {
			t.Error("banner was not properly read before sending")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestMailListener_Receive(t *testing.T) {
	port := 19903
	received := make(chan map[string]interface{}, 1)

	listener, err := NewMailListener(port, func(msg map[string]interface{}) {
		received <- msg
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	banner := string(buf[:n])
	if len(banner) == 0 || banner[:5] != "LINGTAI" {
		t.Errorf("expected LINGTAI banner, got %q", banner)
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"from":    "orchestrator",
		"message": "reply text",
	})
	binary.Write(conn, binary.BigEndian, uint32(len(payload)))
	conn.Write(payload)

	select {
	case msg := <-received:
		if msg["message"] != "reply text" {
			t.Errorf("got %q, want %q", msg["message"], "reply text")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}
