package postgres

import (
	"strings"
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestAppointmentDecisionsNotifyCustomer(t *testing.T) {
	for _, action := range []string{"confirm-appointment", "decline-appointment"} {
		t.Run(action, func(t *testing.T) {
			event, recipient, ok := notificationForAction(workflow.Action{Action: action, Actor: "professional"}, workflow.Job{ID: "job"})
			if !ok || event != action || recipient != "customer" {
				t.Fatalf("event=%q recipient=%q ok=%v", event, recipient, ok)
			}
		})
	}
}

func TestAgentQuestionNotificationCopy(t *testing.T) {
	link := "https://app.example/?workspaceId=ws"
	for locale, want := range map[string]struct{ subject, body string }{
		"en": {"Your agent needs your decision", "Your agent stopped on a service request"},
		"nl": {"Je agent heeft je beslissing nodig", "Je agent is gestopt bij een aanvraag"},
	} {
		subject, body := notificationSubject(agentQuestionEvent, locale), notificationBody(agentQuestionEvent, link, locale)
		if subject != want.subject || !strings.Contains(body, want.body) || !strings.Contains(body, link) {
			t.Fatalf("%s: subject=%q body=%q", locale, subject, body)
		}
	}
	if agentQuestionID("agent-question:42") != 42 || agentQuestionID("agent-question:x") != 0 {
		t.Fatal("agent question id is not read from the event key")
	}
}
