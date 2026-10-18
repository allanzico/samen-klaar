package seed

import (
	"strings"
	"testing"
	"time"
)

func TestTestProfessionalsAreDistinctAndHaveFutureTimes(t *testing.T) {
	if len(testPros) != 10 {
		t.Fatalf("expected ten test professionals, got %d", len(testPros))
	}
	zone, _ := time.LoadLocation("Europe/Amsterdam")
	anchor := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	emails, trades, noTimes, paused := map[string]bool{}, map[string]bool{}, 0, 0
	for _, p := range testPros {
		email := p.slug + "@" + prosDomain
		if emails[email] || !strings.HasSuffix(email, ".test") {
			t.Fatalf("duplicate or non-test address %s", email)
		}
		emails[email] = true
		trades[p.category] = true
		if len(p.slots) == 0 {
			noTimes++
		}
		if !p.mandate.Enabled {
			paused++
		}
		for _, start := range p.slotTimes(anchor, zone) {
			if !start.After(anchor) {
				t.Errorf("%s has a free time in the past: %s", p.slug, start)
			}
		}
		if err := p.mandate.Normalize("professional"); err != nil {
			t.Errorf("%s: %v", p.slug, err)
		}
	}
	if len(trades) != 3 || noTimes != 1 || paused != 1 {
		t.Fatalf("the set should cover three trades, one professional with no times and one with a paused assistant: %v %d %d", trades, noTimes, paused)
	}
}
