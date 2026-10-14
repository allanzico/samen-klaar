package workflow

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
)

func pendingPrice(job Job) bool {
	return job.PriceProposal != nil && job.PriceProposal.Status == "pending"
}

func applyNegotiation(state *State, input Action) error {
	job := &state.Job
	if job.ID == "" || job.WorkStarted || job.Paid || input.Actor != "customer" && input.Actor != "professional" {
		return errors.New("price and date planning are available before work starts")
	}
	switch input.Action {
	case "withdraw-price":
		p := job.PriceProposal
		if job.QuoteApproved || p == nil || p.Status != "pending" || p.Actor != input.Actor || input.ProposalID != p.ID {
			return errors.New("only the proposer can withdraw the current price offer")
		}
		p.Status = "withdrawn"
		addEvent(job, input.Actor+" withdrew the price offer; the current price is unchanged", "done")
	case "withdraw-date":
		p := job.DateProposal
		if p == nil || p.Status != "pending" || p.Actor != input.Actor || input.ProposalID != p.ID {
			return errors.New("only the proposer can withdraw the current visit proposal")
		}
		p.Status = "withdrawn"
		addEvent(job, input.Actor+" withdrew the visit proposal; any existing booking is unchanged", "done")

	case "propose-price":
		if job.QuoteApproved || job.Appointment != nil || input.PriceAmountMinor <= 0 || input.PriceAmountMinor > 100_000_000 {
			return errors.New("propose a valid total price before the plan is approved")
		}
		note := strings.TrimSpace(input.Value)
		if len(note) < 3 || len(note) > 500 {
			return errors.New("explain the price proposal in 3 to 500 characters")
		}
		job.PriceProposal = &PriceProposal{ID: rand.Text(), Status: "pending", Actor: input.Actor, AmountMinor: input.PriceAmountMinor, Note: note, ScopeVersion: job.ScopeVersion}
		addEvent(job, fmt.Sprintf("%s proposed %s %.2f total: %s", input.Actor, job.Quote.Currency, float64(input.PriceAmountMinor)/100, note), "attention")
	case "accept-price", "decline-price":
		p := job.PriceProposal
		if job.QuoteApproved || p == nil || p.Status != "pending" || p.Actor == input.Actor || p.ScopeVersion != job.ScopeVersion || input.ProposalID != p.ID {
			return errors.New("only the other party can decide on the current price proposal")
		}
		if input.Action == "decline-price" {
			p.Status = "declined"
			addEvent(job, input.Actor+" declined the price proposal; the current quote is unchanged", "done")
			return nil
		}
		p.Status = "accepted"
		job.Quote.Lines = append(job.Quote.Lines, QuoteLine{Label: "Agreed price adjustment", AmountMinor: p.AmountMinor - job.Quote.TotalMinor, Basis: "Customer and professional agreed: " + p.Note})
		job.Quote.TotalMinor = p.AmountMinor
		job.ScopeVersion++
		job.ScopeHistory = append(job.ScopeHistory, scopeSnapshot(*job))
		addEvent(job, "Both parties agreed the new total; customer plan approval is still required", "done")
	case "propose-date":
		if !job.QuoteApproved || input.ProposedAppointment == nil {
			return errors.New("approve the plan before proposing a visit time")
		}
		slot, err := ValidateProposedAppointment(*input.ProposedAppointment, job.Quote.DurationMinutes)
		if err != nil {
			return err
		}
		note := strings.TrimSpace(input.Value)
		if len(note) > 500 {
			return errors.New("date proposal note must be 500 characters or fewer")
		}
		job.DateProposal = &DateProposal{ID: rand.Text(), Status: "pending", Actor: input.Actor, Note: note, Appointment: slot}
		addEvent(job, input.Actor+" proposed a visit: "+slot.StartsAt+" · "+slot.Timezone+" "+note, "attention")
	case "accept-date", "decline-date":
		p := job.DateProposal
		if !job.QuoteApproved || p == nil || p.Status != "pending" || p.Actor == input.Actor || input.ProposalID != p.ID {
			return errors.New("only the other party can decide on the current date proposal")
		}
		if input.Action == "decline-date" {
			p.Status = "declined"
			addEvent(job, input.Actor+" declined the proposed visit time; any existing booking is unchanged", "done")
			return nil
		}
		slot, err := ValidateProposedAppointment(p.Appointment, job.Quote.DurationMinutes)
		if err != nil {
			return err
		}
		if serviceAddressRequired(*state) && job.ServiceAddress == "" {
			return errors.New("add the full service address before confirming the visit")
		}
		p.Status = "accepted"
		job.Appointment = &slot
		job.AppointmentConfirmation = &AppointmentConfirmation{Status: "confirmed", RespondedAt: time.Now().UTC().Format(time.RFC3339), Appointment: slot}
		addEvent(job, "Both parties agreed the visit: "+slot.StartsAt+" · "+slot.Timezone, "done")
	default:
		return errors.New("unknown negotiation action")
	}
	return nil
}

func serviceAddressRequired(state State) bool {
	// Workspaces created before area matching are grandfathered; every new
	// customer intake has a service area and therefore requires an address.
	return state.Intake != nil && state.Intake.Location != nil
}

func ValidateProposedAppointment(slot Appointment, durationMinutes int) (Appointment, error) {
	start, startErr := time.Parse(time.RFC3339, slot.StartUTC)
	end, endErr := time.Parse(time.RFC3339, slot.EndUTC)
	zone, zoneErr := time.LoadLocation(slot.Timezone)
	if startErr != nil || endErr != nil || zoneErr != nil || start.Before(time.Now().Add(15*time.Minute)) || start.After(time.Now().AddDate(1, 0, 0)) || end.Sub(start) < time.Duration(durationMinutes)*time.Minute || end.Sub(start) < 15*time.Minute || end.Sub(start) > 6*time.Hour {
		return Appointment{}, errors.New("choose a future visit within the next year, long enough for the service and no longer than six hours, with a valid timezone")
	}
	slot.SlotID = "" // The database allocates or reuses a real availability slot at acceptance.
	slot.StartUTC, slot.EndUTC = start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
	slot.StartsAt = start.In(zone).Format("Mon 02 Jan 2006, 15:04") + "–" + end.In(zone).Format("15:04")
	return slot, nil
}
