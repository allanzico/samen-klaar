package email

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestDeliveryErrorPreservesSMTPStatusWithoutProviderDetails(t *testing.T) {
	err := &DeliveryError{Stage: "sender", cause: &textproto.Error{Code: 550, Msg: "private-password sender@example.test"}}
	var rejection *textproto.Error
	if !errors.As(err, &rejection) || rejection.Code != 550 || !strings.Contains(err.Error(), "sender") || !strings.Contains(err.Error(), "550") {
		t.Fatal("delivery stage/status was lost")
	}
	if strings.Contains(err.Error(), "private-password") || strings.Contains(err.Error(), "sender@example.test") {
		t.Fatal("provider response leaked through the delivery error")
	}
}

func TestNewSMTPFromEnv(t *testing.T) {
	t.Run("absent configuration disables delivery", func(t *testing.T) {
		clearSMTPEnv(t)
		sender, err := NewSMTPFromEnv()
		if err != nil || sender != nil {
			t.Fatalf("NewSMTPFromEnv() = %v, %v; want nil sender without error", sender, err)
		}
	})

	t.Run("partial configuration fails closed", func(t *testing.T) {
		clearSMTPEnv(t)
		t.Setenv("SMTP_ADDR", "mail.example.test:587")
		if _, err := NewSMTPFromEnv(); err == nil {
			t.Fatal("partial SMTP configuration was accepted")
		}
	})

	t.Run("complete configuration is parsed", func(t *testing.T) {
		clearSMTPEnv(t)
		t.Setenv("SMTP_ADDR", "mail.example.test:587")
		t.Setenv("SMTP_FROM", "notifications@example.test")
		t.Setenv("SMTP_USERNAME", "user")
		t.Setenv("SMTP_PASSWORD", "secret")
		sender, err := NewSMTPFromEnv()
		if err != nil || sender == nil || sender.host != "mail.example.test" {
			t.Fatalf("SMTP configuration was not parsed: %v", err)
		}
	})

	t.Run("custom STARTTLS port and sender name", func(t *testing.T) {
		clearSMTPEnv(t)
		t.Setenv("SMTP_ADDR", "mail.example.test:2525")
		t.Setenv("SMTP_FROM", "notifications@example.test")
		t.Setenv("SMTP_FROM_NAME", "SamenKlaar Updates")
		sender, err := NewSMTPFromEnv()
		if err != nil || sender == nil || sender.implicitTLS {
			t.Fatalf("custom STARTTLS configuration rejected: %v", err)
		}
		from, err := mail.ParseAddress(sender.from)
		if err != nil || from.Name != "SamenKlaar Updates" || from.Address != "notifications@example.test" {
			t.Fatal("configured sender name/address was not preserved")
		}
	})

	t.Run("port 465 selects implicit TLS", func(t *testing.T) {
		clearSMTPEnv(t)
		t.Setenv("SMTP_ADDR", "mail.example.test:465")
		t.Setenv("SMTP_FROM", "notifications@example.test")
		sender, err := NewSMTPFromEnv()
		if err != nil || sender == nil || !sender.implicitTLS {
			t.Fatalf("implicit TLS configuration rejected: %v", err)
		}
	})

	for _, address := range []string{"mail.example.test", "mail.example.test:0", "mail.example.test:65536", "mail.example.test:smtp", "https://mail.example.test:587"} {
		t.Run("reject invalid endpoint "+address, func(t *testing.T) {
			clearSMTPEnv(t)
			t.Setenv("SMTP_ADDR", address)
			t.Setenv("SMTP_FROM", "notifications@example.test")
			if _, err := NewSMTPFromEnv(); err == nil {
				t.Fatal("invalid SMTP endpoint accepted")
			}
		})
	}

	t.Run("sender name rejects header injection", func(t *testing.T) {
		clearSMTPEnv(t)
		t.Setenv("SMTP_ADDR", "mail.example.test:2525")
		t.Setenv("SMTP_FROM", "notifications@example.test")
		t.Setenv("SMTP_FROM_NAME", "Updates\r\nBcc: attacker@example.test")
		if _, err := NewSMTPFromEnv(); err == nil {
			t.Fatal("sender header injection accepted")
		}
	})
}

func TestSMTPSendUsesPlainTextMessageAndRejectsHeaderInjection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	messageReceived := make(chan string, 1)
	serverError := make(chan error, 1)
	go serveTestSMTP(listener, messageReceived, serverError)

	address := listener.Addr().String()
	clearSMTPEnv(t)
	t.Setenv("SMTP_ADDR", address)
	t.Setenv("SMTP_FROM", "notifications@example.test")
	t.Setenv("SMTP_FROM_NAME", "SamenKlaar Updates")
	sender, err := NewSMTPFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = sender.Send(ctx, Message{To: "customer@example.test", Subject: "Request update", Body: "Your request has an update.", MessageID: "abc123@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case received := <-messageReceived:
		for _, value := range []string{"To: customer@example.test", `From: "SamenKlaar Updates" <notifications@example.test>`, "Subject: Request update", "Message-ID: <abc123@example.test>", "Your request has an update."} {
			if !strings.Contains(received, value) {
				t.Fatalf("message does not contain %q: %s", value, received)
			}
		}
	case err := <-serverError:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("SMTP server did not receive a message")
	}

	err = sender.Send(ctx, Message{To: "customer@example.test", Subject: "ok\r\nBcc: attacker@example.test", Body: "body"})
	if err == nil {
		t.Fatal("header injection was accepted")
	}
}

func clearSMTPEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SMTP_ADDR", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_FROM_NAME"} {
		t.Setenv(key, "")
	}
}

func serveTestSMTP(listener net.Listener, received chan<- string, serverError chan<- error) {
	conn, err := listener.Accept()
	if err != nil {
		serverError <- err
		return
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	if _, err := fmt.Fprint(conn, "220 localhost ESMTP\r\n"); err != nil {
		serverError <- err
		return
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			serverError <- err
			return
		}
		command := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(command, "EHLO "):
			_, err = fmt.Fprint(conn, "250 localhost\r\n")
		case strings.HasPrefix(command, "MAIL FROM:"):
			_, err = fmt.Fprint(conn, "250 sender ok\r\n")
		case strings.HasPrefix(command, "RCPT TO:"):
			_, err = fmt.Fprint(conn, "250 recipient ok\r\n")
		case command == "DATA":
			_, err = fmt.Fprint(conn, "354 send data\r\n")
			if err == nil {
				var message strings.Builder
				for {
					dataLine, readErr := reader.ReadString('\n')
					if readErr != nil {
						serverError <- readErr
						return
					}
					if dataLine == ".\r\n" {
						break
					}
					message.WriteString(strings.TrimSuffix(dataLine, "\r\n"))
					message.WriteByte('\n')
				}
				received <- message.String()
				_, err = fmt.Fprint(conn, "250 queued\r\n")
			}
		case command == "QUIT":
			_, _ = fmt.Fprint(conn, "221 goodbye\r\n")
			return
		default:
			_, err = fmt.Fprint(conn, "250 ok\r\n")
		}
		if err != nil {
			serverError <- err
			return
		}
	}
}
