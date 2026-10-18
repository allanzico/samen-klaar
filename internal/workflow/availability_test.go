package workflow

import (
	"testing"
	"time"
)

func TestParseAvailability(t *testing.T) {
	zone, _ := time.LoadLocation("Europe/Amsterdam")
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, zone) // Monday
	slot := func(day string, hour int) Appointment {
		start, _ := time.ParseInLocation("2006-01-02 15:04", day+" "+twoDigits(hour)+":00", zone)
		return Appointment{StartUTC: start.UTC().Format(time.RFC3339), EndUTC: start.Add(time.Hour).UTC().Format(time.RFC3339)}
	}
	cases := []struct {
		text         string
		fits, misses []Appointment
	}{
		{"Tuesday afternoon or Thursday after 5", []Appointment{slot("2026-10-20", 14), slot("2026-10-22", 18)}, []Appointment{slot("2026-10-20", 9), slot("2026-10-22", 14), slot("2026-10-21", 14)}},
		{"weekday evenings", []Appointment{slot("2026-10-21", 18)}, []Appointment{slot("2026-10-24", 18), slot("2026-10-21", 10)}},
		{"Tuesday or Thursday, afternoons", []Appointment{slot("2026-10-22", 13)}, []Appointment{slot("2026-10-22", 9)}},
		{"vrijdag ochtend", []Appointment{slot("2026-10-23", 9)}, []Appointment{slot("2026-10-23", 15)}},
		{"tomorrow between 9 and 11", []Appointment{slot("2026-10-20", 9), slot("2026-10-20", 10)}, []Appointment{slot("2026-10-20", 11), slot("2026-10-21", 9)}},
		{"21 oct 10-12", []Appointment{slot("2026-10-21", 10), slot("2026-10-21", 11)}, []Appointment{slot("2026-10-21", 12)}},
		{"next week", []Appointment{slot("2026-10-27", 10)}, []Appointment{slot("2026-10-22", 10)}},
		{"this week", []Appointment{slot("2026-10-22", 10)}, []Appointment{slot("2026-10-27", 10)}},
		{"any time", []Appointment{slot("2026-10-22", 3), slot("2026-10-25", 22)}, nil},
		{"I'm home after 6 on weekdays, but never on Wednesday", []Appointment{slot("2026-10-20", 18), slot("2026-10-22", 19)}, []Appointment{slot("2026-10-21", 19), slot("2026-10-22", 15)}},
		{"weekdays except Friday", []Appointment{slot("2026-10-20", 10)}, []Appointment{slot("2026-10-23", 10)}},
		{"Saturday morning", []Appointment{slot("2026-10-24", 9)}, []Appointment{slot("2026-10-24", 14)}},
	}
	for _, c := range cases {
		windows, ok := ParseAvailability(c.text, now, zone)
		if !ok {
			t.Errorf("%q: nothing parsed", c.text)
			continue
		}
		for _, v := range c.fits {
			if !WindowsAllow(windows, v, zone) {
				t.Errorf("%q should allow %s (%+v)", c.text, v.StartUTC, windows)
			}
		}
		for _, v := range c.misses {
			if WindowsAllow(windows, v, zone) {
				t.Errorf("%q should not allow %s (%+v)", c.text, v.StartUTC, windows)
			}
		}
	}
	for _, text := range []string{"", "hello there", "I don't know"} {
		if _, ok := ParseAvailability(text, now, zone); ok {
			t.Errorf("%q should not parse", text)
		}
	}
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
