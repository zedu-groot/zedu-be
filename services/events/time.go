package events

import (
	"errors"
	"net/url"
	"strings"
	"time"
	_ "time/tzdata"
)

func ParseSchedule(start, end string) (time.Time, time.Time, error) {
	zone, _ := time.LoadLocation("Africa/Lagos")
	a, err := time.ParseInLocation("2006-01-02T15:04", start, zone)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid start date/time")
	}
	b, err := time.ParseInLocation("2006-01-02T15:04", end, zone)
	if err != nil || !b.After(a) {
		return time.Time{}, time.Time{}, errors.New("end must be after start")
	}
	return a.UTC(), b.UTC(), nil
}
func JoinAllowed(start, end, now time.Time) bool {
	return !now.Before(start.Add(-15*time.Minute)) && now.Before(end)
}
func ValidDestination(raw string) bool {
	if len(raw) > 2048 || strings.TrimSpace(raw) != raw {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
