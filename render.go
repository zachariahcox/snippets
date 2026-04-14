package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

func RenderReport(parentIssues []*IssueData, cfg *ReportConfig) {
	if cfg == nil || parentIssues == nil {
		return
	}

	// Render output
	var outputData string
	issuesToRender := parentIssues
	if cfg.JSONOutput {
		outputData = RenderJSONReport(issuesToRender, cfg)
	} else if cfg.CSVOutput {
		outputData = RenderCSVReport(issuesToRender, cfg)
	} else if cfg.SlackOutput {
		outputData = RenderSlackReport(issuesToRender, cfg)
	} else if cfg.URLOutput {
		outputData = RenderURLReport(issuesToRender, cfg)
	} else if cfg.SimpleOutput {
		outputData = RenderSimpleReport(issuesToRender, cfg)
	} else if cfg.SummaryOutput {
		outputData = RenderMarkdownStatusSummary(issuesToRender, cfg)
	} else {
		outputData = RenderMarkdownReport(issuesToRender, cfg)
	}

	// Output
	if cfg.OutputFile != "" {
		f, err := os.OpenFile(cfg.OutputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logError("Error opening file %s: %v", cfg.OutputFile, err)
			fmt.Println(outputData)
			return
		}
		defer f.Close()

		fi, _ := f.Stat()
		if fi.Size() > 0 {
			f.WriteString("\n\n\n\n")
		}
		f.WriteString(outputData)
	} else {
		fmt.Println(outputData)
	}
}

// escapeMarkdownInline backslash-escapes punctuation so titles and issue summaries render
// literally in ATX headings and [label](url) link labels without breaking markdown.
func escapeMarkdownInline(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\', '`', '*', '_', '{', '}', '[', ']', '(', ')', '#', '!', '|':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// trendingCommentForDisplay trims whitespace; empty means no comment (render as blank).
func trendingCommentForDisplay(s string) string {
	return strings.TrimSpace(s)
}

// RenderMarkdownReport renders issues as a markdown report
func RenderMarkdownReport(issues []*IssueData, cfg *ReportConfig) string {
	logInfo("Rendering markdown report for %d issues", len(issues))
	issues = filterAndSortIssues(issues, cfg)

	var result []string
	result = append(result, fmt.Sprintf("\n### %s @ %s", escapeMarkdownInline(cfg.Title), time.Now().Format(time.RFC3339)))

	result = append(result, fmt.Sprintf("* row count: %d", len(issues)))

	// Render header row (type between trending and status); trending comment last
	result = append(result, "\n| trending | type | status | issue | assignee | due date | last update | comment |")
	result = append(result, "|---|---|---|---|:--|:--|:--|:--|")

	// Render rows
	for _, issue := range issues {
		// Format cells
		issueLink := fmt.Sprintf("[%s](%s)", escapeMarkdownInline(issue.Summary), issue.URL)
		trendingWithEmoji := fmt.Sprintf("%s %s", issue.TrendingEmoji, issue.Trending)
		dueDate := FormatDate(issue.Due)
		timestampLink := FormatTimestampWithLink(issue.Comment.Created, issue.Comment.Url, false)
		typeOrStatus := issue.Type
		if typeOrStatus == "" || strings.ToLower(typeOrStatus) == "unknown" {
			typeOrStatus = issue.Status
		}

		trendingCommentCell := trendingCommentForDisplay(issue.TrendingComment)
		if trendingCommentCell != "" {
			trendingCommentCell = escapeMarkdownInline(trendingCommentCell)
		}

		// Render row
		row := fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s |",
			trendingWithEmoji, typeOrStatus, issue.Status, issueLink, issue.Assignee, dueDate, timestampLink, trendingCommentCell)
		result = append(result, row)
	}

	result = append(result, "\n")
	return strings.Join(result, "\n")
}

// RenderMarkdownStatusSummary renders a markdown table of counts and percents by issue Status
// for the filtered list (same filters as other reports).
func RenderMarkdownStatusSummary(issues []*IssueData, cfg *ReportConfig) string {
	issues = filterAndSortIssues(issues, cfg)
	title := ""
	if cfg != nil {
		title = escapeMarkdownInline(cfg.Title)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n### %s — status summary @ %s\n\n", title, time.Now().Format(time.RFC3339))
	n := len(issues)
	fmt.Fprintf(&b, "* total issues: %d*\n\n", n)
	if n == 0 {
		b.WriteString("*No issues in the filtered set.*\n\n")
		return b.String()
	}

	counts := make(map[string]int)
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		st := strings.TrimSpace(issue.Status)
		if st == "" {
			st = "unknown"
		}
		counts[st]++
	}

	type statusCount struct {
		status string
		count  int
	}
	rows := make([]statusCount, 0, len(counts))
	for s, c := range counts {
		rows = append(rows, statusCount{s, c})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		return rows[i].status < rows[j].status
	})

	b.WriteString("| status | count | percent |\n")
	b.WriteString("|---:|---:|---:|\n")
	for _, r := range rows {
		pct := 100.0 * float64(r.count) / float64(n)
		fmt.Fprintf(&b, "| %s | %d | %.1f%% |\n", escapeMarkdownInline(r.status), r.count, pct)
	}
	b.WriteString("\n")
	return b.String()
}

func RenderJSONReport(issues []*IssueData, cfg *ReportConfig) string {
	issues = filterAndSortIssues(issues, cfg)
	jsonData, err := json.Marshal(issues)
	if err != nil {
		logError("Failed to marshal JSON: %v", err)
		return ""
	}
	return string(jsonData)
}

const csvSep = "🐱"

func escapeCSVField(s string) string {
	if strings.Contains(s, csvSep) || strings.Contains(s, "\n") || strings.Contains(s, `"`) {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

func RenderCSVReport(issues []*IssueData, cfg *ReportConfig) string {
	issues = filterAndSortIssues(issues, cfg)

	headers := []string{
		"key", "url", "summary", "status", "status_emoji", "assignee", "priority",
		"created", "updated", "target_end",
		"trending", "trending_emoji", "type", "comment_url", "comment_created",
		"trending_comment",
	}
	escapedHeaders := make([]string, len(headers))
	for i, h := range headers {
		escapedHeaders[i] = escapeCSVField(h)
	}
	var result []string
	result = append(result, strings.Join(escapedHeaders, csvSep))

	for _, issue := range issues {
		commentCreated := issue.Comment.Created
		if commentCreated == "" {
			commentCreated = "N/A"
		}
		row := []string{
			issue.Key,
			issue.URL,
			issue.Summary,
			issue.Status,
			issue.StatusEmoji,
			issue.Assignee,
			issue.Priority,
			issue.Created,
			issue.Updated,
			FormatDate(issue.Due),
			issue.Trending,
			issue.TrendingEmoji,
			issue.Type,
			issue.Comment.Url,
			commentCreated,
			trendingCommentForDisplay(issue.TrendingComment),
		}
		escapedRow := make([]string, len(row))
		for i, v := range row {
			escapedRow[i] = escapeCSVField(v)
		}
		result = append(result, strings.Join(escapedRow, csvSep))
	}
	return strings.Join(result, "\n")
}

// RenderSlackReport renders issues as a Slack-formatted numbered list
func RenderSlackReport(issues []*IssueData, cfg *ReportConfig) string {
	issues = filterAndSortIssues(issues, cfg)

	var result []string
	for i, issue := range issues {
		line := fmt.Sprintf("%d. %s [%s](%s), (due %s)", i+1, issue.TrendingEmoji, issue.Summary, issue.URL, FormatDate(issue.Due))
		if issue.Comment.Url != "" {
			line += fmt.Sprintf(" ([last update](%s))", issue.Comment.Url)
		}
		if tc := trendingCommentForDisplay(issue.TrendingComment); tc != "" {
			line += fmt.Sprintf(", (%s)", tc)
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// RenderSimpleReport renders one line per issue as an aligned table using text/tabwriter.
func RenderSimpleReport(issues []*IssueData, cfg *ReportConfig) string {
	issues = filterAndSortIssues(issues, cfg)
	if len(issues) == 0 {
		return ""
	}
	var buf bytes.Buffer
	// minwidth=4 so emoji columns get padding and align; tabwriter counts runes, not display width
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	// column headers: trending, status, due, type, key, summary, trending comment
	format := "%s\t%s\t%s\t%s\t%s\t%s\t%s\n"
	for _, issue := range issues {
		days, ok := DaysFromNow(issue.Due)
		dueStr := "?"
		if ok {
			if issue.Trending == "done" {
				dueStr = "done"
			} else {
				dueStr = fmt.Sprintf("due in %d days", days)
			}
		} else {
			dueStr = "(no due date)"
		}
		fmt.Fprintf(tw, format,
			issue.TrendingEmoji,
			issue.StatusEmoji,
			dueStr,
			issue.Type,
			issue.Key,
			strings.ReplaceAll(issue.Summary, "\n", " "),
			trendingCommentForDisplay(issue.TrendingComment))
	}
	tw.Flush()
	return strings.TrimRight(buf.String(), "\n")
}

// serverBaseFromIssueURL returns the Jira server base URL (e.g. https://jira.example.com) from an issue browse URL.
func serverBaseFromIssueURL(issueURL string) string {
	u, err := url.Parse(issueURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimSuffix(u.Scheme+"://"+u.Host, "/")
}

// jqlIssueKeysClause returns `key in (A, B, C)` for the given issues.
func jqlIssueKeysClause(issues []*IssueData) string {
	keys := make([]string, len(issues))
	for i, issue := range issues {
		keys[i] = issue.Key
	}
	return fmt.Sprintf("key in (%s)", strings.Join(keys, ", "))
}

// jqlWithUpdatedSince wraps main in (main) AND updated >= "date" when t is non-nil.
func jqlWithUpdatedSince(main string, t *time.Time) string {
	main = strings.TrimSpace(main)
	if t == nil {
		return main
	}
	dateStr := t.Format("2006-01-02")
	if main == "" {
		return fmt.Sprintf(`updated >= "%s"`, dateStr)
	}
	return fmt.Sprintf("(%s) AND updated >= \"%s\"", main, dateStr)
}

// jqlOrderByClauseStart matches the start of an ORDER BY clause (any leading whitespace before "order").
var jqlOrderByClauseStart = regexp.MustCompile(`(?i)\border\s+by\s+`)

// splitJQLOrderBy splits on the last ORDER BY clause (case-insensitive). Whitespace before "order"
// may be a space, newline, etc.; a literal " order by " substring is not required.
func splitJQLOrderBy(jql string) (main, orderBySuffix string) {
	jql = strings.TrimSpace(jql)
	if jql == "" {
		return "", ""
	}
	idxs := jqlOrderByClauseStart.FindAllStringIndex(jql, -1)
	if len(idxs) == 0 {
		return jql, ""
	}
	start := idxs[len(idxs)-1][0]
	main = strings.TrimSpace(jql[:start])
	orderBySuffix = strings.TrimSpace(jql[start:])
	if main == "" {
		return jql, ""
	}
	return main, orderBySuffix
}

var jqlOrderByContainsAssignee = regexp.MustCompile(`(?i)order\s+by\b.*\bassignee\b`)

// mergeURLReportOrderBy ensures the JQL ends with a sensible ORDER BY. If there is no ORDER BY,
// it appends "order by assignee ASC". If one exists but does not reference assignee, it appends
// ", assignee ASC" so the user's sort keys stay primary and assignee is a tie-breaker.
func mergeURLReportOrderBy(jql string) string {
	const urlReportOrderByAssignee = "assignee ASC"

	jql = strings.TrimSpace(jql)
	if jql == "" {
		return jql
	}
	main, orderClause := splitJQLOrderBy(jql)
	if orderClause == "" {
		// add new order clause
		return jql + " and order by " + urlReportOrderByAssignee
	}
	if jqlOrderByContainsAssignee.MatchString(orderClause) {
		// assignee already in order clause, do not modify
		return jql
	}
	// add additional order clause
	return main + " " + orderClause + ", " + urlReportOrderByAssignee
}

// RenderURLReport emits a single Jira issues URL with a jql= query parameter.
// Key-only runs use key in (...); --since is omitted unless --needs-update is also in effect
// (then (key in (...)) AND updated >= "date" from --since only). User JQL is reused with --since
// merged via jqlWithUpdatedSince. ORDER BY is handled by mergeURLReportOrderBy.
//
// NoCommentAfter (--needs-update) is about last-comment age, not issue updated; it cannot be encoded
// as updated >= ... in JQL, so when set the URL uses key in (...) for the filtered result set.
func RenderURLReport(issues []*IssueData, cfg *ReportConfig) string {
	issues = filterAndSortIssues(issues, cfg)
	if len(issues) == 0 {
		return ""
	}
	base := serverBaseFromIssueURL(issues[0].URL)
	if base == "" {
		return ""
	}

	var jql string
	switch {
	case cfg.NoCommentAfter != nil:
		jql = mergeURLReportOrderBy(jqlWithUpdatedSince(jqlIssueKeysClause(issues), cfg.UpdatedAfter))
	case strings.TrimSpace(cfg.JQLQuery) != "":
		main, orderSuffix := splitJQLOrderBy(strings.TrimSpace(cfg.JQLQuery))
		if main == "" {
			main = strings.TrimSpace(cfg.JQLQuery)
			orderSuffix = ""
		}
		main = jqlWithUpdatedSince(main, cfg.UpdatedAfter)
		jql = mergeURLReportOrderBy(strings.TrimSpace(main + " " + orderSuffix))
	default:
		jql = mergeURLReportOrderBy(jqlIssueKeysClause(issues))
	}

	params := url.Values{"jql": {jql}}
	return base + "/issues/?" + params.Encode()
}

func filterAndSortIssues(issues []*IssueData, cfg *ReportConfig) []*IssueData {
	// Filter issues
	var filteredIssues []*IssueData
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		// Exclude issues updated before the since date
		if cfg.UpdatedAfter != nil && issue.Updated != "" && issue.Updated != "N/A" {
			updateDate, err := ParseJiraDate(issue.Updated)
			if err != nil {
				logWarning("Could not parse date '%s': %v", issue.Updated, err)
				continue
			}
			if updateDate.Before(*cfg.UpdatedAfter) {
				continue
			}
		}

		// Exclude issues with a comment since the "no comment since" date
		if cfg.NoCommentAfter != nil && issue.Comment.Created != "" && issue.Comment.Created != "N/A" {
			commentDate, err := ParseJiraDate(issue.Comment.Created)
			if err != nil {
				logWarning("Could not parse date '%s': %v", issue.Comment.Created, err)
				continue
			}
			if commentDate.After(*cfg.NoCommentAfter) {
				continue
			}
		}

		filteredIssues = append(filteredIssues, issue)
	}
	logInfo("Filters excluded %d issues. %d issues remain.", len(issues)-len(filteredIssues), len(filteredIssues))

	// Sort issues by status, target end, updated
	sort.Slice(filteredIssues, func(i, j int) bool {
		// By status priority
		pi := GetStatusPriority(filteredIssues[i].Status)
		pj := GetStatusPriority(filteredIssues[j].Status)
		if pi != pj {
			return pi < pj
		}

		// By target end
		ti := filteredIssues[i].Due
		tj := filteredIssues[j].Due
		if ti == "" {
			ti = "9999-99-99"
		}
		if tj == "" {
			tj = "9999-99-99"
		}
		if ti != tj {
			return ti < tj
		}

		// By updated
		ui := filteredIssues[i].Updated
		uj := filteredIssues[j].Updated
		if ui != uj {
			return ui < uj
		}

		// By summary
		return filteredIssues[i].Summary < filteredIssues[j].Summary
	})
	return filteredIssues
}

// GetStatusPriority returns the sort priority for a status (index in statusOrder, or 999 if unknown)
func GetStatusPriority(statusName string) int {
	s := strings.ToLower(strings.TrimSpace(statusName))
	if i := slices.Index(statusOrder, s); i >= 0 {
		return i
	}
	return 999
}

// FormatDate formats a date string for display
func FormatDate(dateStr string) string {
	if dateStr == "" {
		return "N/A"
	}
	t, err := ParseJiraDate(dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("2006-01-02")
}

// FormatTimestampWithLink formats a timestamp as a markdown link
func FormatTimestampWithLink(timestamp, issueURL string, includeDaysAgo bool) string {
	if timestamp == "" || timestamp == "N/A" || issueURL == "" {
		return "N/A"
	}

	t, err := ParseJiraDate(timestamp)
	if err != nil {
		logWarning("Could not format timestamp '%s': %v", timestamp, err)
		return timestamp
	}

	dateStr := t.Format("2006-01-02")
	daysText := ""

	if includeDaysAgo {
		now := time.Now().UTC()
		tUTC := t.UTC()
		delta := now.Sub(tUTC)
		daysAgo := int(delta.Hours() / 24)

		switch daysAgo {
		case 0:
			daysText = " (today)"
		case 1:
			daysText = " (1 day ago)"
		default:
			daysText = fmt.Sprintf(" (%d days ago)", daysAgo)
		}
	}

	return fmt.Sprintf("[%s%s](%s)", dateStr, daysText, issueURL)
}
