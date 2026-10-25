package workflow

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// An agent acts for one person on one request, using the same named actions a person would press.
// It only ever acts inside that person's mandate: explicit limits the person set in advance. Anything
// outside the mandate, anything uncertain, and every step that needs a person (a physical visit, a
// buyer approval in PayPal, a physical inspection, a dispute) is escalated instead of guessed.
// Completion review and capture of an existing hold require explicit per-booking permission.
//
// DecideAgent is pure: it reads a snapshot and returns what the agent would do. The runner applies it
// through the normal workflow validation, which re-checks every rule, so a stale snapshot cannot
// commit anything the mandate or the workflow would not allow.

const (
	AgentAct      = "act"
	AgentEscalate = "escalate"
	AgentWait     = "wait"
)

// AgentMandate is the authority a person grants their agent. Money limits are in minor units.
type AgentMandate struct {
	Enabled bool `json:"enabled"`
	Version int  `json:"version,omitempty"`

	// When visits may happen (both roles). Weekdays are ISO 8601 (1 = Monday).
	Weekdays  []int  `json:"weekdays"`
	StartHour int    `json:"startHour"`
	EndHour   int    `json:"endHour"`
	Timezone  string `json:"timezone,omitempty"`
	// The hour a customer agent proposes when none of the professional's free times fit. It snaps to
	// the start of the window when it falls outside it.
	PreferredHour int `json:"preferredHour,omitempty"`

	// Customer agent.
	MaxTotalMinor         int64  `json:"maxTotalMinor,omitempty"`
	MaxDaysAhead          int    `json:"maxDaysAhead,omitempty"`
	ServiceAddress        string `json:"serviceAddress,omitempty"`
	AutoApproveExtraMinor int64  `json:"autoApproveExtraMinor,omitempty"`
	CounterPercent        int    `json:"counterPercent,omitempty"`
	// Legacy work-only preference. New bookings use an explicit BookingAutonomy decision.
	AutoApproveWork bool `json:"autoApproveWork,omitempty"`
	// By default the assistant asks the customer before booking a time. This lets it book on its own.
	BookWithoutAsking bool `json:"bookWithoutAsking,omitempty"`

	// Professional agent.
	FloorPercent       int  `json:"floorPercent,omitempty"`
	AutoConfirmVisits  bool `json:"autoConfirmVisits,omitempty"`
	AutoAcceptDates    bool `json:"autoAcceptDates,omitempty"`
	AcceptOutsideHours bool `json:"acceptOutsideHours,omitempty"`
}

// DefaultMandate is the authority an agent has before its person changes anything: it is on from the
// start, books inside ordinary days and hours, and grants no completion or payment authority.
func DefaultMandate(role string) AgentMandate {
	m := AgentMandate{Enabled: true}
	if role == "customer" {
		m.Weekdays, m.StartHour, m.EndHour, m.PreferredHour = []int{1, 2, 3, 4, 5, 6}, 8, 20, 10
	} else {
		m.AutoConfirmVisits, m.AutoAcceptDates = true, true
	}
	_ = m.Normalize(role)
	return m
}

var budgetAmount = regexp.MustCompile(`\d[\d.,]*`)

// BudgetMinor reads the amount a customer named as their budget in the chat ("My budget is EUR 120",
// "around €1.250,50"), in euro cents, or 0 when they gave no figure ("I'm flexible").
func BudgetMinor(text string) int64 {
	raw := strings.TrimRight(budgetAmount.FindString(text), ".,")
	if raw == "" {
		return 0
	}
	whole, fraction := raw, ""
	if last := max(strings.LastIndex(raw, "."), strings.LastIndex(raw, ",")); last >= 0 {
		after := raw[last+1:]
		mixed := strings.Contains(raw, ".") && strings.Contains(raw, ",")
		repeated := strings.Count(raw, string(raw[last])) > 1
		// "1.250" and "1.250.000" group thousands; "120.50" and "1.250,50" have decimals.
		if mixed || !repeated && len(after) != 3 {
			whole, fraction = raw[:last], after
		}
	}
	value, err := strconv.ParseInt(strings.NewReplacer(".", "", ",", "").Replace(whole), 10, 64)
	if err != nil || value > 1_000_000 {
		return 0
	}
	var cents int64
	if fraction != "" {
		cents, _ = strconv.ParseInt((fraction + "0")[:2], 10, 64)
	}
	return value*100 + cents
}

// agentLimit is the most the customer's agent may agree to for this request: the budget they named in
// the chat, capped by any ceiling in their preferences. Zero means they set neither, so the price is
// theirs to approve.
func agentLimit(c AgentContext) int64 {
	var budget int64
	if c.State.Intake != nil {
		budget = BudgetMinor(c.State.Intake.Budget)
	}
	switch ceiling := c.Mandate.MaxTotalMinor; {
	case budget > 0 && ceiling > 0:
		return min(budget, ceiling)
	case budget > 0:
		return budget
	default:
		return ceiling
	}
}

// Normalize applies defaults and bounds. It returns an error for values that would grant unsafe or
// meaningless authority, so a mandate can never be wider than the product allows.
func (m *AgentMandate) Normalize(role string) error {
	if m.Timezone == "" {
		m.Timezone = "Europe/Amsterdam"
	}
	if _, err := time.LoadLocation(m.Timezone); err != nil {
		return fmt.Errorf("choose a valid time zone")
	}
	if len(m.Weekdays) == 0 {
		m.Weekdays = []int{1, 2, 3, 4, 5}
	}
	slices.Sort(m.Weekdays)
	m.Weekdays = slices.Compact(m.Weekdays)
	for _, day := range m.Weekdays {
		if day < 1 || day > 7 {
			return fmt.Errorf("weekdays must be between 1 (Monday) and 7 (Sunday)")
		}
	}
	if m.StartHour == 0 && m.EndHour == 0 {
		m.StartHour, m.EndHour = 8, 18
	}
	if m.StartHour < 0 || m.EndHour > 24 || m.StartHour >= m.EndHour {
		return fmt.Errorf("the visit window must start before it ends, between 00:00 and 24:00")
	}
	if m.PreferredHour < m.StartHour || m.PreferredHour >= m.EndHour {
		m.PreferredHour = m.StartHour
	}
	switch role {
	case "customer":
		if m.MaxTotalMinor < 0 || m.MaxTotalMinor > 1_000_000 {
			return fmt.Errorf("the price limit must be between 0 and 10,000")
		}
		if m.AutoApproveExtraMinor < 0 || m.MaxTotalMinor > 0 && m.AutoApproveExtraMinor > m.MaxTotalMinor {
			return fmt.Errorf("extra work approved automatically cannot exceed the price limit")
		}
		if m.MaxDaysAhead == 0 {
			m.MaxDaysAhead = 14
		}
		if m.MaxDaysAhead < 1 || m.MaxDaysAhead > 90 {
			return fmt.Errorf("choose a booking horizon between 1 and 90 days")
		}
		if m.CounterPercent == 0 {
			m.CounterPercent = 75
		}
		if m.CounterPercent < 50 || m.CounterPercent > 100 {
			return fmt.Errorf("an agent may counter only when your limit is 50 to 100 percent of the price")
		}
		if len(m.ServiceAddress) > 250 {
			return fmt.Errorf("the service address is too long")
		}
	case "professional":
		if m.FloorPercent == 0 {
			m.FloorPercent = 90
		}
		if m.FloorPercent < 50 || m.FloorPercent > 100 {
			return fmt.Errorf("the lowest acceptable price must be 50 to 100 percent of your published price")
		}
	default:
		return fmt.Errorf("agents act for customers and professionals")
	}
	return nil
}

// An agent sends at most one counter-offer per request: if the other side does not meet it, the people decide.

// AgentContext is everything DecideAgent may consider.
type AgentContext struct {
	ProfessionalSchedule *ProfessionalScheduleView
	State                State
	Role                 string
	Locale               string // the language of the person this agent acts for: "en" or "nl"
	Mandate              AgentMandate
	Now                  time.Time
	Counters             int // price counter-offers this agent already sent on this request
	// Visit times this agent already proposed on this request. Like price, one proposal each, then people decide.
	DateProposals int
	// Avoid lists professional profiles a customer agent must not pick again on this request, because
	// that professional already cancelled it.
	Avoid []string
	// Interpreted is a language model's reading of the reply with this Seq, when one was made.
	Interpreted    *ReplyIntent
	InterpretedSeq int
}

// AgentDecision is one step: act, escalate to the person, or wait for someone else.
// Code and Params describe it for the activity feed, which localizes them.
type AgentDecision struct {
	Kind   string
	Action *Action
	Code   string
	Params map[string]string
}

func wait() AgentDecision { return AgentDecision{Kind: AgentWait} }
func escalate(code string, params map[string]string) AgentDecision {
	return AgentDecision{Kind: AgentEscalate, Code: code, Params: params}
}
func act(code string, action Action, params map[string]string) AgentDecision {
	return AgentDecision{Kind: AgentAct, Action: &action, Code: code, Params: params}
}

func euros(minor int64) string { return fmt.Sprintf("%.2f", float64(minor)/100) }

// Decision parameters are machine values; message text is formatted separately.
func amountParams(params map[string]string, minor int64) map[string]string {
	result := make(map[string]string, len(params)+1)
	for key, value := range params {
		result[key] = value
	}
	result["amount"] = euros(minor)
	return result
}

// PublishedTotal is the professional's published price for this job, before any negotiation.
func PublishedTotal(job Job) int64 {
	if len(job.ScopeHistory) > 0 && job.ScopeHistory[0].TotalMinor > 0 {
		return job.ScopeHistory[0].TotalMinor
	}
	return job.Quote.TotalMinor
}

// InVisitWindow reports whether a visit starts inside the mandate's weekdays and hours.
func (m AgentMandate) InVisitWindow(a Appointment, now time.Time) bool {
	start, err := time.Parse(time.RFC3339, a.StartUTC)
	if err != nil {
		return false
	}
	end, err := time.Parse(time.RFC3339, a.EndUTC)
	if err != nil || !end.After(start) {
		return false
	}
	zone, err := time.LoadLocation(m.Timezone)
	if err != nil {
		return false
	}
	local, localEnd := start.In(zone), end.In(zone)
	weekday := int(local.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	if !slices.Contains(m.Weekdays, weekday) {
		return false
	}
	minutes := local.Hour()*60 + local.Minute()
	endMinutes := localEnd.Hour()*60 + localEnd.Minute()
	if localEnd.Day() != local.Day() {
		endMinutes = 24 * 60
	}
	if minutes < m.StartHour*60 || endMinutes > m.EndHour*60 {
		return false
	}
	if m.MaxDaysAhead > 0 && start.After(now.Add(time.Duration(m.MaxDaysAhead)*24*time.Hour)) {
		return false
	}
	return start.After(now)
}

// DecideAgent returns the next step for one agent on one request.
func DecideAgent(c AgentContext) AgentDecision {
	if !c.Mandate.Enabled {
		return wait()
	}
	if c.State.Cancelled && c.Role == "customer" && c.State.CancelledBy == "customer" {
		return c.cancelNotice()
	}
	if c.State.Cancelled && !(c.Role == "customer" && c.State.CancelledBy == "professional") {
		return wait()
	}
	if c.Role == "customer" {
		return decideCustomer(c)
	}
	return decideProfessional(c)
}

func decideCustomer(c AgentContext) AgentDecision {
	if c.State.Intake != nil && c.State.Intake.Step == "professional_review" {
		return escalate("review_professional", nil)
	}
	state, m := c.State, c.Mandate
	job := state.Job
	code := requestStatusCode(state, c.Role)

	// The professional could not take the job: find another within the same limits.
	if state.Cancelled && state.CancelledBy == "professional" {
		return act("find-another", Action{Action: "return-to-matches"}, map[string]string{"profileId": job.ProfessionalProfileID, "name": job.Professional})
	}
	if job.ID == "" {
		if state.Intake == nil || state.Intake.Step != "matches" || len(state.Intake.Matches) == 0 {
			return wait()
		}
		return recommendMatch(c)
	}
	limit := agentLimit(c)
	published := PublishedTotal(job)
	dialogMode := !m.BookWithoutAsking && state.Intake != nil
	if note, ok := c.milestone(); ok {
		return note
	}
	// After booking the assistant also reports what happened to a move, a cancellation and the payment,
	// and handles what the customer asks it to do with the visit.
	if note, ok := c.visitNotices(); ok {
		return note
	}
	if request, ok := c.visitRequests(); ok {
		return request
	}

	switch code {
	case "quote", "booking", "choose_visit":
		if job.Dispute != nil && job.Dispute.Status == "open" {
			return wait()
		}
		if job.NeedsClarification {
			return escalate("clarification_needed", nil)
		}
		if serviceAddressRequired(state) && job.ServiceAddress == "" {
			if m.ServiceAddress == "" {
				if dialogMode {
					return c.dialogAddress()
				}
				return escalate("address_needed", nil)
			}
			return act("set-address", Action{Action: "set-service-address", Value: m.ServiceAddress}, nil)
		}
		if limit == 0 {
			if !job.QuoteApproved {
				if dialogMode && state.AgentDialog.clone().Stage != "manual" {
					return c.dialogConfirm("approve-quote", "", "confirm_price", map[string]string{"pro": c.proName(), "amount": c.money(job.Quote.TotalMinor), "price": euros(job.Quote.TotalMinor)})
				}
				return escalate("approve_price", map[string]string{"price": euros(job.Quote.TotalMinor)})
			}
		} else if !job.QuoteApproved && job.Quote.TotalMinor > limit {
			// Above the limit: counter at the limit only when it is a plausible offer, otherwise ask the person.
			if c.Counters == 0 && limit*100 >= published*int64(m.CounterPercent) {
				return act("counter-price", Action{Action: "propose-price", PriceAmountMinor: limit, Value: "Within the budget set for this request."}, map[string]string{"amount": euros(limit), "published": euros(job.Quote.TotalMinor)})
			}
			return escalate("price_above_limit", map[string]string{"price": euros(job.Quote.TotalMinor), "limit": euros(limit)})
		}
		if job.PriceProposal != nil && job.PriceProposal.Status == "pending" || job.DateProposal != nil && job.DateProposal.Status == "pending" {
			return wait()
		}
		if BookingAutonomyChoiceNeeded(job) {
			if state.Intake != nil {
				return c.dialogBookingAutonomy()
			}
			return escalate("booking_autonomy_choice", nil)
		}
		// A professional who declined or let a reservation lapse is not offered the same time again.
		var released string
		if confirmation := job.AppointmentConfirmation; confirmation != nil && (confirmation.Status == "declined" || confirmation.Status == "expired") {
			released = confirmation.Appointment.StartUTC
		}
		if dialogMode {
			return c.dialogSchedule(released)
		}
		for _, slot := range state.AvailableSlots {
			if released != "" && slot.StartUTC == released {
				continue
			}
			if m.InVisitWindow(slot, c.Now) {
				action := "approve-and-book"
				if job.QuoteApproved {
					action = "book-appointment"
				}
				return act("book-visit", Action{Action: action, Value: slot.SlotID, PaymentAuthorizationTiming: chosenPaymentTiming(c.State.Job)}, map[string]string{"start": slot.StartUTC, "end": slot.EndUTC, "amount": euros(job.Quote.TotalMinor)})
			}
		}
		return proposeVisit(c, released)

	case "review_price":
		p := job.PriceProposal
		if limit == 0 {
			if dialogMode {
				return c.dialogConfirm("accept-price", p.ID, "confirm_counter", map[string]string{"pro": c.proName(), "amount": c.money(p.AmountMinor), "price": euros(p.AmountMinor)})
			}
			return escalate("approve_price", map[string]string{"price": euros(p.AmountMinor)})
		}
		if p.AmountMinor <= limit {
			return act("accept-price", Action{Action: "accept-price", ProposalID: p.ID}, map[string]string{"amount": euros(p.AmountMinor)})
		}
		if c.Counters == 0 && limit*100 >= published*int64(m.CounterPercent) {
			return act("counter-price", Action{Action: "propose-price", PriceAmountMinor: limit, Value: "This is the most I can agree to for this job."}, map[string]string{"amount": euros(limit), "offered": euros(p.AmountMinor)})
		}
		return escalate("price_above_limit", map[string]string{"price": euros(p.AmountMinor), "limit": euros(limit)})

	case "review_date":
		if job.Appointment == nil && BookingAutonomyChoiceNeeded(job) {
			if state.Intake != nil {
				return c.dialogBookingAutonomy()
			}
			return escalate("booking_autonomy_choice", nil)
		}
		p := job.DateProposal
		blocked, params := visitChangeBlocker(c, p.Appointment)
		if dialogMode && blocked != "late_change_needs_you" && blocked != "visit_change_involves_payment" {
			// The customer decides a time the professional suggests; the assistant only asks.
			return c.dialogConfirm("accept-date", p.ID, "confirm_visit", map[string]string{"pro": c.proName(), "when": c.visitWhen(p.Appointment), "start": p.Appointment.StartUTC, "end": p.Appointment.EndUTC})
		}
		if blocked != "" {
			return escalate(blocked, params)
		}
		return act("accept-visit", Action{Action: "accept-date", ProposalID: p.ID, PaymentAuthorizationTiming: chosenPaymentTiming(c.State.Job)}, map[string]string{"start": p.Appointment.StartUTC, "end": p.Appointment.EndUTC})

	case "review_change":
		ch := job.Change
		if m.AutoApproveExtraMinor > 0 && ch.AmountMinor <= m.AutoApproveExtraMinor && job.Quote.TotalMinor+ch.AmountMinor <= limit+m.AutoApproveExtraMinor {
			return act("approve-extra", Action{Action: "approve-change"}, map[string]string{"amount": euros(ch.AmountMinor)})
		}
		return escalate("extra_work_needs_approval", map[string]string{"amount": euros(ch.AmountMinor)})

	case "review":
		if BookingAutonomyCurrent(job) {
			if job.EvidenceReview == nil || job.EvidenceReview.EvidenceFingerprint != CompletionEvidenceFingerprint(job) {
				return c.autonomyAction("check-completion-photos", "review-completion-evidence")
			}
			if err := ValidateAutonomousCompletion(job); err != nil {
				return escalate("autonomy_review_needed", nil)
			}
			return c.autonomyAction("approve-work", "approve-completion")
		}
		// Judging finished work is the person's decision unless they handed it to their agent.
		if m.AutoApproveWork && job.BookingAutonomy == nil {
			return act("approve-work", Action{Action: "approve-completion"}, nil)
		}
		// The assistant asks in the chat and acts on the answer; the page's own buttons still work.
		if state.Intake != nil && workApprovable(job) {
			return c.dialogWork()
		}
		if state.Intake != nil {
			return c.workBlocked()
		}
		return escalate("check_the_work", nil)
	case "payment":
		if BookingAutonomyCurrent(job) {
			if err := ValidateAutonomousCompletion(job); err != nil {
				return escalate("autonomy_review_needed", nil)
			}
			if job.PaymentApprovedVersion != job.ScopeVersion {
				return c.autonomyAction("approve-delegated-payment", "approve-payment")
			}
			if job.PaymentApprovedByAgent {
				return c.autonomyAction("release-delegated-payment", "release-authorized-payment")
			}
		}
		return escalate("payment_needs_you", nil)
	case "dispute_review", "cancellation_review", "refund_pending":
		return wait()
	}
	return wait()
}

// Recommending a professional does not assign them. The customer must choose who comes,
// including when they have delegated visit-time booking within their preferences.
func recommendMatch(c AgentContext) AgentDecision {
	m := c.Mandate
	limit := agentLimit(c)
	for _, match := range c.State.Intake.Matches {
		if slices.Contains(c.Avoid, match.ID) {
			continue
		}
		if limit > 0 && match.TotalMinor > limit && !(c.Counters == 0 && limit*100 >= match.TotalMinor*int64(m.CounterPercent)) {
			continue
		}
		return escalate("choose_professional", map[string]string{"profileId": match.ID, "name": match.Name, "price": euros(match.TotalMinor)})
	}
	return escalate("no_match_within_limits", nil)
}

func decideProfessional(c AgentContext) AgentDecision {
	if c.State.Intake != nil && c.State.Intake.Step == "professional_review" {
		return wait()
	}
	state, m := c.State, c.Mandate
	job := state.Job
	if job.ID == "" {
		return wait()
	}
	code := requestStatusCode(state, c.Role)
	published := PublishedTotal(job)
	floor := published * int64(m.FloorPercent) / 100
	dialogMode := state.Intake != nil
	if note, ok := c.proMilestone(); ok {
		return note
	}
	if note, ok := c.proPaymentMilestones(); ok {
		return note
	}
	customer := c.customerName()

	switch code {
	case "confirm_visit":
		appt := job.Appointment
		if appt == nil {
			return wait()
		}
		outside := !c.professionalVisitAllowed(*appt)
		if m.AutoConfirmVisits && !outside {
			return act("confirm-visit", Action{Action: "confirm-appointment"}, map[string]string{"start": appt.StartUTC, "end": appt.EndUTC})
		}
		if dialogMode && appt != nil {
			key := "pro_confirm_visit"
			if outside {
				key += "_outside"
			}
			return c.proConfirm("confirm-appointment", "", key, map[string]string{"customer": customer, "when": c.visitWhen(*appt), "start": appt.StartUTC, "end": appt.EndUTC})
		}
		if !m.AutoConfirmVisits {
			return escalate("confirm_needed", nil)
		}
		return escalate("visit_outside_window", map[string]string{"start": appt.StartUTC})

	case "review_price":
		p := job.PriceProposal
		if p.AmountMinor >= floor {
			return act("accept-price", Action{Action: "accept-price", ProposalID: p.ID}, map[string]string{"amount": euros(p.AmountMinor)})
		}
		if c.Counters == 0 {
			return act("counter-price", Action{Action: "propose-price", PriceAmountMinor: floor, Value: "This is the lowest price I can do for this job."}, map[string]string{"amount": euros(floor), "offered": euros(p.AmountMinor)})
		}
		if dialogMode {
			return c.proConfirm("accept-price", p.ID, "pro_confirm_price", map[string]string{"customer": customer, "amount": c.money(p.AmountMinor), "floor": c.money(floor)})
		}
		return escalate("price_below_floor", map[string]string{"offered": euros(p.AmountMinor), "floor": euros(floor)})

	case "review_date":
		p := job.DateProposal
		blocked, params := visitChangeBlocker(c, p.Appointment)
		if blocked == "late_change_needs_you" || blocked == "visit_change_involves_payment" {
			return escalate(blocked, params)
		}
		if m.AutoAcceptDates {
			if blocked == "" {
				return act("accept-visit", Action{Action: "accept-date", ProposalID: p.ID}, map[string]string{"start": p.Appointment.StartUTC, "end": p.Appointment.EndUTC})
			}
			// A first visit outside the hours gets one counter-proposal at the nearest time inside them.
			if blocked == "visit_outside_window" && job.Appointment == nil && c.DateProposals == 0 {
				if counter, ok := nearestVisit(c, p.Appointment); ok {
					return act("propose-visit", proposeDate(counter, "This is the closest time I can offer."), map[string]string{"start": counter.StartUTC, "end": counter.EndUTC, "asked": p.Appointment.StartUTC})
				}
			}
		}
		if dialogMode {
			key := "pro_confirm_date"
			if blocked != "" {
				key += "_outside"
			}
			return c.proConfirm("accept-date", p.ID, key, map[string]string{"customer": customer, "when": c.visitWhen(p.Appointment), "start": p.Appointment.StartUTC, "end": p.Appointment.EndUTC})
		}
		if !m.AutoAcceptDates {
			return escalate("visit_proposal_needs_you", map[string]string{"start": p.Appointment.StartUTC})
		}
		return escalate(blocked, params)
	}
	return wait()
}

func proposeDate(a Appointment, note string) Action {
	return Action{Action: "propose-date", Value: note, ProposedAppointment: &Appointment{StartUTC: a.StartUTC, EndUTC: a.EndUTC, Timezone: a.Timezone}}
}

// visitLength is how long a visit lasts; the quote's duration, never under the 15-minute minimum.
func visitLength(job Job) time.Duration {
	return time.Duration(max(job.Quote.DurationMinutes, 15)) * time.Minute
}

// proposeVisit is the customer agent's way to a visit when none of the professional's published free
// times fits the mandate: approve the plan, then propose the preferred hour on the first allowed day.
func proposeVisit(c AgentContext, released string) AgentDecision {
	job, m := c.State.Job, c.Mandate
	if c.DateProposals > 0 {
		return escalate("no_time_in_window", nil)
	}
	zone, err := time.LoadLocation(m.Timezone)
	if err != nil {
		return escalate("no_time_in_window", nil)
	}
	length := visitLength(job)
	for day := 1; day <= max(m.MaxDaysAhead, 1)+1; day++ {
		d := c.Now.In(zone).AddDate(0, 0, day)
		weekday := int(d.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := time.Date(d.Year(), d.Month(), d.Day(), m.PreferredHour, 0, 0, 0, zone)
		end := start.Add(length)
		if !slices.Contains(m.Weekdays, weekday) || start.Before(c.Now.Add(24*time.Hour)) {
			continue
		}
		candidate := Appointment{StartUTC: start.UTC().Format(time.RFC3339), EndUTC: end.UTC().Format(time.RFC3339), Timezone: m.Timezone}
		if candidate.StartUTC == released || !m.InVisitWindow(candidate, c.Now) {
			continue
		}
		if !job.QuoteApproved {
			return act("approve-plan", Action{Action: "approve-quote", PaymentAuthorizationTiming: chosenPaymentTiming(c.State.Job)}, map[string]string{"amount": euros(job.Quote.TotalMinor)})
		}
		return act("propose-visit", proposeDate(candidate, "None of your listed times fit my schedule; this one does."), map[string]string{"start": candidate.StartUTC, "end": candidate.EndUTC})
	}
	return escalate("no_time_in_window", nil)
}

// nearestVisit finds the professional's closest time inside their mandate to a proposed one: a free
// published slot first (known to be free), otherwise the proposed day clamped into the hours, then the
// next allowed days.
func nearestVisit(c AgentContext, asked Appointment) (Appointment, bool) {
	m := c.Mandate
	want, err := time.Parse(time.RFC3339, asked.StartUTC)
	if err != nil {
		return Appointment{}, false
	}
	var best *Appointment
	var gap time.Duration
	for _, slot := range c.State.AvailableSlots {
		start, err := time.Parse(time.RFC3339, slot.StartUTC)
		if err != nil || !c.professionalVisitAllowed(slot) {
			continue
		}
		if d := start.Sub(want).Abs(); best == nil || d < gap {
			copy := slot
			best, gap = &copy, d
		}
	}
	if best != nil {
		return *best, true
	}
	// A saved weekly schedule is the source of free times. Do not invent a
	// counter from the old assistant-hours fields when none are published.
	if c.ProfessionalSchedule != nil && c.ProfessionalSchedule.Schedule != nil {
		return Appointment{}, false
	}
	zone, err := time.LoadLocation(m.Timezone)
	if err != nil {
		return Appointment{}, false
	}
	length := visitLength(c.State.Job)
	local := want.In(zone)
	for offset := 0; offset <= 14; offset++ {
		d := local.AddDate(0, 0, offset)
		weekday := int(d.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if !slices.Contains(m.Weekdays, weekday) {
			continue
		}
		hour, minute := local.Hour(), local.Minute()
		if offset > 0 {
			hour, minute = m.StartHour, 0
		}
		start := time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, zone)
		if latest := time.Date(d.Year(), d.Month(), d.Day(), m.EndHour, 0, 0, 0, zone).Add(-length); start.After(latest) {
			start = latest
		}
		if earliest := time.Date(d.Year(), d.Month(), d.Day(), m.StartHour, 0, 0, 0, zone); start.Before(earliest) {
			start = earliest
		}
		candidate := Appointment{StartUTC: start.UTC().Format(time.RFC3339), EndUTC: start.Add(length).UTC().Format(time.RFC3339), Timezone: m.Timezone}
		if m.InVisitWindow(candidate, c.Now.Add(24*time.Hour)) {
			return candidate, true
		}
	}
	return Appointment{}, false
}

// visitChangeBlocker says why an agent may not accept a proposed visit time on its own, or "" when
// accepting is inside the mandate. It covers a first visit proposal and a reschedule of a booked
// visit. A reschedule inside the late-change window (under 24 hours before the booked visit) is
// decided by the operator team, and anything tied to payment timing stays with the person.
func visitChangeBlocker(c AgentContext, proposed Appointment) (string, map[string]string) {
	job := c.State.Job
	if job.PaymentAuthorizationTiming == PaymentAuthorizationAtBooking && job.Appointment != nil {
		return "visit_change_involves_payment", map[string]string{"start": proposed.StartUTC}
	}
	if job.Appointment != nil && (job.Appointment.StartUTC != proposed.StartUTC || job.Appointment.EndUTC != proposed.EndUTC) {
		if late, err := VisitChangeNeedsReview(job.Appointment, c.Now); err != nil || late {
			return "late_change_needs_you", map[string]string{"start": job.Appointment.StartUTC, "proposed": proposed.StartUTC}
		}
	}
	allowed := c.Mandate.InVisitWindow(proposed, c.Now)
	if c.Role == "professional" {
		allowed = c.professionalVisitAllowed(proposed)
	}
	if !allowed {
		return "visit_outside_window", map[string]string{"start": proposed.StartUTC}
	}
	return "", nil
}

// Outside-hours permission changes only the professional's calendar window.
// Workflow booking checks still enforce duration, conflicts and payment/review gates.
func (c AgentContext) professionalVisitAllowed(visit Appointment) bool {
	start, err := time.Parse(time.RFC3339, visit.StartUTC)
	end, endErr := time.Parse(time.RFC3339, visit.EndUTC)
	if err != nil || endErr != nil || !start.After(c.Now) || !end.After(start) {
		return false
	}
	view := c.ProfessionalSchedule
	if view == nil || view.Schedule == nil {
		return c.Mandate.AcceptOutsideHours || c.Mandate.InVisitWindow(visit, c.Now)
	}
	zone, err := time.LoadLocation(view.Schedule.Timezone)
	if err != nil {
		return false
	}
	local, localEnd := start.In(zone), end.In(zone)
	date, endDate := local.Format("2006-01-02"), localEnd.Add(-time.Nanosecond).Format("2006-01-02")
	for _, off := range view.DaysOff {
		if date <= off.LastDay && endDate >= off.FirstDay {
			return false
		}
	}
	if c.Mandate.AcceptOutsideHours {
		return true
	}
	weekday := int(local.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	for _, work := range view.Schedule.Week {
		if weekday != work.Weekday {
			continue
		}
		from, ok1 := scheduleMinute(work.Start)
		to, ok2 := scheduleMinute(work.End)
		first := time.Date(local.Year(), local.Month(), local.Day(), from/60, from%60, 0, 0, zone)
		last := time.Date(local.Year(), local.Month(), local.Day(), to/60, to%60, 0, 0, zone)
		if ok1 && ok2 && !start.Before(first) && !end.After(last) {
			return true
		}
	}
	return false
}

// requestStatusCode maps the shared request status to the client vocabulary the agent reasons in.
func requestStatusCode(state State, role string) string {
	status, _ := RequestStatus(state, role)
	switch status {
	case "waiting_visit_confirmation", "confirm_visit":
		if role == "professional" {
			return "confirm_visit"
		}
		return "waiting_visit_confirmation"
	}
	return status
}

// AgentActivity is one entry in a request's agent activity: what an agent did or why it stopped.
type AgentActivity struct {
	ID        int64             `json:"id"`
	Role      string            `json:"role"`
	Code      string            `json:"code"`
	Action    string            `json:"action,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
	Outcome   string            `json:"outcome"`
	Error     string            `json:"error,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

// AgentEscalation is an open question an agent put to its person because it may not decide alone.
type AgentEscalation struct {
	ID          int64             `json:"id"`
	WorkspaceID string            `json:"workspaceId"`
	Role        string            `json:"role"`
	Code        string            `json:"code"`
	Params      map[string]string `json:"params,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
}

// AgentParty is an account and the name its agent acts under.
type AgentParty struct {
	AccountID   string
	DisplayName string
	Locale      string
	Active      bool // the account's membership in this role is active; a disabled person's agent does nothing
}
