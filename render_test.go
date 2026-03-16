package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGetStatusPriority(t *testing.T) {
	if got := GetStatusPriority("closed"); got != 0 {
		t.Errorf("GetStatusPriority(done) = %d, want 0", got)
	}
	if got := GetStatusPriority("new"); got != 6 {
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

func TestIsStale(t *testing.T) {
	if !IsStale("2000-01-01") {
		t.Error("IsStale(2000-01-01) should be true")
	}
	if !IsStale("2000-01-01") {
		t.Error("IsStale(2000-01-01) should be true")
	}
	if IsStale("") {
		t.Error("IsStale(\"\") should be false")
	}
	if IsStale("None") {
		t.Error("IsStale(None) should be false")
	}
	if !IsStale("2000-01-01") {
		t.Error("IsStale(2000-01-01) should be true")
	}
}

func TestIsDueWithinNextMonth(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	twoMonths := time.Now().AddDate(0, 2, 0).Format("2006-01-02")

	if IsDueWithinNextMonth("") {
		t.Error("IsDueWithinNextMonth(\"\") want false")
	}
	if IsDueWithinNextMonth("None") {
		t.Error("IsDueWithinNextMonth(None) want false")
	}
	if !IsDueWithinNextMonth(tomorrow) {
		t.Error("IsDueWithinNextMonth(tomorrow) want true")
	}
	if IsDueWithinNextMonth(twoMonths) {
		t.Error("IsDueWithinNextMonth(2 months) want false")
	}
}

func TestDaysFromNow(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).UTC().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).UTC().Format("2006-01-02")
	nextWeek := time.Now().AddDate(0, 0, 7).UTC().Format("2006-01-02")
	lastWeek := time.Now().AddDate(0, 0, -7).UTC().Format("2006-01-02")

	// Invalid or empty -> ok=false
	for _, in := range []string{"", "None", "not-a-date", "2025-13-45"} {
		if _, ok := DaysFromNow(in); ok {
			t.Errorf("DaysFromNow(%q) want ok=false", in)
		}
	}

	// Date-only format: positive = future, negative = past
	if d, ok := DaysFromNow(today); !ok || d != 0 {
		t.Errorf("DaysFromNow(today) = %d, ok=%v, want 0, true", d, ok)
	}
	if d, ok := DaysFromNow(tomorrow); !ok || d != 1 {
		t.Errorf("DaysFromNow(tomorrow) = %d, ok=%v, want 1, true", d, ok)
	}
	if d, ok := DaysFromNow(yesterday); !ok || d != -1 {
		t.Errorf("DaysFromNow(yesterday) = %d, ok=%v, want -1, true", d, ok)
	}
	if d, ok := DaysFromNow(nextWeek); !ok || d != 7 {
		t.Errorf("DaysFromNow(nextWeek) = %d, ok=%v, want 7, true", d, ok)
	}
	if d, ok := DaysFromNow(lastWeek); !ok || d != -7 {
		t.Errorf("DaysFromNow(lastWeek) = %d, ok=%v, want -7, true", d, ok)
	}

	// Datetime format (Jira-style) uses same day
	tomorrowDT := time.Now().AddDate(0, 0, 1).UTC().Format(time.RFC3339)
	if d, ok := DaysFromNow(tomorrowDT); !ok || d != 1 {
		t.Errorf("DaysFromNow(datetime tomorrow) = %d, ok=%v, want 1, true", d, ok)
	}
}

func TestRenderMarkdownReport(t *testing.T) {
	issues := []*IssueData{
		{
			Key:           "A-1",
			URL:           "https://jira/a",
			Summary:       "First",
			Status:        "resolved",
			Type:          "story",
			Assignee:      "Alice",
			TargetEnd:     "2025-01-01",
			Updated:       "2025-01-02",
			TrendingEmoji: "🟣",
			Trending:      "done",
		},
	}
	cfg := &ReportConfig{Title: "abc"}
	out := RenderMarkdownReport(issues, cfg)
	if out == "" {
		t.Error("RenderMarkdownReport returned empty string")
	}
	if !strings.Contains(out, "abc") {
		t.Errorf("output missing title: %s", out)
	}
	if !strings.Contains(out, "🟣 done") {
		t.Errorf("trending mapping failed: %s", out)
	}
	if !strings.Contains(out, "| type |") {
		t.Error("expected type column in markdown header")
	}
	if !strings.Contains(out, "story") {
		t.Error("expected issue type (story) in markdown output")
	}
}

func TestRenderMarkdownReport_children(t *testing.T) {
	issues := []*IssueData{
		{
			Key:           "PROJ-123-1",
			URL:           "https://jira/browse/PROJ-123-1",
			Summary:       "Subtask one",
			Status:        "in progress",
			Type:          "subtask",
			Assignee:      "Bob",
			ParentKey:     "PROJ-123",
			ParentSummary: "Parent epic",
			ParentURL:     "https://jira/browse/PROJ-123",
			TargetEnd:     "2025-02-01",
			Updated:       "2025-01-15",
			TrendingEmoji: "🟢",
			Trending:      "in progress",
		},
	}
	cfg := &ReportConfig{ShowChildren: true, Title: "Children Report"}
	out := RenderMarkdownReport(issues, cfg)
	if out == "" {
		t.Error("RenderMarkdownReport returned empty string")
	}
	if !strings.Contains(out, "| type |") {
		t.Error("expected type column in markdown header")
	}
	if !strings.Contains(out, "| parent |") {
		t.Error("expected parent column header when ShowChildren=true")
	}
	if !strings.Contains(out, "Subtask one") {
		t.Error("output missing child summary")
	}
	if !strings.Contains(out, "PROJ-123") {
		t.Error("output missing parent key")
	}
	if !strings.Contains(out, "subtask") {
		t.Error("expected issue type (subtask) in markdown output")
	}
}

func TestRenderMarkdownReport_filterSince(t *testing.T) {
	jan1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	issues := []*IssueData{
		{Key: "X-1", Updated: "2024-12-01T00:00:00Z", Summary: "Old"},
		{Key: "X-2", Updated: "2025-02-01T00:00:00Z", Summary: "New"},
	}
	cfg := &ReportConfig{UpdatedAfter: &jan1}
	out := RenderMarkdownReport(issues, cfg)
	if strings.Contains(out, "Old") {
		t.Error("expected issue updated before since to be filtered out")
	}
	if !strings.Contains(out, "New") {
		t.Error("expected issue updated after since to be included")
	}
}

func TestRenderMarkdownReport_filterNoCommentAfter(t *testing.T) {
	now := time.Now().UTC()
	since := now.AddDate(0, 0, -10)
	recentComment := now.AddDate(0, 0, -1).Format(time.RFC3339)
	oldComment := now.AddDate(0, 0, -60).Format(time.RFC3339)
	issues := []*IssueData{
		{Key: "X-1", Summary: "Recently commented", Comment: IssueComment{Created: recentComment}},
		{Key: "X-2", Summary: "Stale, needs update", Comment: IssueComment{Created: oldComment}},
		{Key: "X-3", Summary: "No comment", Comment: IssueComment{}},
	}
	cfg := &ReportConfig{NoCommentAfter: &since}
	out := RenderMarkdownReport(issues, cfg)
	if out == "" {
		t.Error("RenderMarkdownReport returned empty string")
	}
	if strings.Contains(out, "Recently commented") {
		t.Error("issues with recent comments should be excluded")
	}
	if !strings.Contains(out, "Stale, needs update") {
		t.Error("issues with old comments should be included")
	}
	if !strings.Contains(out, "No comment") {
		t.Error("issues with no comments should be included")
	}
}

func TestRenderJSONReport(t *testing.T) {
	issues := []*IssueData{
		{
			Key:      "A-1",
			URL:      "https://jira/a",
			Summary:  "First",
			Status:   "in progress",
			Assignee: "Alice",
			Updated:  "2025-01-02",
		},
	}
	cfg := &ReportConfig{}
	out := RenderJSONReport(issues, cfg)
	if out == "" {
		t.Fatal("RenderJSONReport returned empty string")
	}
	if !json.Valid([]byte(out)) {
		t.Errorf("output is not valid JSON: %s", out)
	}
	var decoded []*IssueData
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if len(decoded) != 1 {
		t.Errorf("decoded %d issues, want 1", len(decoded))
	}
	if decoded[0].Key != "A-1" || decoded[0].Summary != "First" {
		t.Errorf("decoded issue = %+v, want Key=A-1 Summary=First", decoded[0])
	}
}

func TestRenderJSONReport_filterSince(t *testing.T) {
	jan1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	issues := []*IssueData{
		{Key: "X-1", Updated: "2024-12-01T00:00:00Z", Summary: "Old"},
		{Key: "X-2", Updated: "2025-02-01T00:00:00Z", Summary: "New"},
	}
	cfg := &ReportConfig{UpdatedAfter: &jan1}
	out := RenderJSONReport(issues, cfg)
	var decoded []*IssueData
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(decoded) != 1 {
		t.Errorf("after filter: %d issues, want 1", len(decoded))
	}
	if decoded[0].Summary != "New" {
		t.Errorf("expected filtered issue Summary=New, got %q", decoded[0].Summary)
	}
}

func TestRenderJSONReport_empty(t *testing.T) {
	cfg := &ReportConfig{}
	out := RenderJSONReport(nil, cfg)
	if out == "" {
		t.Fatal("RenderJSONReport returned empty string for nil input")
	}
	var decoded []*IssueData
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("failed to unmarshal empty output: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("decoded %d issues, want 0", len(decoded))
	}
}

func TestRenderCSVReport(t *testing.T) {
	issues := []*IssueData{
		{
			Key:           "A-1",
			URL:           "https://jira/a",
			Summary:       "First issue",
			Status:        "in progress",
			Assignee:      "Alice",
			TargetEnd:     "2025-02-01",
			Updated:       "2025-01-15",
			TrendingEmoji: "🟢",
			Trending:      "in progress",
		},
	}
	cfg := &ReportConfig{}
	out := RenderCSVReport(issues, cfg)
	if out == "" {
		t.Fatal("RenderCSVReport returned empty string")
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Errorf("expected header + at least 1 row, got %d lines", len(lines))
	}
	if !strings.Contains(out, "🐱") {
		t.Error("expected CSV output to contain separator 🐱")
	}
	if !strings.Contains(out, "First issue") {
		t.Error("expected output to contain issue summary")
	}
	if !strings.Contains(out, "status") {
		t.Error("expected header row with status column")
	}
}

func TestRenderCSVReport_children(t *testing.T) {
	issues := []*IssueData{
		{
			Key:           "PROJ-123-1",
			Summary:       "Subtask",
			ParentKey:     "PROJ-123",
			Assignee:      "Bob",
			TrendingEmoji: "🟢",
			Trending:      "in progress",
		},
	}
	cfg := &ReportConfig{ShowChildren: true}
	out := RenderCSVReport(issues, cfg)
	if !strings.Contains(out, "parent") {
		t.Error("expected parent column when ShowChildren=true")
	}
	if !strings.Contains(out, "PROJ-123") {
		t.Error("expected parent key in output")
	}
}

func TestRenderCSVReport_escapeSeparator(t *testing.T) {
	issues := []*IssueData{
		{
			Summary:       "Contains🐱emoji",
			TrendingEmoji: "🟢",
			Trending:      "in progress",
		},
	}
	cfg := &ReportConfig{}
	out := RenderCSVReport(issues, cfg)
	if !strings.Contains(out, `"Contains🐱emoji"`) {
		t.Errorf("expected quoted field for value containing separator, got: %s", out)
	}
}

func TestRenderSlackReport(t *testing.T) {
	issues := []*IssueData{
		{
			Key:           "A-1",
			URL:           "https://jira/browse/A-1",
			Summary:       "First issue",
			TrendingEmoji: "🟢",
			Trending:      "in progress",
			TargetEnd:     "2025-02-01",
			Comment:       IssueComment{Url: "https://jira/comment/1", Created: "2025-01-15"},
		},
		{
			Key:           "A-2",
			URL:           "https://jira/browse/A-2",
			Summary:       "Second issue",
			TargetEnd:     "2025-02-02",
			TrendingEmoji: "🟣",
			Trending:      "done",
		},
	}
	cfg := &ReportConfig{}
	out := RenderSlackReport(issues, cfg)
	if !strings.HasPrefix(out, "1. ") {
		t.Errorf("expected numbered list, got: %s", out)
	}
	if !strings.Contains(out, "[First issue](https://jira/browse/A-1)") {
		t.Error("expected summary link in output")
	}
	if !strings.Contains(out, "([last update](https://jira/comment/1))") {
		t.Error("expected update link for first issue")
	}
}

func TestRenderURLReport(t *testing.T) {
	issues := []*IssueData{
		{Key: "SCM-1079", URL: "https://jirasw.nvidia.com/browse/SCM-1079"},
		{Key: "SCM-3791", URL: "https://jirasw.nvidia.com/browse/SCM-3791"},
	}
	cfg := &ReportConfig{}
	out := RenderURLReport(issues, cfg)
	if out == "" {
		t.Fatal("RenderURLReport returned empty string")
	}
	if !strings.HasPrefix(out, "https://jirasw.nvidia.com/issues/") {
		t.Errorf("expected base URL from issue, got %s", out)
	}
	if !strings.Contains(out, "jql=") {
		t.Error("expected jql param in URL")
	}
	if !strings.Contains(out, "SCM-1079") || !strings.Contains(out, "SCM-3791") {
		t.Error("expected issue keys in URL")
	}
	if !strings.Contains(out, "order+by+assignee+ASC") {
		t.Error("expected order by assignee in JQL")
	}
}

func TestRenderURLReport_empty(t *testing.T) {
	cfg := &ReportConfig{}
	out := RenderURLReport(nil, cfg)
	if out != "" {
		t.Errorf("expected empty string for no issues, got %q", out)
	}
}

func TestRenderSimpleReport(t *testing.T) {
	issues := []*IssueData{
		{Key: "A-1", Summary: "First task", Status: "in progress", Type: "story", StatusEmoji: "▶️", TrendingEmoji: "🟢", URL: "https://jira/browse/A-1"},
		{Key: "A-2", Summary: "Second task", Status: "ready for work", Type: "story", StatusEmoji: "🪏", TrendingEmoji: "⚪", URL: "https://jira/browse/A-2"},
	}
	cfg := &ReportConfig{}
	out := RenderSimpleReport(issues, cfg)
	if out == "" {
		t.Fatal("RenderSimpleReport returned empty string")
	}
	if strings.Contains(out, "https://") || strings.Contains(out, "jira") {
		t.Error("simple output must not contain URLs")
	}
	if !strings.Contains(out, "🟢") || !strings.Contains(out, "▶️") || !strings.Contains(out, "A-1") || !strings.Contains(out, "First task") {
		t.Errorf("expected first line with emoji, status, type, key, summary; got: %s", out)
	}
	if !strings.Contains(out, "⚪") || !strings.Contains(out, "🪏") || !strings.Contains(out, "A-2") || !strings.Contains(out, "Second task") {
		t.Errorf("expected second line with emoji, status, type, key, summary; got: %s", out)
	}
}

func TestServerBaseFromIssueURL(t *testing.T) {
	if got := serverBaseFromIssueURL("https://jirasw.nvidia.com/browse/SCM-1079"); got != "https://jirasw.nvidia.com" {
		t.Errorf("serverBaseFromIssueURL = %q, want https://jirasw.nvidia.com", got)
	}
	if got := serverBaseFromIssueURL(""); got != "" {
		t.Errorf("serverBaseFromIssueURL empty = %q", got)
	}
}

func TestFilterAndSortIssues_skipsNil(t *testing.T) {
	issues := []*IssueData{
		nil,
		{Key: "X-1", Summary: "Valid", Updated: "2025-01-15"},
		nil,
	}
	cfg := &ReportConfig{}
	out := filterAndSortIssues(issues, cfg)
	if len(out) != 1 {
		t.Errorf("expected 1 issue after filtering nils, got %d", len(out))
	}
	if out[0].Key != "X-1" {
		t.Errorf("expected X-1, got %s", out[0].Key)
	}
}
