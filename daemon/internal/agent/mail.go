package agent

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// MailClient sends messages to a lingtai TCP mail server.
type MailClient struct {
	address string
}

func NewMailClient(address string) *MailClient {
	return &MailClient{address: address}
}

// Send connects, reads banner, sends a length-prefixed JSON message.
// Each send is a fresh TCP connection (connect -> read banner -> send -> close).
// No persistent connection needed since user messages are infrequent.
func (c *MailClient) Send(msg map[string]interface{}) error {
	conn, err := net.Dial("tcp", c.address)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", c.address, err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	_, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read banner: %w", err)
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := binary.Write(conn, binary.BigEndian, uint32(len(payload))); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

// MailListener listens for incoming TCP mail messages.
type MailListener struct {
	listener net.Listener
	handler  func(map[string]interface{})
	wg       sync.WaitGroup
	done     chan struct{}
}

func NewMailListener(port int, handler func(map[string]interface{})) (*MailListener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}
	ml := &MailListener{
		listener: ln,
		handler:  handler,
		done:     make(chan struct{}),
	}
	ml.wg.Add(1)
	go ml.acceptLoop()
	return ml, nil
}

func (ml *MailListener) Stop() {
	close(ml.done)
	ml.listener.Close()
	ml.wg.Wait()
}

func (ml *MailListener) acceptLoop() {
	defer ml.wg.Done()
	for {
		conn, err := ml.listener.Accept()
		if err != nil {
			select {
			case <-ml.done:
				return
			default:
				continue
			}
		}
		go ml.handleConn(conn)
	}
}

func (ml *MailListener) handleConn(conn net.Conn) {
	defer conn.Close()

	fmt.Fprintf(conn, "LINGTAI daemon\n")

	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return
	}
	if length == 0 || length > 10*1024*1024 {
		return
	}
	buf := make([]byte, length)
	if _, err := readFull(conn, buf); err != nil {
		return
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(buf, &msg); err != nil {
		return
	}
	ml.handler(msg)
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
