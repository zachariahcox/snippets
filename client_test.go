package main

import (
	"testing"
	"time"
)

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
	data := extractIssueData(issue, "https://jira.example.com")
	computeTrending(data)
	if data.Key != "PROJ-1" {
		t.Errorf("Key = %q, want PROJ-1", data.Key)
	}
	if data.Summary != "Test issue" {
		t.Errorf("Summary = %q, want Test issue", data.Summary)
	}
	if data.Status != "in progress" {
		t.Errorf("Status = %q, want 'in progress'", data.Status)
	}
	if data.Trending != "on track" {
		t.Errorf("Trending = %q, want 'on track'", data.Trending)
	}
	if data.Assignee != "Alice" {
		t.Errorf("Assignee = %q, want Alice", data.Assignee)
	}
	if data.URL != "https://jira.example.com/browse/PROJ-1" {
		t.Errorf("URL = %q", data.URL)
	}
}

func TestExtractIssueData_missingFields(t *testing.T) {
	issue := map[string]any{
		"key": "PROJ-2",
		"fields": map[string]any{
			"summary": "Minimal",
		},
	}
	data := extractIssueData(issue, "https://jira.example.com")
	computeTrending(data)
	if data.Key != "PROJ-2" {
		t.Errorf("Key = %q, want PROJ-2", data.Key)
	}
	if data.Status != "unknown" {
		t.Errorf("Status = %q, want 'unknown'", data.Status)
	}
	if data.Trending != "unknown" {
		t.Errorf("Trending = %q, want 'unknown'", data.Trending)
	}
	if data.Assignee != "N/A" {
		t.Errorf("Assignee = %q, want N/A", data.Assignee)
	}
	if data.Priority != "None" {
		t.Errorf("Priority = %q, want None", data.Priority)
	}
}

func TestExtractIssueData_trendingAndEmoji(t *testing.T) {
	base := map[string]any{
		"key": "X-1",
		"fields": map[string]any{
			"summary": "Test",
			"created": "2025-01-01T00:00:00Z",
			"updated": "2025-01-02T00:00:00Z",
		},
	}
	tests := []struct {
		statusName        string
		wantTrending      string
		wantTrendingEmoji string
	}{
		{"Resolved", "done", "🟣"},
		{"Closed", "done", "🟣"},
		{"In Progress", "on track", "🟢"},
		{"Not Started", "not started", "⚪"},
		{"New", "not started", "⚪"},
		{"Blocked", "off track", "🔴"},
		// "At risk" is a trending outcome (e.g. not started + due soon), not a Jira status we branch on.
		{"At Risk", "INCONCLUSIVE", "❓"},
	}
	for _, tt := range tests {
		issue := copyMap(base)
		fields := issue["fields"].(map[string]any)
		fields["status"] = map[string]any{"name": tt.statusName}
		data := extractIssueData(issue, "https://jira.example.com")
		computeTrending(data)
		if data.Trending != tt.wantTrending {
			t.Errorf("status %q: Trending = %q, want %q", tt.statusName, data.Trending, tt.wantTrending)
		}
		if data.TrendingEmoji != tt.wantTrendingEmoji {
			t.Errorf("status %q: Emoji = %q, want %q", tt.statusName, data.TrendingEmoji, tt.wantTrendingEmoji)
		}
	}
}

func TestExtractIssueData_createdUpdated(t *testing.T) {
	issue := map[string]any{
		"key": "P-1",
		"fields": map[string]any{
			"summary": "Test",
			"status":  map[string]any{"name": "Resolved"},
			"created": "2024-06-15T09:00:00.000Z",
			"updated": "2024-07-20T14:30:00.000-0700",
		},
	}
	data := extractIssueData(issue, "https://jira.example.com")
	if data.Created != "2024-06-15T09:00:00.000Z" {
		t.Errorf("Created = %q", data.Created)
	}
	if data.Updated != "2024-07-20T14:30:00.000-0700" {
		t.Errorf("Updated = %q", data.Updated)
	}
}

func TestExtractIssueData_overdue(t *testing.T) {
	// In progress with past target end -> overdue (🔴, trending "overdue")
	issue := map[string]any{
		"key": "P-1",
		"fields": map[string]any{
			"summary": "Overdue task",
			"status":  map[string]any{"name": "in progress"},
			"created": "2025-01-01T00:00:00Z",
			"updated": "2025-01-02T00:00:00Z",
		},
	}
	// Target end comes from custom field; customFields["Target end"] may be unset.
	// If set in code, we'd need to inject. So test without target end first.
	data := extractIssueData(issue, "https://jira.example.com")
	computeTrending(data)
	if data.TrendingEmoji != "🟢" {
		t.Errorf("without target end: Emoji = %q, want 🟢", data.TrendingEmoji)
	}
	// Resolved/closed are terminal Jira statuses; "done" is not a status string we branch on.
	// Resolved + past target end must stay trending done (not flipped to off track by isStale).
	oldKey := customFields["Target end"]
	customFields["Target end"] = "targetEnd"
	defer func() { customFields["Target end"] = oldKey }()
	resolvedIssue := map[string]any{
		"key": "P-2",
		"fields": map[string]any{
			"summary":   "Shipped task",
			"status":    map[string]any{"name": "Resolved"},
			"created":   "2025-01-01T00:00:00Z",
			"updated":   "2025-01-02T00:00:00Z",
			"targetEnd": "2020-01-01",
		},
	}
	dataResolved := extractIssueData(resolvedIssue, "https://jira.example.com")
	computeTrending(dataResolved)
	if dataResolved.Trending != "done" || dataResolved.TrendingEmoji != "🟣" {
		t.Errorf("resolved issue (past target): Trending=%q Emoji=%q, want done/🟣", dataResolved.Trending, dataResolved.TrendingEmoji)
	}
}

func TestExtractIssueData_atRisk(t *testing.T) {
	// Not started + due within next month -> at risk (🟡, trending "at risk")
	// Use UTC to align with DaysFromNow / isDueWithinNextMonth (UTC calendar "today").
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	oldKey := customFields["Target end"]
	customFields["Target end"] = "targetEnd"
	defer func() { customFields["Target end"] = oldKey }()

	issue := map[string]any{
		"key": "P-1",
		"fields": map[string]any{
			"summary":   "Due soon task",
			"status":    map[string]any{"name": "Not Started"},
			"created":   "2025-01-01T00:00:00Z",
			"updated":   "2025-01-02T00:00:00Z",
			"targetEnd": tomorrow,
		},
	}
	data := extractIssueData(issue, "https://jira.example.com")
	computeTrending(data)
	if data.Trending != "at risk" {
		t.Errorf("not started + due tomorrow: Trending = %q, want at risk", data.Trending)
	}
	if data.TrendingEmoji != "🟡" {
		t.Errorf("not started + due tomorrow: Emoji = %q, want 🟡", data.TrendingEmoji)
	}
}

func TestExtractIssueData_statusNormalized(t *testing.T) {
	issue := map[string]any{
		"key": "P-1",
		"fields": map[string]any{
			"summary": "Test",
			"status":  map[string]any{"name": "  IN PROGRESS  "},
		},
	}
	data := extractIssueData(issue, "https://jira.example.com")
	computeTrending(data)
	if data.Status != "in progress" {
		t.Errorf("Status = %q, want 'in progress'", data.Status)
	}
	if data.Trending != "on track" {
		t.Errorf("Trending = %q, want 'on track'", data.Trending)
	}
}

// copyMap does a shallow copy of a map for building test variants.
func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if inner, ok := v.(map[string]any); ok {
			innerCopy := make(map[string]any, len(inner))
			for k2, v2 := range inner {
				innerCopy[k2] = v2
			}
			out[k] = innerCopy
		} else {
			out[k] = v
		}
	}
	return out
}
