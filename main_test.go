package main

import (
	"strings"
	"testing"
	"time"
)

func TestGetStatusEmoji(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"done", "🟣"},
		{"Done", "🟣"},
		{"  in progress  ", "🟢"},
		{"not started", "⚪"},
		{"at risk", "🟡"},
		{"blocked", "🔴"},
		{"unknown-status", "❓"},
		{"", "❓"},
	}
	for _, tt := range tests {
		got := GetStatusEmoji(tt.status)
		if got != tt.want {
			t.Errorf("GetStatusEmoji(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestGetStatusPriority(t *testing.T) {
	if got := GetStatusPriority("done"); got != 0 {
		t.Errorf("GetStatusPriority(done) = %d, want 0", got)
	}
	if got := GetStatusPriority("new"); got != 10 {
		t.Errorf("GetStatusPriority(new) = %d, want 10", got)
	}
	if got := GetStatusPriority("unknown"); got != 999 {
		t.Errorf("GetStatusPriority(unknown) = %d, want 999", got)
	}
}

func TestParseJiraDate(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"2025-01-15", true},
		{"2025-01-02T15:04:05.000Z", true},
		{"2025-01-02T15:04:05.000-0700", true},
		{"", false},
		{"not-a-date", false},
	}
	for _, tt := range tests {
		_, err := ParseJiraDate(tt.input)
		gotValid := err == nil
		if gotValid != tt.valid {
			t.Errorf("ParseJiraDate(%q) valid=%v, want %v (err=%v)", tt.input, gotValid, tt.valid, err)
		}
	}
	// Check a parsed value
	tm, err := ParseJiraDate("2025-06-10")
	if err != nil {
		t.Fatalf("ParseJiraDate(2025-06-10): %v", err)
	}
	if y, m, d := tm.Date(); y != 2025 || m != 6 || d != 10 {
		t.Errorf("ParseJiraDate(2025-06-10) = %v, want 2025-06-10", tm)
	}
}

func TestFormatDate(t *testing.T) {
	if got := FormatDate(""); got != "N/A" {
		t.Errorf("FormatDate(\"\") = %q, want N/A", got)
	}
	if got := FormatDate("2025-03-20"); got != "2025-03-20" {
		t.Errorf("FormatDate(2025-03-20) = %q, want 2025-03-20", got)
	}
}

func TestIsOverdue(t *testing.T) {
	// Done/resolved never overdue
	if IsOverdue("done", "2000-01-01") {
		t.Error("IsOverdue(done, past date) want false")
	}
	if IsOverdue("resolved", "2000-01-01") {
		t.Error("IsOverdue(resolved, past date) want false")
	}
	// No target end -> not overdue
	if IsOverdue("in progress", "") {
		t.Error("IsOverdue(in progress, \"\") want false")
	}
	if IsOverdue("in progress", "None") {
		t.Error("IsOverdue(in progress, None) want false")
	}
	// Past date and not done -> overdue (use fixed past date so test is stable)
	if !IsOverdue("in progress", "2000-01-01") {
		t.Error("IsOverdue(in progress, 2000-01-01) want true")
	}
}

func TestExtractIssueData(t *testing.T) {
	issue := map[string]any{
		"key": "PROJ-1",
		"fields": map[string]any{
			"summary":  "Test issue",
			"status":   map[string]any{"name": "In Progress"},
			"assignee": map[string]any{"displayName": "Alice"},
			"priority": map[string]any{"name": "High"},
			"created":  "2025-01-01T10:00:00.000Z",
			"updated":  "2025-01-02T12:00:00.000Z",
		},
	}
	data := ExtractIssueData(issue, "https://jira.example.com", "", "")
	if data.Key != "PROJ-1" {
		t.Errorf("Key = %q, want PROJ-1", data.Key)
	}
	if data.Summary != "Test issue" {
		t.Errorf("Summary = %q, want Test issue", data.Summary)
	}
	if data.StatusName != "in progress" {
		t.Errorf("StatusName = %q, want 'in progress'", data.StatusName)
	}
	if data.Assignee != "Alice" {
		t.Errorf("Assignee = %q, want Alice", data.Assignee)
	}
	if data.URL != "https://jira.example.com/browse/PROJ-1" {
		t.Errorf("URL = %q", data.URL)
	}
	if data.ParentKey != "PROJ-1" {
		t.Errorf("ParentKey = %q, want PROJ-1 (defaults to issue key)", data.ParentKey)
	}
}

func TestExtractIssueData_missingFields(t *testing.T) {
	issue := map[string]any{
		"key": "PROJ-2",
		"fields": map[string]any{
			"summary": "Minimal",
			// no status, assignee, priority -> defaults
		},
	}
	data := ExtractIssueData(issue, "https://jira.example.com", "PARENT-1", "Parent summary")
	if data.Key != "PROJ-2" {
		t.Errorf("Key = %q, want PROJ-2", data.Key)
	}
	if data.StatusName != "unknown" {
		t.Errorf("StatusName = %q, want 'unknown'", data.StatusName)
	}
	if data.Assignee != "Unassigned" {
		t.Errorf("Assignee = %q, want Unassigned", data.Assignee)
	}
	if data.Priority != "None" {
		t.Errorf("Priority = %q, want None", data.Priority)
	}
	if data.ParentKey != "PARENT-1" || data.ParentSummary != "Parent summary" {
		t.Errorf("ParentKey=%q ParentSummary=%q", data.ParentKey, data.ParentSummary)
	}
}

func TestRenderMarkdownReport(t *testing.T) {
	issues := []IssueData{
		{
			Key:        "A-1",
			URL:        "https://jira/a",
			Summary:    "First",
			StatusName: "Resolved",
			Assignee:   "Alice",
			TargetEnd:  "2025-01-01",
			Updated:    "2025-01-02",
			Emoji:      "🟣",
			Trending:   "done",
		},
	}
	out := RenderMarkdownReport(issues, false, nil, "Test Report")
	if out == "" {
		t.Error("RenderMarkdownReport returned empty string")
	}
	if !strings.Contains(out, "Test Report") {
		t.Errorf("output missing title: %s", out)
	}
	// if !strings.Contains(out, "A-1") || !strings.Contains(out, "First") {
	// 	t.Errorf("output missing key: %s", out)
	// }
	// ensure "resolved" is mapped to done
	if !strings.Contains(out, "🟣 done") {
		t.Errorf("trending mapping failed: %s", out)
	}
}

func TestRenderMarkdownReport_filterSince(t *testing.T) {
	jan1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	issues := []IssueData{
		{Key: "X-1", Updated: "2024-12-01T00:00:00Z", Summary: "Old"},
		{Key: "X-2", Updated: "2025-02-01T00:00:00Z", Summary: "New"},
	}
	out := RenderMarkdownReport(issues, false, &jan1, "")
	if strings.Contains(out, "Old") {
		t.Error("expected issue updated before since to be filtered out")
	}
	if !strings.Contains(out, "New") {
		t.Error("expected issue updated after since to be included")
	}
}
