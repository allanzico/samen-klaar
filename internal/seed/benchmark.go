package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	core "github.com/allanzico/samen-klaar/internal/ai"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

type latency struct {
	Samples int     `json:"samples"`
	P50     float64 `json:"p50_ms"`
	P95     float64 `json:"p95_ms"`
	Max     float64 `json:"max_ms"`
}

func milliseconds(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
func summarize(samples []float64) latency {
	if len(samples) == 0 {
		return latency{}
	}
	values := append([]float64(nil), samples...)
	sort.Float64s(values)
	return latency{len(values), values[int(math.Ceil(float64(len(values))*.5))-1], values[int(math.Ceil(float64(len(values))*.95))-1], values[len(values)-1]}
}

type benchmarkCase struct {
	Prompt           string   `json:"prompt"`
	Expected         string   `json:"expected_category"`
	Actual           string   `json:"actual_category,omitempty"`
	Route            string   `json:"route,omitempty"`
	Question         string   `json:"question,omitempty"`
	Options          []string `json:"options,omitempty"`
	AI               float64  `json:"ai_ms"`
	Directory        float64  `json:"directory_ms"`
	Total            float64  `json:"pipeline_ms"`
	Correct          bool     `json:"category_correct"`
	MatchID          string   `json:"first_profile_id,omitempty"`
	Service          string   `json:"first_service,omitempty"`
	ExpectedServices []string `json:"expected_services,omitempty"`
	ServiceSuitable  *bool    `json:"service_suitable,omitempty"`
	Price            int64    `json:"first_price_minor,omitempty"`
	Callout          int64    `json:"first_callout_minor,omitempty"`
	Earliest         string   `json:"first_availability_utc,omitempty"`
	Error            string   `json:"error,omitempty"`
}

var benchmarkCases = []benchmarkCase{
	{Prompt: "My kitchen sink is leaking underneath, but water still runs from the tap.", Expected: "Plumbing", ExpectedServices: []string{"Sink leak repair"}},
	{Prompt: "My kitchen sink is blocked and water backs up when I turn on the tap.", Expected: "Plumbing", ExpectedServices: []string{"Drain clearing"}},
	{Prompt: "The whole building's drains are backing up. I need the earliest available person with suitable drain equipment.", Expected: "Plumbing", ExpectedServices: []string{"Industrial drain assessment"}},
	{Prompt: "My toilet keeps running after flushing.", Expected: "Plumbing", ExpectedServices: []string{"Toilet repair"}},
	{Prompt: "The wooden bedroom door sticks and will not close.", Expected: "Carpentry", ExpectedServices: []string{"Door adjustment"}},
	{Prompt: "A kitchen cabinet hinge has come loose and the door is hanging crooked.", Expected: "Carpentry", ExpectedServices: []string{"Cabinet repair"}},
	{Prompt: "I need two wooden shelves installed in my living room.", Expected: "Carpentry", ExpectedServices: []string{"Shelf installation"}},
	{Prompt: "My wooden table legs wobble and the joints are loose.", Expected: "Carpentry", ExpectedServices: []string{"Furniture repair"}},
	{Prompt: "My circuit breaker keeps tripping when I turn on the oven.", Expected: "Electrical assessment", ExpectedServices: []string{"Circuit breaker assessment", "Appliance circuit assessment"}},
	{Prompt: "A wall socket sparks when I plug something in. I need the earliest available qualified professional.", Expected: "Electrical assessment", ExpectedServices: []string{"Socket fault assessment", "Wiring safety assessment"}},
	{Prompt: "The ceiling light flickers even with a new bulb.", Expected: "Electrical assessment", ExpectedServices: []string{"Lighting fault assessment"}},
	{Prompt: "The outdoor power outlet no longer works.", Expected: "Electrical assessment", ExpectedServices: []string{"Outdoor power assessment"}},
	{Prompt: "Something in my house is broken and I need help.", Expected: "Unclear"},
	{Prompt: "I need someone to fix a problem tomorrow, but I am not sure how to describe it.", Expected: "Unclear"},
}

func Benchmark(ctx context.Context, store *postgres.Store, ai *core.Coordinator, databaseURL, output string) error {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	defer pool.Close()
	var seeded, slots int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM professional_profiles p JOIN accounts a ON a.id=p.account_id WHERE a.subject LIKE 'auth0|relay-seed-professional%'`).Scan(&seeded); err != nil {
		return err
	}
	if seeded != Professionals {
		return fmt.Errorf("expected %d seeded professionals; found %d (run make seed-local)", Professionals, seeded)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM professional_availability_slots s JOIN accounts a ON a.id=s.professional_account_id WHERE a.subject LIKE 'auth0|relay-seed-professional%' AND s.status='available' AND s.starts_at>now()`).Scan(&slots); err != nil {
		return err
	}
	if slots < Professionals {
		return errors.New("seed availability is insufficient or has expired; inspect the seed report")
	}
	var customer string
	if err = pool.QueryRow(ctx, `SELECT id FROM accounts WHERE subject='auth0|relay-seed-customer1'`).Scan(&customer); err != nil {
		return err
	}
	categories := []string{"Plumbing", "Carpentry", "Electrical assessment"}
	distribution := map[string]int{}
	rows, err := pool.Query(ctx, `SELECT category,count(*) FROM professional_profiles p JOIN accounts a ON a.id=p.account_id WHERE a.subject LIKE 'auth0|relay-seed-professional%' GROUP BY category`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var category string
		var count int
		if err = rows.Scan(&category, &count); err != nil {
			rows.Close()
			return err
		}
		distribution[category] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Independently order all eligible slots, rather than repeating the
	// repository's per-professional LATERAL subquery.
	groundTruth := map[string]string{}
	for _, category := range categories {
		var profile string
		if err = pool.QueryRow(ctx, `SELECT p.id FROM professional_profiles p
JOIN account_memberships m ON m.account_id=p.account_id AND m.role='professional' AND m.status='active'
JOIN professional_availability_slots a ON a.professional_account_id=p.account_id AND a.status='available' AND a.starts_at>now()
WHERE p.status='active' AND p.category=$1 AND p.account_id<>$2
AND a.ends_at-a.starts_at>=make_interval(mins=>p.duration_minutes)
AND NOT EXISTS (SELECT 1 FROM booking_reservations b WHERE b.professional_id=p.account_id AND b.status='booked' AND b.starts_at<a.ends_at AND b.ends_at>a.starts_at)
ORDER BY a.starts_at,p.display_name,p.id,a.id LIMIT 1`, category, customer).Scan(&profile); err != nil {
			return err
		}
		groundTruth[category] = profile
	}
	valid := func(matches []workflow.Match, category, exclude string) bool {
		if len(matches) == 0 || matches[0].ID != groundTruth[category] {
			return false
		}
		for i, m := range matches {
			if m.Category != category || m.AccountID == exclude {
				return false
			}
			if i > 0 && m.EarliestAppointment != nil && matches[i-1].EarliestAppointment != nil && m.EarliestAppointment.StartUTC < matches[i-1].EarliestAppointment.StartUTC {
				return false
			}
		}
		return true
	}
	start := time.Now()
	first, err := store.ListProfessionalMatches(ctx, categories[0], workflow.Location{}, customer)
	cold := milliseconds(time.Since(start))
	if err != nil {
		return err
	}
	quality := valid(first, categories[0], customer)
	var serial []float64
	for i := 0; i < 300; i++ {
		category := categories[i%len(categories)]
		start := time.Now()
		matches, err := store.ListProfessionalMatches(ctx, category, workflow.Location{}, customer)
		serial = append(serial, milliseconds(time.Since(start)))
		if err != nil {
			return err
		}
		quality = quality && valid(matches, category, customer)
	}
	var mu sync.Mutex
	var concurrent []float64
	var concurrentErr error
	var wg sync.WaitGroup
	for worker := 0; worker < 10; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				category := categories[(worker*10+j)%len(categories)]
				start := time.Now()
				matches, err := store.ListProfessionalMatches(ctx, category, workflow.Location{}, customer)
				elapsed := milliseconds(time.Since(start))
				mu.Lock()
				concurrent = append(concurrent, elapsed)
				if err != nil {
					concurrentErr = err
				}
				quality = quality && err == nil && valid(matches, category, customer)
				mu.Unlock()
			}
		}(worker)
	}
	wg.Wait()
	if concurrentErr != nil {
		return concurrentErr
	}
	selfExcluded := true
	for _, category := range categories {
		matches, err := store.ListProfessionalMatches(ctx, category, workflow.Location{}, customer)
		if err != nil {
			return err
		}
		self := matches[0].AccountID
		matches, err = store.ListProfessionalMatches(ctx, category, workflow.Location{}, self)
		if err != nil {
			return err
		}
		for _, match := range matches {
			if match.AccountID == self {
				selfExcluded = false
			}
		}
		if len(matches) == 0 {
			selfExcluded = false
		}
	}
	cases := append([]benchmarkCase(nil), benchmarkCases...)
	var aiTimes, pipelineTimes []float64
	type timingScope struct {
		category string
		service  matching.Service
	}
	var serviceScopes []timingScope
	correct, failed := 0, 0
	for i := range cases {
		c := &cases[i]
		start := time.Now()
		requestCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
		suggestion, err := ai.Clarify(requestCtx, "Infer from description", c.Prompt)
		cancel()
		c.AI = milliseconds(time.Since(start))
		aiTimes = append(aiTimes, c.AI)
		if err != nil {
			c.Error = err.Error()
			failed++
			fmt.Printf("AI case %d/%d failed: %s\n", i+1, len(cases), c.Error)
			continue
		}
		c.Actual = suggestion.Category
		c.Route = suggestion.Route
		c.Question = suggestion.Question
		c.Options = suggestion.Options
		c.Correct = c.Actual == c.Expected
		if c.Correct {
			correct++
		}
		if c.Actual != "Unclear" {
			queryStart := time.Now()
			matches, _, err := matching.Find(ctx, store, ai, c.Actual, c.Prompt, customer)
			if err != nil {
				return err
			}
			c.Directory = milliseconds(time.Since(queryStart))
			for j, match := range matches {
				quality = quality && match.Category == c.Actual && match.AccountID != customer
				if j > 0 && match.EarliestAppointment != nil && matches[j-1].EarliestAppointment != nil {
					quality = quality && match.EarliestAppointment.StartUTC >= matches[j-1].EarliestAppointment.StartUTC
				}
			}
			if len(matches) == 0 {
				quality = false
			} else {
				first := matches[0]
				exclusions, marshalErr := json.Marshal(first.Exclusions)
				if marshalErr != nil {
					return marshalErr
				}
				var earliestID string
				err := pool.QueryRow(ctx, `SELECT p.id FROM professional_profiles p
 JOIN account_memberships m ON m.account_id=p.account_id AND m.role='professional' AND m.status='active'
 JOIN professional_availability_slots a ON a.professional_account_id=p.account_id AND a.status='available' AND a.starts_at>now()
 WHERE p.status='active' AND p.category=$1 AND p.account_id<>$2 AND p.service_name=$3 AND p.service_description=$4
 AND p.materials_policy=$5 AND p.exclusions_json=$6::jsonb
 AND a.ends_at-a.starts_at>=make_interval(mins=>p.duration_minutes)
 AND NOT EXISTS(SELECT 1 FROM booking_reservations b WHERE b.professional_id=p.account_id AND b.status='booked' AND b.starts_at<a.ends_at AND b.ends_at>a.starts_at)
 ORDER BY a.starts_at,p.id,a.id LIMIT 1`, c.Actual, customer, first.ServiceName, first.ServiceDescription, first.MaterialsPolicy, exclusions).Scan(&earliestID)
				if err != nil {
					return err
				}
				quality = quality && earliestID == first.ID
				service := matching.Service{Name: first.ServiceName, Description: first.ServiceDescription, Materials: first.MaterialsPolicy, Exclusions: first.Exclusions}
				serviceScopes = append(serviceScopes, timingScope{c.Actual, service})
				selfMatches, err := store.ServiceMatches(ctx, c.Actual, first.AccountID, service)
				if err != nil {
					return err
				}
				for _, m := range selfMatches {
					selfExcluded = selfExcluded && m.AccountID != first.AccountID
				}
			}
			if len(matches) > 0 {
				c.MatchID = matches[0].ID
				if matches[0].EarliestAppointment != nil {
					c.Earliest = matches[0].EarliestAppointment.StartUTC
				}
				offer, err := store.CatalogOffer(ctx, c.MatchID)
				if err != nil {
					return err
				}
				c.Service = offer.ServiceName
				c.Price = offer.PriceMinor
				c.Callout = offer.CalloutMinor
			}
		}
		c.Total = milliseconds(time.Since(start))
		pipelineTimes = append(pipelineTimes, c.Total)
		fmt.Printf("AI case %d/%d: %s, %.0f ms, category correct=%t\n", i+1, len(cases), c.Actual, c.Total, c.Correct)
	}
	var serviceSerial, serviceConcurrent []float64
	if len(serviceScopes) > 0 {
		for i := 0; i < 300; i++ {
			scope := serviceScopes[i%len(serviceScopes)]
			start := time.Now()
			_, err := store.ServiceMatches(ctx, scope.category, customer, scope.service)
			if err != nil {
				return err
			}
			serviceSerial = append(serviceSerial, milliseconds(time.Since(start)))
		}
		wg = sync.WaitGroup{}
		for worker := 0; worker < 10; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					scope := serviceScopes[(worker*10+j)%len(serviceScopes)]
					start := time.Now()
					_, err := store.ServiceMatches(ctx, scope.category, customer, scope.service)
					elapsed := milliseconds(time.Since(start))
					mu.Lock()
					serviceConcurrent = append(serviceConcurrent, elapsed)
					if err != nil {
						concurrentErr = err
					}
					mu.Unlock()
				}
			}(worker)
		}
		wg.Wait()
		if concurrentErr != nil {
			return concurrentErr
		}
	}
	serviceCases, serviceCorrect := assessServices(cases)
	report := map[string]any{
		"measured_at": time.Now().UTC(), "professionals": seeded, "future_available_slots": slots, "categories": distribution, "provider": ai.Provider(), "model": ai.Model(),
		"legacy_trade_directory_first_call_ms": cold, "legacy_trade_directory_serial": summarize(serial), "legacy_trade_directory_concurrent_10": summarize(concurrent), "service_directory_serial": summarize(serviceSerial), "service_directory_concurrent_10": summarize(serviceConcurrent), "ai_routing": summarize(aiTimes), "routing_and_matching": summarize(pipelineTimes),
		"service_relevance_cases": serviceCases, "service_relevance_correct": serviceCorrect, "service_relevance_passed": serviceCases == serviceCorrect, "category_correct": correct, "category_cases": len(cases), "ai_errors": failed, "earliest_and_category_checks_passed": quality, "self_booking_exclusion_passed": selfExcluded, "cases": cases,
		"limitations": []string{"Direct production coordinator and repository calls; excludes browser, HTTP, login, price negotiation and booking latency.", "Live AI service selection requests are sequential. Both legacy trade and service-specific directory reads measure 300 serial queries and 100 queries with 10 concurrent workers. Database timings exclude AI selection; routing_and_matching includes both live AI calls.", "Published service scope is selected by AI before earliest eligible availability ranking. Budget is a preference; published prices are never invented. Catalog input is bounded to 200 distinct scopes.", "AI categorization on 14 labeled prompts is a smoke evaluation, not proof of best-provider quality or model reliability."},
	}
	if err = writeJSON(filepath.Join(output, "matching-benchmark.json"), report); err != nil {
		return err
	}
	fmt.Printf("Directory serial p50/p95: %.2f/%.2f ms; concurrent 10 p50/p95: %.2f/%.2f ms\n", summarize(serial).P50, summarize(serial).P95, summarize(concurrent).P50, summarize(concurrent).P95)
	fmt.Printf("Live AI category accuracy: %d/%d; errors: %d; earliest/category checks: %t; self exclusion: %t\n", correct, len(cases), failed, quality, selfExcluded)
	fmt.Printf("Service directory p50/p95: %.2f/%.2f ms; concurrent 10 p50/p95: %.2f/%.2f ms\n", summarize(serviceSerial).P50, summarize(serviceSerial).P95, summarize(serviceConcurrent).P50, summarize(serviceConcurrent).P95)
	fmt.Printf("Selected service relevance: %d/%d\n", serviceCorrect, serviceCases)
	if !quality || !selfExcluded || failed > 0 || correct != len(cases) || serviceCorrect != serviceCases {
		return errors.New("benchmark found failures; inspect matching-benchmark.json")
	}
	return nil
}

// Judge the selected published offer against the labeled services in this seed
// dataset. A trade-correct but unrelated offer must fail the quality evaluation.
func assessServices(cases []benchmarkCase) (total, correct int) {
	for i := range cases {
		c := &cases[i]
		if len(c.ExpectedServices) == 0 {
			continue
		}
		total++
		suitable := false
		for _, expected := range c.ExpectedServices {
			if c.Correct && c.Service == expected {
				suitable = true
				break
			}
		}
		c.ServiceSuitable = &suitable
		if suitable {
			correct++
		}
	}
	return
}
