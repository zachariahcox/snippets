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
	if got := GetStatusPriority("unknown"); got != 7 {
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
	if !isStale("2000-01-01") {
		t.Error("isStale(2000-01-01) should be true")
	}
	if !isStale("2000-01-01") {
		t.Error("isStale(2000-01-01) should be true")
	}
	if isStale("") {
		t.Error("isStale(\"\") should be false")
	}
	if isStale("None") {
		t.Error("isStale(None) should be false")
	}
	if !isStale("2000-01-01") {
		t.Error("isStale(2000-01-01) should be true")
	}
}

func TestIsDueWithinNextMonth(t *testing.T) {
	// DaysFromNow uses UTC calendar dates for "today"; match that so local TZ does not flip tomorrow to 0 days.
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	twoMonths := time.Now().UTC().AddDate(0, 2, 0).Format("2006-01-02")

	if isDueWithinDays("", 30) {
		t.Error("isDueWithinNextMonth(\"\") want false")
	}
	if isDueWithinDays("None", 30) {
		t.Error("isDueWithinNextMonth(None) want false")
	}
	if !isDueWithinDays(tomorrow, 30) {
		t.Error("isDueWithinNextMonth(tomorrow) want true")
	}
	if isDueWithinDays(twoMonths, 30) {
		t.Error("isDueWithinNextMonth(2 months) want false")
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
			Due:           "2025-01-01",
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
	if !strings.Contains(out, "| comment |") {
		t.Error("expected trending comment column in markdown header")
	}
	if !strings.Contains(out, "story") {
		t.Error("expected issue type (story) in markdown output")
	}
}

func TestRenderMarkdownStatusSummary(t *testing.T) {
	issues := []*IssueData{
		{Key: "A-1", Status: "in progress"},
		{Key: "A-2", Status: "in progress"},
		{Key: "A-3", Status: "done"},
		{Key: "A-4", Status: "done"},
		{Key: "A-5", Status: "done"},
	}
	cfg := &ReportConfig{Title: "Sprint"}
	out := RenderMarkdownStatusSummary(issues, cfg)
	if !strings.Contains(out, "* total issues: 5*") {
		t.Errorf("want total 5: %s", out)
	}
	if !strings.Contains(out, "| done | 3 | 60.0% |") {
		t.Errorf("want done row: %s", out)
	}
	if !strings.Contains(out, "| in progress | 2 | 40.0% |") {
		t.Errorf("want in progress row: %s", out)
	}
	// Higher count first: done before in progress
	doneIdx := strings.Index(out, "| done |")
	ipIdx := strings.Index(out, "| in progress |")
	if doneIdx <= 0 || ipIdx <= 0 || doneIdx >= ipIdx {
		t.Errorf("want descending count order (done then in progress): %s", out)
	}
}

func TestRenderMarkdownStatusSummary_empty(t *testing.T) {
	out := RenderMarkdownStatusSummary(nil, &ReportConfig{Title: "Empty"})
	if !strings.Contains(out, "* total issues: 0*") || !strings.Contains(out, "No issues") {
		t.Errorf("empty set: %s", out)
	}
}

func TestRenderMarkdownStatusSummary_escapesStatus(t *testing.T) {
	issues := []*IssueData{{Key: "X-1", Status: `Open | review`}}
	out := RenderMarkdownStatusSummary(issues, &ReportConfig{Title: "T"})
	if !strings.Contains(out, `Open \| review`) {
		t.Errorf("status cell should escape pipe: %s", out)
	}
}

func TestRenderMarkdownReport_titleEscaped(t *testing.T) {
	summary := `Fix [P-1] (urgent) | see #2`
	jiraURL := "https://jira.example.com/browse/A-1"
	issues := []*IssueData{{Key: "A-1", URL: jiraURL, Summary: summary, Status: "open", Type: "story"}}
	cfg := &ReportConfig{Title: `Report | Q1 [draft] #1`}
	out := RenderMarkdownReport(issues, cfg)
	if !strings.Contains(out, "### Report \\| Q1 \\[draft\\] \\#1 @") {
		snippet := out
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		t.Errorf("title should escape markdown punctuation: %s", snippet)
	}
	wantLink := "[Fix \\[P-1\\] \\(urgent\\) \\| see \\#2](" + jiraURL + ")"
	if !strings.Contains(out, wantLink) {
		t.Errorf("issue summary in link text should be escaped; want substring:\n%s\n\noutput:\n%s", wantLink, out)
	}
}

func TestRenderMarkdownReport_subtaskRow(t *testing.T) {
	issues := []*IssueData{
		{
			Key:             "PROJ-123-1",
			URL:             "https://jira/browse/PROJ-123-1",
			Summary:         "Subtask one",
			Status:          "in progress",
			Type:            "subtask",
			Assignee:        "Bob",
			Due:             "2025-02-01",
			Updated:         "2025-01-15",
			TrendingEmoji:   "🟢",
			Trending:        "in progress",
			TrendingComment: "Watch dependency X",
		},
	}
	cfg := &ReportConfig{Title: "Children Report"}
	out := RenderMarkdownReport(issues, cfg)
	if out == "" {
		t.Error("RenderMarkdownReport returned empty string")
	}
	if !strings.Contains(out, "| type |") {
		t.Error("expected type column in markdown header")
	}
	if !strings.Contains(out, "Subtask one") {
		t.Error("output missing issue summary")
	}
	if !strings.Contains(out, "subtask") {
		t.Error("expected issue type (subtask) in markdown output")
	}
	if !strings.Contains(out, "Watch dependency X") {
		t.Error("expected trending comment in markdown row")
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
	if !strings.Contains(out, `"trending_comment"`) {
		t.Error("JSON output should include trending_comment field")
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
			Due:           "2025-02-01",
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
	if !strings.Contains(out, "trending_comment") {
		t.Error("expected trending_comment column in CSV header")
	}
}

func TestRenderCSVReport_noParentColumns(t *testing.T) {
	issues := []*IssueData{
		{
			Key:           "PROJ-123-1",
			Summary:       "Subtask",
			Assignee:      "Bob",
			TrendingEmoji: "🟢",
			Trending:      "in progress",
		},
	}
	cfg := &ReportConfig{}
	out := RenderCSVReport(issues, cfg)
	if strings.Contains(out, "parent_key") || strings.Contains(out, "parent_summary") || strings.Contains(out, "parent_url") {
		t.Error("CSV should not include parent_* columns (use Children in JSON for hierarchy)")
	}
	if !strings.Contains(out, "PROJ-123-1") {
		t.Error("expected issue key in output")
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
			Due:           "2025-02-01",
			Comment:       IssueComment{Url: "https://jira/comment/1", Created: "2025-01-15"},
		},
		{
			Key:           "A-2",
			URL:           "https://jira/browse/A-2",
			Summary:       "Second issue",
			Due:           "2025-02-02",
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
	if strings.Contains(out, ", ()") {
		t.Error("empty trending comment should not add a trailing segment")
	}
	issues[0].TrendingComment = "Needs review"
	out2 := RenderSlackReport(issues, cfg)
	if !strings.Contains(out2, ", (Needs review)") {
		t.Error("non-empty trending comment should appear at end of line")
	}
}

func TestSplitJQLOrderBy_lastOrderByWins(t *testing.T) {
	main, ob := splitJQLOrderBy("project = X order by rank ASC order by key DESC")
	if main != "project = X order by rank ASC" {
		t.Errorf("main = %q", main)
	}
	if ob != "order by key DESC" {
		t.Errorf("orderBySuffix = %q", ob)
	}
}

func TestSplitJQLOrderBy_newlineBeforeOrderBy(t *testing.T) {
	jql := "project = ABC\nand status = open\norder by updated ASC"
	main, ob := splitJQLOrderBy(jql)
	if main == "" || ob == "" {
		t.Fatalf("split failed: main=%q ob=%q", main, ob)
	}
	if !strings.HasPrefix(strings.ToLower(ob), "order by ") {
		t.Errorf("order suffix: %q", ob)
	}
	merged := mergeURLReportOrderBy(jql)
	if strings.Count(strings.ToLower(merged), "order by") != 1 {
		t.Errorf("want exactly one ORDER BY, got: %q", merged)
	}
	if !strings.Contains(merged, ", assignee ASC") {
		t.Errorf("want assignee merged with comma, got: %q", merged)
	}
}

func TestMergeURLReportOrderBy(t *testing.T) {
	if got := mergeURLReportOrderBy(""); got != "" {
		t.Errorf("empty: %q", got)
	}
	if got := mergeURLReportOrderBy("project = FOO"); got != "project = FOO and order by assignee ASC" {
		t.Errorf("no order: %q", got)
	}
	if got := mergeURLReportOrderBy("project = FOO AND ORDER BY updated DESC"); got != "project = FOO AND ORDER BY updated DESC, assignee ASC" {
		t.Errorf("merge: %q", got)
	}
	if got := mergeURLReportOrderBy("project = FOO and order by assignee DESC"); got != "project = FOO and order by assignee DESC" {
		t.Errorf("assignee already present: %q", got)
	}
}

func TestRenderURLReport(t *testing.T) {
	issues := []*IssueData{
		{Key: "ABC-1079", URL: "https://jira.example.com/browse/ABC-1079"},
		{Key: "ABC-3791", URL: "https://jira.example.com/browse/ABC-3791"},
	}
	cfg := &ReportConfig{}
	out := RenderURLReport(issues, cfg)
	if out == "" {
		t.Fatal("RenderURLReport returned empty string")
	}
	if !strings.HasPrefix(out, "https://jira.example.com/issues/") {
		t.Errorf("expected base URL from issue, got %s", out)
	}
	if !strings.Contains(out, "jql=") {
		t.Error("expected jql param in URL")
	}
	if !strings.Contains(out, "ABC-1079") || !strings.Contains(out, "ABC-3791") {
		t.Error("expected issue keys in URL")
	}
	if !strings.Contains(out, "order+by+assignee+ASC") {
		t.Error("expected order by assignee in JQL")
	}
}

func TestRenderURLReport_jqlMergesAssigneeAfterExistingOrderBy(t *testing.T) {
	issues := []*IssueData{{Key: "X-1", URL: "https://jira.example.com/browse/X-1"}}
	cfg := &ReportConfig{JQLQuery: "project = BAR order by updated DESC"}
	out := RenderURLReport(issues, cfg)
	if !strings.Contains(out, "order+by+updated+DESC") && !strings.Contains(out, "order+by+updated+desc") {
		t.Errorf("expected user's order by in URL: %s", out)
	}
	if !strings.Contains(out, "assignee+ASC") {
		t.Errorf("expected merged assignee tie-break in URL: %s", out)
	}
}

func TestRenderURLReport_jqlExistingOrderByAssigneeNotDuplicated(t *testing.T) {
	issues := []*IssueData{{Key: "X-1", URL: "https://jira.example.com/browse/X-1"}}
	cfg := &ReportConfig{JQLQuery: "project = BAR order by assignee DESC"}
	out := RenderURLReport(issues, cfg)
	if !strings.Contains(out, "assignee+DESC") {
		t.Errorf("expected assignee DESC from user JQL: %s", out)
	}
	// Must not append ", assignee ASC" when assignee is already in ORDER BY.
	if strings.Contains(out, "assignee+DESC%2C+assignee") || strings.Contains(out, "assignee+DESC,+assignee") {
		t.Errorf("should not append a second assignee sort: %s", out)
	}
}

func TestRenderURLReport_reusesJQL(t *testing.T) {
	issues := []*IssueData{{Key: "ABC-1079", URL: "https://jira.example.com/browse/ABC-1079"}}
	cfg := &ReportConfig{JQLQuery: "project = FOO AND status = Open"}
	out := RenderURLReport(issues, cfg)
	if !strings.Contains(out, "project+%3D+FOO") && !strings.Contains(out, "project=FOO") {
		t.Errorf("expected original JQL in URL: %s", out)
	}
	if strings.Contains(out, "key+in+") || strings.Contains(out, "key%20in%20") {
		t.Errorf("did not expect key in clause when JQL is set: %s", out)
	}
}

func TestRenderURLReport_jqlWithSince(t *testing.T) {
	jan15 := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	issues := []*IssueData{
		{Key: "X-1", URL: "https://jira.example.com/browse/X-1", Updated: "2025-01-20T12:00:00.000Z"},
	}
	cfg := &ReportConfig{
		JQLQuery:     "project = BAR",
		UpdatedAfter: &jan15,
	}
	out := RenderURLReport(issues, cfg)
	if !strings.Contains(out, "updated+%3E%3D+%222025-01-15%22") && !strings.Contains(out, `updated>=%222025-01-15%22`) {
		t.Errorf("expected updated >= date in JQL: %s", out)
	}
}

func TestRenderURLReport_keyListNoNeedsUpdateOmitsSinceInJQL(t *testing.T) {
	jan15 := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	issues := []*IssueData{
		{Key: "X-1", URL: "https://jira.example.com/browse/X-1", Updated: "2025-01-20T12:00:00.000Z"},
	}
	cfg := &ReportConfig{UpdatedAfter: &jan15}
	out := RenderURLReport(issues, cfg)
	if strings.Contains(strings.ToLower(out), "updated") {
		t.Errorf("key-only without needs-update should not add updated clause: %s", out)
	}
	if !strings.Contains(out, "X-1") {
		t.Errorf("expected key in URL: %s", out)
	}
}

func TestRenderURLReport_keyListNeedsUpdateAndSince(t *testing.T) {
	jan15 := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	noComment := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	issues := []*IssueData{
		{Key: "X-1", URL: "https://jira.example.com/browse/X-1", Updated: "2025-01-20T12:00:00.000Z", Comment: IssueComment{Created: "2025-01-01"}},
		{Key: "X-2", URL: "https://jira.example.com/browse/X-2", Updated: "2025-01-18T12:00:00.000Z", Comment: IssueComment{Created: "2025-01-02"}},
	}
	cfg := &ReportConfig{
		UpdatedAfter:   &jan15,
		NoCommentAfter: &noComment,
	}
	out := RenderURLReport(issues, cfg)
	if !strings.Contains(out, "key+in+") && !strings.Contains(out, "key%20in%20") {
		t.Errorf("expected key in (...) for needs-update: %s", out)
	}
	if !strings.Contains(out, "X-1") || !strings.Contains(out, "X-2") {
		t.Errorf("expected both keys in URL: %s", out)
	}
	if !strings.Contains(out, "updated+%3E%3D+%222025-01-15%22") && !strings.Contains(out, `updated>=%222025-01-15%22`) {
		t.Errorf("expected since as updated >= in JQL: %s", out)
	}
}

func TestRenderURLReport_needsUpdateUsesKeyIn(t *testing.T) {
	noComment := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	issues := []*IssueData{
		{Key: "K-1", URL: "https://jira.example.com/browse/K-1", Comment: IssueComment{Created: "2025-01-01"}},
		{Key: "K-2", URL: "https://jira.example.com/browse/K-2", Comment: IssueComment{Created: "2025-01-01"}},
	}
	cfg := &ReportConfig{
		JQLQuery:       "project = ZZZ",
		NoCommentAfter: &noComment,
	}
	out := RenderURLReport(issues, cfg)
	if !strings.Contains(out, "key+in+") && !strings.Contains(out, "key%20in%20") {
		t.Errorf("expected key in (...) not project JQL: %s", out)
	}
	if !strings.Contains(out, "K-1") || !strings.Contains(out, "K-2") {
		t.Errorf("expected keys in URL: %s", out)
	}
	if strings.Contains(out, "ZZZ") {
		t.Errorf("needs-update mode should not reuse JQL: %s", out)
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
	withComment := []*IssueData{
		{Key: "B-1", Summary: "With note", Status: "in progress", Type: "story", StatusEmoji: "▶️", TrendingEmoji: "🟢", TrendingComment: "Follow up"},
	}
	out2 := RenderSimpleReport(withComment, cfg)
	if !strings.Contains(out2, "Follow up") {
		t.Errorf("expected trending comment in simple output: %q", out2)
	}
}

// RenderSimpleReport due column: DaysFromNow + trending "done" shows "done", else "due in N days", else "(no due date)".
func TestRenderSimpleReport_dueColumn(t *testing.T) {
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	farFuture := "2099-06-15"
	cfg := &ReportConfig{}

	noDue := []*IssueData{{
		Key: "N-1", Summary: "No due", Status: "in progress", Type: "story",
		StatusEmoji: "🚀", TrendingEmoji: "🟢", Trending: "on track", Due: "",
	}}
	out := RenderSimpleReport(noDue, cfg)
	if !strings.Contains(out, "(no due date)") {
		t.Errorf("empty Due: want substring '(no due date)', got:\n%s", out)
	}

	doneStillDue := []*IssueData{{
		Key: "D-1", Summary: "Shipped", Status: "resolved", Type: "story",
		StatusEmoji: "🎉", TrendingEmoji: "🟣", Trending: "done", Due: farFuture,
	}}
	out = RenderSimpleReport(doneStillDue, cfg)
	if !strings.Contains(out, "D-1") {
		t.Errorf("expected key D-1 in output:\n%s", out)
	}
	hasDoneWord := false
	for _, w := range strings.Fields(strings.TrimSpace(out)) {
		if w == "done" {
			hasDoneWord = true
			break
		}
	}
	if !hasDoneWord {
		t.Errorf("Trending done with parseable Due: want word 'done' in simple row, got:\n%s", out)
	}

	activeTomorrow := []*IssueData{{
		Key: "A-1", Summary: "Active", Status: "in progress", Type: "story",
		StatusEmoji: "🚀", TrendingEmoji: "🟢", Trending: "on track", Due: tomorrow,
	}}
	out = RenderSimpleReport(activeTomorrow, cfg)
	if !strings.Contains(out, "due in 1 days") {
		t.Errorf("active issue due tomorrow: want 'due in 1 days' in output, got:\n%s", out)
	}
}

func TestServerBaseFromIssueURL(t *testing.T) {
	if got := serverBaseFromIssueURL("https://jira.example.com/browse/ABC-1079"); got != "https://jira.example.com" {
		t.Errorf("serverBaseFromIssueURL = %q, want https://jira.example.com", got)
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
