package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strconv"
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

// DeliveryError exposes the failed stage and SMTP status without logging a
// provider response that may quote credentials or recipient details.
type DeliveryError struct {
	Stage string
	cause error
}

func (e *DeliveryError) Error() string {
	var rejection *textproto.Error
	if errors.As(e.cause, &rejection) {
		return fmt.Sprintf("SMTP %s failed (status %d)", e.Stage, rejection.Code)
	}
	return "SMTP " + e.Stage + " failed"
}

func (e *DeliveryError) Unwrap() error { return e.cause }

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
	fromName := strings.TrimSpace(os.Getenv("SMTP_FROM_NAME"))
	if address == "" && username == "" && password == "" {
		return nil, nil
	}
	if address == "" || from == "" || (username == "") != (password == "") {
		return nil, fmt.Errorf("SMTP_ADDR and SMTP_FROM are required, and SMTP_USERNAME/SMTP_PASSWORD must be provided together")
	}
	host, port, err := net.SplitHostPort(address)
	portNumber, portError := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, " \t\r\n/?#@") || portError != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("SMTP_ADDR must be a host:port address")
	}
	sender, err := mail.ParseAddress(from)
	if err != nil || unsafeHeader(from) {
		return nil, fmt.Errorf("SMTP_FROM must be a valid email address")
	}
	if unsafeHeader(fromName) {
		return nil, fmt.Errorf("SMTP_FROM_NAME must not contain line breaks")
	}
	if fromName != "" {
		sender.Name = fromName
	}
	return &SMTP{address: address, host: host, implicitTLS: portNumber == 465, username: username, password: password, from: sender.String()}, nil
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
		return &DeliveryError{Stage: "session initialization", cause: err}
	}
	defer client.Close()
	if !s.implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
				return &DeliveryError{Stage: "TLS negotiation", cause: err}
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
			return &DeliveryError{Stage: "authentication", cause: err}
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return &DeliveryError{Stage: "sender", cause: err}
	}
	if err := client.Rcpt(to.Address); err != nil {
		return &DeliveryError{Stage: "recipient", cause: err}
	}
	writer, err := client.Data()
	if err != nil {
		return &DeliveryError{Stage: "message submission", cause: err}
	}
	toHeader := to.Address
	if strings.TrimSpace(message.ToName) != "" {
		toHeader = (&mail.Address{Name: strings.TrimSpace(message.ToName), Address: to.Address}).String()
	}
	if from.Name == "" {
		from.Name = "SamenKlaar"
	}
	fromHeader := from.String()
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
		return &DeliveryError{Stage: "message acceptance", cause: err}
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
