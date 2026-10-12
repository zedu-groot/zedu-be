package test_events

import (
	"testing"
	"time"

	events "github.com/hngprojects/telex_be/services/events"
)

func TestScheduleConvertsWATAndRejectsInvalidRange(t *testing.T) {
	start, end, err := events.ParseSchedule("2026-10-16T16:00", "2026-10-16T17:00")
	if err != nil || start.Format(time.RFC3339) != "2026-10-16T15:00:00Z" || end.Format(time.RFC3339) != "2026-10-16T16:00:00Z" {
		t.Fatalf("incorrect UTC schedule: %v %v %v", start, end, err)
	}
	for _, input := range [][2]string{{"2026-10-16T17:00", "2026-10-16T16:00"}, {"2026-02-30T16:00", "2026-02-30T17:00"}} {
		if _, _, err := events.ParseSchedule(input[0], input[1]); err == nil {
			t.Fatalf("accepted invalid schedule %v", input)
		}
	}
}
func TestExactJoinWindow(t *testing.T) {
	start := time.Date(2026, 10, 16, 15, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	for _, tc := range []struct {
		at   time.Time
		want bool
	}{{start.Add(-15*time.Minute - time.Second), false}, {start.Add(-15 * time.Minute), true}, {end.Add(-time.Second), true}, {end, false}} {
		if got := events.JoinAllowed(start, end, tc.at); got != tc.want {
			t.Errorf("join at %v: got %v want %v", tc.at, got, tc.want)
		}
	}
}
func TestDestinationRejectsUnsafeProtocolsAndCredentials(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
	}{{"https://meet.example.test/session?code=123", true}, {"javascript:alert(1)", false}, {"http://meet.example.test/session", false}, {"https://user:password@meet.example.test/session", false}, {"https://", false}, {"", false}} {
		if got := events.ValidDestination(tc.url); got != tc.want {
			t.Errorf("destination %q: got %v want %v", tc.url, got, tc.want)
		}
	}
}
