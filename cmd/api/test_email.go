package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	emailadapter "github.com/allanzico/samen-klaar/internal/adapters/email"
)

// This command opens only SMTP: it never starts workers or drains queued mail.
// Stdin keeps a test recipient out of command histories and process arguments.
func testEmailCommand(args []string, input io.Reader, output io.Writer) error {
	if len(args) > 1 {
		return errors.New("usage: api test-email [recipient]; omit recipient to read it from stdin")
	}
	recipient := ""
	if len(args) == 1 {
		recipient = args[0]
	} else {
		scanner := bufio.NewScanner(io.LimitReader(input, 1024))
		if scanner.Scan() {
			recipient = scanner.Text()
		}
		if scanner.Err() != nil {
			return errors.New("could not read test email recipient")
		}
	}
	sender, err := emailadapter.NewSMTPFromEnv()
	if err != nil {
		return err
	}
	if sender == nil {
		return errors.New("configure SMTP_ADDR and SMTP_FROM before sending a test email")
	}
	return sendTestEmail(sender, recipient, output)
}

func sendTestEmail(sender emailadapter.Sender, recipient string, output io.Writer) error {
	recipient = strings.TrimSpace(recipient)
	address, err := mail.ParseAddress(recipient)
	if err != nil || strings.ContainsAny(recipient, "\r\n") {
		return errors.New("provide one valid test email recipient")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = sender.Send(ctx, emailadapter.Message{
		To:      address.Address,
		Subject: "SamenKlaar email delivery test",
		Body:    "This is a test of SamenKlaar's email delivery.\n\nNo customer or job details are included. If you received this message, outbound email delivery is working.\n\nSamenKlaar",
	})
	if err != nil {
		// SMTP errors can quote account/recipient details. Report only the status.
		var rejection *textproto.Error
		if errors.As(err, &rejection) {
			stage := "delivery"
			var failure *emailadapter.DeliveryError
			if errors.As(err, &failure) {
				stage = failure.Stage
			}
			return fmt.Errorf("SMTP relay rejected the test email during %s (status %d)", stage, rejection.Code)
		}
		return errors.New("test email failed; check SMTP connectivity, TLS and authentication")
	}
	_, err = fmt.Fprintln(output, "SMTP relay accepted the test email. Confirm arrival in the recipient inbox or spam folder.")
	return err
}
