package jira

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseSince parses --since: YYYY-MM-DD or numeric days ago (e.g. 14 = now - 14 days).
func ParseSince(s string, now time.Time) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		t := now.AddDate(0, 0, -n)
		return &t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		t = t.UTC()
		return &t, nil
	}
	return nil, fmt.Errorf("invalid --since %q: use YYYY-MM-DD or number of days", s)
}

// ParseJiraDate parses a Jira date string.
func ParseJiraDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}

	formats := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	re := regexp.MustCompile(`(\d{2})(\d{2})$`)
	fixed := re.ReplaceAllString(dateStr, "$1:$2")
	if t, err := time.Parse(time.RFC3339, fixed); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("could not parse date: %s", dateStr)
}

// DaysFromNow returns the number of days from today for the given date string.
// Positive = future, negative = past. The second return is false if the date cannot be parsed.
func DaysFromNow(dateStr string) (int, bool) {
	if dateStr == "" || dateStr == "None" {
		return 0, false
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var target time.Time
	if !strings.Contains(dateStr, "T") {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return 0, false
		}
		target = t.UTC()
	} else {
		t, err := ParseJiraDate(dateStr)
		if err != nil {
			return 0, false
		}
		target = t.UTC()
	}
	targetDay := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	days := int(targetDay.Sub(today).Hours() / 24)
	return days, true
}
