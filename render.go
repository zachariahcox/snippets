package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

func RenderReport(parentIssues []*IssueData, childIssues []*IssueData, cfg *ReportConfig) {
	if cfg == nil || (parentIssues == nil && childIssues == nil) {
		return
	}

	// Render output
	var outputData string
	issuesToRender := parentIssues
	if cfg.ShowChildren {
		issuesToRender = childIssues
	}
	if cfg.JSONOutput {
		outputData = RenderJSONReport(issuesToRender, cfg)
	} else if cfg.CSVOutput {
		outputData = RenderCSVReport(issuesToRender, cfg)
	} else if cfg.SlackOutput {
		outputData = RenderSlackReport(issuesToRender, cfg)
	} else if cfg.URLOutput {
		outputData = RenderURLReport(issuesToRender, cfg)
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

// RenderMarkdownReport renders issues as a markdown report
func RenderMarkdownReport(issues []*IssueData, cfg *ReportConfig) string {
	logInfo("Rendering markdown report for %d issues", len(issues))
	issues = filterAndSortIssues(issues, cfg)

	var result []string
	result = append(result, fmt.Sprintf("\n### %s", cfg.Title))
	result = append(result, fmt.Sprintf("* generated at: %s", time.Now().Format(time.RFC3339)))
	result = append(result, fmt.Sprintf("* row count: %d", len(issues)))

	// Render header row
	if cfg.ShowChildren {
		result = append(result, "\n| status | parent | issue | assignee | target date | last update |")
		result = append(result, "|---|:--|:--|:--|:--|:--|")
	} else {
		result = append(result, "\n| status | issue | assignee | target date | last update |")
		result = append(result, "|---|:--|:--|:--|:--|")
	}

	// Render rows
	for _, issue := range issues {
		// Format cells
		issueLink := fmt.Sprintf("[%s](%s)", issue.Summary, issue.URL)
		statusWithEmoji := fmt.Sprintf("%s %s", issue.Emoji, issue.Trending)
		targetEnd := FormatDate(issue.TargetEnd)
		timestampLink := FormatTimestampWithLink(issue.Comment.Created, issue.Comment.Url, false)

		// Render row
		var row string
		if cfg.ShowChildren {
			parentLink := fmt.Sprintf("[%s](%s)", issue.ParentKey, issue.ParentURL)
			row = fmt.Sprintf("| %s | %s | %s | %s | %s | %s |",
				statusWithEmoji, parentLink, issueLink, issue.Assignee, targetEnd, timestampLink)
		} else {
			row = fmt.Sprintf("| %s | %s | %s | %s | %s |",
				statusWithEmoji, issueLink, issue.Assignee, targetEnd, timestampLink)
		}
		result = append(result, row)
	}

	result = append(result, "\n")
	return strings.Join(result, "\n")
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

	var result []string
	if cfg.ShowChildren {
		result = append(result, strings.Join([]string{
			escapeCSVField("status"),
			escapeCSVField("parent"),
			escapeCSVField("issue"),
			escapeCSVField("assignee"),
			escapeCSVField("target date"),
			escapeCSVField("last update"),
		}, csvSep))
	} else {
		result = append(result, strings.Join([]string{
			escapeCSVField("status"),
			escapeCSVField("issue"),
			escapeCSVField("assignee"),
			escapeCSVField("target date"),
			escapeCSVField("last update"),
		}, csvSep))
	}

	for _, issue := range issues {
		statusWithEmoji := fmt.Sprintf("%s %s", issue.Emoji, issue.Trending)
		targetEnd := FormatDate(issue.TargetEnd)
		lastUpdate := issue.Comment.Created
		if lastUpdate == "" {
			lastUpdate = "N/A"
		}

		if cfg.ShowChildren {
			result = append(result, strings.Join([]string{
				escapeCSVField(statusWithEmoji),
				escapeCSVField(issue.ParentKey),
				escapeCSVField(issue.Summary),
				escapeCSVField(issue.Assignee),
				escapeCSVField(targetEnd),
				escapeCSVField(lastUpdate),
			}, csvSep))
		} else {
			result = append(result, strings.Join([]string{
				escapeCSVField(statusWithEmoji),
				escapeCSVField(issue.Summary),
				escapeCSVField(issue.Assignee),
				escapeCSVField(targetEnd),
				escapeCSVField(lastUpdate),
			}, csvSep))
		}
	}
	return strings.Join(result, "\n")
}

// RenderSlackReport renders issues as a Slack-formatted numbered list
func RenderSlackReport(issues []*IssueData, cfg *ReportConfig) string {
	issues = filterAndSortIssues(issues, cfg)

	var result []string
	for i, issue := range issues {
		line := fmt.Sprintf("%d. %s [%s](%s), (due %s)", i+1, issue.Emoji, issue.Summary, issue.URL, FormatDate(issue.TargetEnd))
		if issue.Comment.Url != "" {
			line += fmt.Sprintf(" ([last update](%s))", issue.Comment.Url)
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// serverBaseFromIssueURL returns the Jira server base URL (e.g. https://jira.example.com) from an issue browse URL.
func serverBaseFromIssueURL(issueURL string) string {
	u, err := url.Parse(issueURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimSuffix(u.Scheme+"://"+u.Host, "/")
}

// RenderURLReport emits a single Jira issues URL with filtered keys as JQL.
// The server base URL is derived from the first issue's URL in IssueData.
func RenderURLReport(issues []*IssueData, cfg *ReportConfig) string {
	issues = filterAndSortIssues(issues, cfg)
	if len(issues) == 0 {
		return ""
	}
	base := serverBaseFromIssueURL(issues[0].URL)
	if base == "" {
		return ""
	}
	keys := make([]string, len(issues))
	for i, issue := range issues {
		keys[i] = issue.Key
	}
	jql := fmt.Sprintf("key in (%s) order by assignee ASC", strings.Join(keys, ", "))
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
		if cfg.Since != nil {
			timestamp := issue.Updated
			if timestamp == "" || timestamp == "N/A" {
				continue
			}
			updateDate, err := ParseJiraDate(timestamp)
			if err != nil {
				logWarning("Could not parse date '%s': %v", timestamp, err)
				continue
			}
			if updateDate.Before(*cfg.Since) {
				continue
			}
		}

		// Exclude issues with a comment since the "no comment since" date
		if cfg.NoCommentSince != nil && issue.Comment.Created != "" {
			commentDate, err := ParseJiraDate(issue.Comment.Created)
			if err == nil && commentDate.After(*cfg.NoCommentSince) {
				continue
			}
		}

		filteredIssues = append(filteredIssues, issue)
	}
	logInfo("Filtered %d issues", len(issues)-len(filteredIssues))

	// Sort issues
	sort.Slice(filteredIssues, func(i, j int) bool {
		// By status priority
		pi := GetStatusPriority(filteredIssues[i].StatusName)
		pj := GetStatusPriority(filteredIssues[j].StatusName)
		if pi != pj {
			return pi < pj
		}

		// By target end
		ti := filteredIssues[i].TargetEnd
		tj := filteredIssues[j].TargetEnd
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

// statusPriority maps status name to sort priority (O(1) lookup)
var statusPriority = func() map[string]int {
	m := make(map[string]int, len(statusOrder))
	for i, s := range statusOrder {
		m[s] = i
	}
	return m
}()

// GetStatusPriority returns the sort priority for a status
func GetStatusPriority(statusName string) int {
	status := strings.ToLower(strings.TrimSpace(statusName))
	if p, ok := statusPriority[status]; ok {
		return p
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
