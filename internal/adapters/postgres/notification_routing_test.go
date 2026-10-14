package postgres

import (
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
