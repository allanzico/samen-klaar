package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// Message contains a transactional notification. Callers must not include
// private job details in the subject or body.
type Message struct {
	To        string
	ToName    string
	From      string
	Subject   string
	Body      string
	MessageID string
}

type Sender interface {
	Send(context.Context, Message) error
}

type SMTP struct {
	address     string
	host        string
	implicitTLS bool
	username    string
	password    string
	from        string
}

// NewSMTPFromEnv accepts an absent SMTP configuration (no address or
// credentials): the app runs and notification emails stay queued. A partial
// configuration is an error so it cannot silently claim delivery is configured.
func NewSMTPFromEnv() (*SMTP, error) {
	address := strings.TrimSpace(os.Getenv("SMTP_ADDR"))
	username := strings.TrimSpace(os.Getenv("SMTP_USERNAME"))
	password := os.Getenv("SMTP_PASSWORD")
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if address == "" && username == "" && password == "" {
		return nil, nil
	}
	if address == "" || from == "" || (username == "") != (password == "") {
		return nil, fmt.Errorf("SMTP_ADDR and SMTP_FROM are required, and SMTP_USERNAME/SMTP_PASSWORD must be provided together")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return nil, fmt.Errorf("SMTP_ADDR must be a host:port address")
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return nil, fmt.Errorf("SMTP_FROM must be a valid email address")
	}
	return &SMTP{address: address, host: host, implicitTLS: port == "465", username: username, password: password, from: from}, nil
}

func (s *SMTP) Send(ctx context.Context, message Message) error {
	if s == nil {
		return fmt.Errorf("email delivery is not configured")
	}
	to, err := mail.ParseAddress(message.To)
	if err != nil || to.Address == "" {
		return fmt.Errorf("notification recipient is not a valid email address")
	}
	from, err := mail.ParseAddress(s.from)
	if err != nil {
		return fmt.Errorf("notification sender is not a valid email address")
	}
	if unsafeHeader(message.Subject) || unsafeHeader(message.ToName) || unsafeHeader(message.MessageID) {
		return fmt.Errorf("notification contains an invalid email header")
	}
	if message.Subject == "" || message.Body == "" {
		return fmt.Errorf("notification subject and body are required")
	}

	dialer := net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if s.implicitTLS {
		tlsDialer := tls.Dialer{NetDialer: &dialer, Config: &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", s.address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", s.address)
	}
	if err != nil {
		return fmt.Errorf("connect to notification mail server: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(20 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = conn.SetDeadline(deadline)
	stopDeadline := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stopDeadline()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("initialize notification mail session: %w", err)
	}
	defer client.Close()
	if !s.implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("secure notification mail session: %w", err)
			}
		} else if !isLoopbackHost(s.host) {
			return fmt.Errorf("notification mail server does not support STARTTLS")
		}
	}
	if s.username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("notification mail server does not support authentication")
		}
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fmt.Errorf("authenticate with notification mail server: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("set notification sender: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("set notification recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("start notification message: %w", err)
	}
	toHeader := to.Address
	if strings.TrimSpace(message.ToName) != "" {
		toHeader = (&mail.Address{Name: strings.TrimSpace(message.ToName), Address: to.Address}).String()
	}
	fromHeader := (&mail.Address{Name: "SamenKlaar", Address: from.Address}).String()
	data := "To: " + toHeader + "\r\n" +
		"From: " + fromHeader + "\r\n" +
		"Subject: " + message.Subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n"
	if message.MessageID != "" {
		data += "Message-ID: <" + message.MessageID + ">\r\n"
	}
	if _, err := writer.Write([]byte(data + "\r\n")); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write notification message: %w", err)
	}
	bodyWriter := quotedprintable.NewWriter(writer)
	if _, err := bodyWriter.Write([]byte(message.Body)); err != nil {
		_ = bodyWriter.Close()
		_ = writer.Close()
		return fmt.Errorf("write notification body: %w", err)
	}
	if err := bodyWriter.Close(); err != nil {
		_ = writer.Close()
		return fmt.Errorf("encode notification body: %w", err)
	}
	if _, err := writer.Write([]byte("\r\n")); err != nil {
		_ = writer.Close()
		return fmt.Errorf("finish notification body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish notification message: %w", err)
	}
	_ = client.Quit()
	return nil
}

func unsafeHeader(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
