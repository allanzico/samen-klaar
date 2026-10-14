package workflow

import (
	"crypto/rand"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

// Intake is the shared conversation contract. An AI provider suggests
// a trade and a question; workflow rules retain ownership of matching and booking.
type Message struct {
	SenderName      string `json:"senderName,omitempty"`
	ThreadProfileID string `json:"threadProfileId,omitempty"`
	Role            string `json:"role"`
	Text            string `json:"text"`
}
type Location struct {
	Label     string  `json:"label"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Source    string  `json:"source"`
}
type Match struct {
	ServiceName         string       `json:"serviceName"`
	ServiceDescription  string       `json:"serviceDescription"`
	FitReason           string       `json:"fitReason"`
	Currency            string       `json:"currency"`
	TotalMinor          int64        `json:"totalMinor"`
	DurationMinutes     int          `json:"durationMinutes"`
	MaterialsPolicy     string       `json:"materialsPolicy"`
	Exclusions          []string     `json:"exclusions"`
	AccountID           string       `json:"-"`
	EarliestAppointment *Appointment `json:"earliestAppointment,omitempty"`
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Category            string       `json:"category"`
	DistanceKM          float64      `json:"distanceKm"`
	Area                string       `json:"area"`
}
type Intake struct {
	Step        string    `json:"step"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Route       string    `json:"route"`
	Question    string    `json:"question"`
	Options     []string  `json:"options"`
	Messages    []Message `json:"messages"`
	Detail      string    `json:"detail"`
	Urgency     string    `json:"urgency"`
	Budget      string    `json:"budget"`
	Location    *Location `json:"location"`
	Matches     []Match   `json:"matches"`
	NeedsReview bool      `json:"needsReview"`
	Waitlisted  bool      `json:"waitlisted"`
}

func containsAny(s string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
}

// InferIntake is a conservative offline fallback, not a diagnosis.
func InferIntake(description string) IntakeSuggestion {
	s := strings.ToLower(description)
	result := IntakeSuggestion{Category: "Unclear", Route: "assessment", Question: "Where is the problem, and what happens when you notice it?", Options: []string{"I'll describe more", "I'm not sure"}}
	plumbing := containsAny(s, "sink", "tap", "pipe", "drain", "toilet", "water leak", "plumb")
	electrical := containsAny(s, "outlet", "socket", "electric", "wiring", "breaker", "sparks", "power cut")
	carpentry := containsAny(s, "cupboard", "cabinet", "hinge", "wooden door")
	count := 0
	for _, match := range []bool{plumbing, electrical, carpentry} {
		if match {
			count++
		}
	}
	if count != 1 {
		return result
	}
	if plumbing {
		result = IntakeSuggestion{Category: "Plumbing", Route: "assessment", Question: "Does water still come out of the tap, and where is it leaking?", Options: []string{"Water runs · pipe drips underneath", "No water comes out", "The drain is backing up", "I'm not sure"}}
		if containsAny(s, "sink", "tap", "pipe") && containsAny(s, "leak", "drip") {
			result.Route = "catalog"
		}
	}
	if electrical {
		result = IntakeSuggestion{Category: "Electrical assessment", Route: "assessment", Question: "Is it one outlet or several parts of the property? Describe only what you've already noticed.", Options: []string{"One outlet", "Several rooms", "I'm not sure"}}
	}
	if carpentry {
		result = IntakeSuggestion{Category: "Carpentry", Route: "assessment", Question: "Is the door catching, or is the wood or frame damaged?", Options: []string{"The door catches", "The wood or frame is damaged", "I'm not sure"}}
		if containsAny(s, "cupboard", "cabinet") && containsAny(s, "door", "hinge") {
			result.Route = "catalog"
		}
	}
	if containsAny(s, "industrial", "commercial", "factory", "flood", "sewage", "machine") {
		result.Route = "assessment"
	}
	return result
}

func ask(i *Intake, step, question string, options ...string) {
	i.Step, i.Question, i.Options = step, question, options
	i.Messages = append(i.Messages, Message{Role: "assistant", Text: question})
}

func applyIntake(state *State, input Action) error {
	if input.Actor != "customer" {
		return errors.New("only the customer can update this conversation")
	}
	value := strings.TrimSpace(input.Value)
	if input.Action == "start-intake" {
		if state.Intake != nil {
			return errors.New("continue the current intake or create a new request workspace")
		}
		if state.Job.ID != "" {
			return errors.New("this workspace already contains a request; create a new workspace to start another")
		}
		if len(value) < 5 || len(value) > 500 {
			return errors.New("describe the job in 5 to 500 characters")
		}
		suggestion := InferIntake(value)
		if input.Suggestion != nil {
			suggestion = *input.Suggestion
		}
		// Assessment changes the plan discussion, never access to matching.
		if !ValidSuggestion(suggestion) {
			return errors.New("the intake suggestion is not supported")
		}
		i := &Intake{Description: value, Category: suggestion.Category, Route: suggestion.Route, Matches: []Match{}, Messages: []Message{{Role: "customer", Text: value}}}
		ask(i, "detail", suggestion.Question, suggestion.Options...)
		state.Intake = i
		return nil
	}
	i := state.Intake
	if i == nil {
		return errors.New("describe the job first")
	}

	if input.Action == "return-to-matches" || input.Action == "revise-intake" {
		if input.Action == "return-to-matches" && state.Cancelled && state.CancelledBy == "professional" {
			state.Cancelled = false
			state.CancelledBy = ""
			state.Job = NewState().Job
			state.AvailableSlots = []Appointment{}
			finishIntake(i, input.Matches, input.SuggestedQuestion)
			return nil
		}
		if state.Job.QuoteApproved || state.Job.Appointment != nil || state.Job.WorkStarted || state.Job.Paid {
			return errors.New("an approved plan cannot be replaced; discuss changes with your professional")
		}
		if input.Action == "revise-intake" && (len(value) < 5 || len(value) > 500 || input.Suggestion == nil || !ValidSuggestion(*input.Suggestion)) {
			return errors.New("describe the corrected job in 5 to 500 characters")
		}
		state.Job = NewState().Job
		state.AvailableSlots = []Appointment{}
		if input.Action == "return-to-matches" {
			finishIntake(i, input.Matches, input.SuggestedQuestion)
			return nil
		}
		i.Description, i.Detail = value, ""
		i.Category, i.Route = input.Suggestion.Category, input.Suggestion.Route
		i.Matches = []Match{}
		i.Waitlisted = false
		i.Messages = append(i.Messages, Message{Role: "customer", Text: value})
		ask(i, "detail", input.Suggestion.Question, input.Suggestion.Options...)
		return nil
	}

	if input.Action == "refresh-matches" {
		if i.Step != "matches" && i.Step != "review" {
			return errors.New("finish describing the request before searching")
		}
		finishIntake(i, input.Matches, input.SuggestedQuestion)
		return nil
	}
	if input.Action == "set-location" {
		if i.Step != "location" && i.Step != "matches" && i.Step != "waitlist" {
			return errors.New("finish describing the request before choosing an area")
		}
		location, err := ResolveLocation(input.Location, value)
		if err != nil {
			return err
		}
		i.Location = &location
		i.Waitlisted = false
		i.Messages = append(i.Messages, Message{Role: "customer", Text: location.Label})
		finishIntake(i, input.Matches, input.SuggestedQuestion)
		return nil
	}
	if input.Action == "join-waitlist" {
		if i.Step != "waitlist" || i.Location == nil {
			return errors.New("there is no uncovered service area to add to the waitlist")
		}
		if i.Waitlisted {
			return errors.New("this request is already on the area waitlist")
		}
		i.Waitlisted = true
		i.Messages = append(i.Messages, Message{Role: "assistant", Text: "Your request is saved on the area waitlist for our operations team. No booking or provider match has been made yet."})
		return nil
	}
	if input.Action == "select-match" {
		if i.Step != "matches" || i.NeedsReview {
			return errors.New("finish the conversation before choosing a professional")
		}
		var match *Match
		for _, candidate := range i.Matches {
			if candidate.ID == value {
				copy := candidate
				match = &copy
				break
			}
		}
		if match == nil {
			return errors.New("choose a professional from these results")
		}
		offer := input.SelectedOffer
		if offer == nil || offer.ProfileID != match.ID || offer.Category != i.Category || offer.AccountID == "" || offer.PriceMinor < 0 || offer.CalloutMinor < 0 || len(offer.Currency) != 3 {
			return errors.New("the selected professional catalog entry is unavailable")
		}
		if input.CustomerAccountID != "" && offer.AccountID == input.CustomerAccountID {
			return errors.New("you cannot book your own services")
		}
		if offer.Currency != strings.ToUpper(offer.Currency) {
			return errors.New("the provider currency must be uppercase")
		}
		total := offer.PriceMinor + offer.CalloutMinor
		job := Job{
			ID: "job_" + rand.Text(), ProfessionalProfileID: offer.ProfileID, ProfessionalAccountID: offer.AccountID,
			Title: offer.ServiceName, ServiceDescription: offer.ServiceDescription, Customer: input.CustomerName, Professional: offer.DisplayName,
			Description: i.Description, Category: offer.Category, ScopeVersion: 1,
			Location: i.Location, IntakeRoute: i.Route, ClarificationAnswer: i.Detail,
			Quote: Quote{
				Currency: offer.Currency, Lines: []QuoteLine{
					{Label: offer.ServiceName, AmountMinor: offer.PriceMinor, Basis: "Published service price"},
					{Label: "Call-out", AmountMinor: offer.CalloutMinor, Basis: "Published provider call-out fee"},
				},
				TotalMinor: total, MaterialsPolicy: offer.MaterialsPolicy, Exclusions: append([]string{}, offer.Exclusions...),
				DurationMinutes: offer.DurationMinutes, Expires: time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339),
			},
			Evidence: []Evidence{}, Timeline: []Event{},
		}
		job.ScopeHistory = []ScopeSnapshot{scopeSnapshot(job)}
		if i.Route == "assessment" {
			job.Quote.Lines[0].Basis = "Published starting price; agree the scope and price together"
			job.ScopeHistory = []ScopeSnapshot{scopeSnapshot(job)}
		}
		next := State{Job: job, AvailableSlots: append([]Appointment{}, input.AvailableSlots...)}
		ask(i, "selected", match.Name+" is your selected professional. Review or negotiate the price below, then agree a time. Nothing is booked yet.")
		next.Intake = i
		*state = next
		return nil
	}
	if input.Action != "answer-intake" {
		return errors.New("unknown conversation action")
	}
	if len(value) < 2 || len(value) > 500 {
		return errors.New("answer in 2 to 500 characters")
	}
	if i.Step != "detail" && i.Step != "urgency" && i.Step != "budget" {
		return errors.New("use the options for the current step")
	}
	i.Messages = append(i.Messages, Message{Role: "customer", Text: value})
	switch i.Step {
	case "detail":
		i.Detail = strings.TrimSpace(i.Detail + "\n" + value)
		i.Category = ClarifiedCategory(*i, value, input.Suggestion)
		if input.Suggestion != nil && ValidSuggestion(*input.Suggestion) {
			i.Route = input.Suggestion.Route
		}
		if i.Urgency != "" && i.Budget != "" {
			finishIntake(i, input.Matches, input.SuggestedQuestion)
			return nil
		}
		ask(i, "urgency", "When would you like someone to take a look?", "As soon as possible", "In the next few days", "I'm flexible")
	case "urgency":
		i.Urgency = value
		// Timing is a preference. A visit still requires agreement on a date.
		if containsAny(strings.ToLower(value), "urgent", "today", "now", "asap", "immediate", "emergency") {
			i.Urgency = "As soon as possible"
		}
		ask(i, "budget", "Do you have a budget in mind? It's okay if you're not sure yet.", "I’ll describe my budget", "I’m flexible", "Help me understand the cost")
	case "budget":
		i.Budget = value
		finishIntake(i, input.Matches, input.SuggestedQuestion)
	}
	return nil
}

func ClarifiedCategory(i Intake, detail string, suggestion *IntakeSuggestion) string {
	if suggestion != nil && ValidSuggestion(*suggestion) {
		return suggestion.Category
	}
	if i.Category != "Unclear" {
		return i.Category
	}
	return InferIntake(i.Description + " " + detail).Category
}

func finishIntake(i *Intake, matches []Match, questions ...string) {
	i.Matches = append([]Match{}, matches...)
	i.NeedsReview = false
	if i.Category == "Unclear" {
		i.Matches = []Match{}
		ask(i, "detail", "Where is the problem, and what happens when you notice it? A little more detail will help me find the right professional.", "I'll describe more")
	} else if i.Location == nil {
		ask(i, "location", "Which neighbourhood or postcode area in Amsterdam should the professional cover? Share a neighbourhood or postcode only; we’ll ask for the full address after you choose a professional.")
	} else if len(i.Matches) == 0 {
		if len(questions) > 0 && questions[0] != "" && !strings.Contains(strings.ToLower(questions[0]), "area waitlist") {
			i.Step = "detail"
			i.Question = questions[0]
			i.Options = []string{"I’ll describe more"}
			i.Messages = append(i.Messages, Message{Role: "assistant", Text: questions[0]})
			return
		}
		i.Step = "waitlist"
		question := "There isn’t an approved professional with a published service covering this area yet. You can save this request to the area waitlist; no provider or booking will be invented."
		i.Question = question
		if len(i.Messages) == 0 || i.Messages[len(i.Messages)-1].Text != question {
			i.Messages = append(i.Messages, Message{Role: "assistant", Text: question})
		}
	} else {
		sort.SliceStable(i.Matches, func(a, b int) bool {
			x, y := i.Matches[a].EarliestAppointment, i.Matches[b].EarliestAppointment
			if x == nil {
				return false
			}
			if y == nil {
				return true
			}
			return x.StartUTC < y.StartUTC
		})
		ask(i, "matches", "Choose a professional to discuss your job and agree the plan, price and visit. Nothing is booked until you approve it.")
		if i.Urgency == "As soon as possible" {
			if first := i.Matches[0]; first.EarliestAppointment != nil {
				i.Messages = append(i.Messages, Message{Role: "assistant", Text: "The earliest available visit is " + first.EarliestAppointment.StartsAt + " (" + first.EarliestAppointment.Timezone + ") with " + first.Name + ". Review the plan and approve this time if it works for you."})
			} else {
				i.Messages = append(i.Messages, Message{Role: "assistant", Text: "These professionals have no published visit times yet. Choose one and propose a time; the visit is booked when you both agree."})
			}
		}
	}
}

func ValidSuggestion(s IntakeSuggestion) bool {
	validCategory := s.Category == "Plumbing" || s.Category == "Carpentry" || s.Category == "Electrical assessment" || s.Category == "Unclear"
	if !validCategory || s.Route != "catalog" && s.Route != "assessment" || len(s.Question) < 8 || len(s.Question) > 240 || len(s.Options) > 4 {
		return false
	}
	for _, option := range s.Options {
		if len(option) < 2 || len(option) > 80 {
			return false
		}
	}
	return true
}

func resolveLocation(location *Location, area string) (Location, error) {
	if location != nil {
		if location.Source != "area" || math.IsNaN(location.Latitude) || math.IsNaN(location.Longitude) || math.IsInf(location.Latitude, 0) || math.IsInf(location.Longitude, 0) || math.Abs(location.Latitude) > 90 || math.Abs(location.Longitude) > 180 {
			return Location{}, errors.New("invalid service area")
		}
		if location.Source == "area" {
			label, err := serviceAreaLabel(location.Label)
			if err != nil {
				return Location{}, err
			}
			return Location{Label: label, Source: "area"}, nil
		}
	}
	return ResolveLocation(nil, area)
}

func ResolveLocation(location *Location, area string) (Location, error) {
	if location != nil {
		return resolveLocation(location, area)
	}
	label, err := serviceAreaLabel(area)
	if err != nil {
		return Location{}, err
	}
	return Location{Label: label, Source: "area"}, nil
}

func serviceAreaLabel(value string) (string, error) {
	label := strings.TrimSpace(value)
	if len(label) < 2 || len(label) > 80 {
		return "", errors.New("enter an Amsterdam neighbourhood or postcode area")
	}
	// A postcode or its four-digit area prefix is useful for discovery. Reject
	// other digits here so customers do not send a street number before they
	// have selected a professional.
	digits := 0
	for _, r := range label {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	if digits > 0 {
		compact := strings.NewReplacer(" ", "", "-", "").Replace(label)
		valid := len(compact) == 4 || len(compact) == 6
		for index, r := range compact {
			if index < 4 && (r < '0' || r > '9') {
				valid = false
			}
			if index >= 4 && ((r < 'A' || r > 'Z') && (r < 'a' || r > 'z')) {
				valid = false
			}
		}
		if !valid {
			return "", errors.New("use a neighbourhood or postcode only; the full address is requested after you choose a professional")
		}
	}
	return label, nil
}

// IsAmsterdamServiceArea constrains launch coverage to the agreed Amsterdam
// market. Unknown neighbourhood names remain usable when the profile or request
// includes the city, while postcode entries are limited to Amsterdam's 10xx area.
func IsAmsterdamServiceArea(value string) bool {
	label := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	if label == "amsterdam" || strings.HasPrefix(label, "amsterdam ") || strings.HasPrefix(label, "amsterdam-") || strings.HasPrefix(label, "amsterdam/") {
		return true
	}
	compact := strings.NewReplacer(" ", "", "-", "").Replace(strings.ToUpper(label))
	if (len(compact) == 4 || len(compact) == 6) && len(compact) >= 4 && compact[:2] == "10" {
		valid := true
		for index, r := range compact {
			if index < 4 && (r < '0' || r > '9') || index >= 4 && (r < 'A' || r > 'Z') {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	area := strings.TrimPrefix(label, "amsterdam ")
	area = strings.TrimPrefix(area, "amsterdam-")
	area = strings.TrimPrefix(area, "amsterdam/")
	switch area {
	case "centrum", "noord", "zuid", "oost", "west", "nieuw-west", "zuidoost", "de pijp", "jordaan", "oud-west", "oud-zuid", "ijburg", "watergraafsmeer", "bos en lommer", "slotervaart", "zeeburg", "indische buurt", "osdorp":
		return true
	default:
		return false
	}
}

func DistanceKM(lat1, lon1, lat2, lon2 float64) float64 {
	const rad = math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*rad/2), 2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Pow(math.Sin((lon2-lon1)*rad/2), 2)
	return 6371 * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
