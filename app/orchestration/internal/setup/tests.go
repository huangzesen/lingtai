package setup

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"
)

type TestResult struct {
	OK      bool
	Message string
}

func TestEnvVar(name string) TestResult {
	val := os.Getenv(name)
	if val == "" {
		return TestResult{OK: false, Message: fmt.Sprintf("%s is not set", name)}
	}
	return TestResult{OK: true, Message: fmt.Sprintf("%s is set (%d chars)", name, len(val))}
}

func TestIMAP(host string, port int, user, pass string) TestResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp", addr,
		&tls.Config{ServerName: host},
	)
	if err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("IMAP connect failed: %v", err)}
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.Read(buf)

	fmt.Fprintf(conn, "A001 LOGIN %q %q\r\n", user, pass)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("IMAP login failed: %v", err)}
	}
	resp := string(buf[:n])
	if strings.Contains(resp, "OK") {
		fmt.Fprintf(conn, "A002 LOGOUT\r\n")
		return TestResult{OK: true, Message: "IMAP connection successful"}
	}
	return TestResult{OK: false, Message: fmt.Sprintf("IMAP login rejected: %s", resp)}
}

func TestSMTP(host string, port int, user, pass string) TestResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := smtp.Dial(addr)
	if err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("SMTP connect failed: %v", err)}
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("STARTTLS failed: %v", err)}
	}

	auth := smtp.PlainAuth("", user, pass, host)
	if err := client.Auth(auth); err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("SMTP auth failed: %v", err)}
	}

	client.Quit()
	return TestResult{OK: true, Message: "SMTP connection successful"}
}

func TestTelegram(token string) TestResult {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("Telegram API error: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	json.Unmarshal(body, &result)

	if result.OK {
		return TestResult{OK: true, Message: fmt.Sprintf("Telegram bot: @%s", result.Result.Username)}
	}
	return TestResult{OK: false, Message: fmt.Sprintf("Telegram rejected: %s", string(body))}
}
