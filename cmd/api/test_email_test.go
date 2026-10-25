package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"testing"

	emailadapter "github.com/allanzico/samen-klaar/internal/adapters/email"
)

type testMailSender struct {
	message emailadapter.Message
	calls   int
	err     error
}

func (s *testMailSender) Send(ctx context.Context, message emailadapter.Message) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing delivery deadline")
	}
	s.calls++
	s.message = message
	return s.err
}

func TestTestEmailSendsOneGenericMessageWithoutLoggingRecipient(t *testing.T) {
	sender := &testMailSender{}
	var output bytes.Buffer
	if err := sendTestEmail(sender, "recipient@example.test", &output); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 || sender.message.To != "recipient@example.test" || sender.message.Subject != "SamenKlaar email delivery test" || !strings.Contains(sender.message.Body, "No customer or job details") {
		t.Fatal("test mail did not use the intended single, generic message")
	}
	if !strings.Contains(output.String(), "accepted") || strings.Contains(output.String(), "recipient@example.test") {
		t.Fatal("delivery result must acknowledge relay acceptance without logging the recipient")
	}
}

func TestTestEmailRejectsInvalidRecipientsBeforeSending(t *testing.T) {
	for _, recipient := range []string{"", "not an email", "one@example.test,two@example.test", "one@example.test\r\nBcc: two@example.test"} {
		sender := &testMailSender{}
		if err := sendTestEmail(sender, recipient, &bytes.Buffer{}); err == nil || sender.calls != 0 {
			t.Fatal("invalid or multiple recipients reached the relay")
		}
	}
}

func TestTestEmailDoesNotExposeProviderErrors(t *testing.T) {
	for _, cause := range []error{
		fmt.Errorf("SMTP auth: %w", &textproto.Error{Code: 535, Msg: "private-password recipient@example.test"}),
		errors.New("private-password recipient@example.test"),
	} {
		err := sendTestEmail(&testMailSender{err: cause}, "recipient@example.test", &bytes.Buffer{})
		if err == nil || strings.Contains(err.Error(), "private-password") || strings.Contains(err.Error(), "recipient@example.test") {
			t.Fatal("provider details leaked through test-mail failure")
		}
	}
}
