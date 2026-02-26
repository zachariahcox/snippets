// Generate status reports for Jira issues (optionally including subtasks and linked issues) using the Jira REST API.
//
// Features:
//   - Fetch issues by JQL query or direct issue keys.
//   - Get subtasks and/or linked issues for each parent.
//   - Derive status from Jira's native status field with emoji decoration.
//   - Include target date and last update timestamps.
//   - Filter issues by a minimum last-update date.
//   - Emit a combined report for multiple issues or individual reports per issue.
//   - Output to stdout or append/write to a specified markdown file.
//   - Supports both Jira Cloud and Jira Server/Data Center.
//
// Configuration:
//
//	Environment variables (required):
//	  JIRA_SERVER      - Jira server URL (e.g., https://mycompany.atlassian.net or https://jira.company.com)
//	  JIRA_API_TOKEN   - Your API token or Personal Access Token (PAT)
//
//	For Jira Cloud:
//	  JIRA_EMAIL       - Your Atlassian account email (required for Cloud)
//
//	For Jira Server/Data Center:
//	  JIRA_EMAIL       - Optional (your username, not email)
//
// Usage:
//
//	snippets [options] <issue_keys_or_jql>
//
// Examples:
//
//	snippets --include-subtasks --since 2025-01-01 PROJECT-123 PROJECT-456
//	snippets --jql "project = MYPROJ AND status != Done" --output-file status.md
//	cat issues.txt | snippets --stdin --include-subtasks --include-parent -o aggregated.md
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Default configuration values
const defaultPageSize = 50

// Version and BuildDate are set via ldflags when building with make
var (
	Version   = "dev"
	BuildDate = "unknown"
)

// Status categories mapped to emojis
var statusCategories = map[string]string{
	"done":           "🟣",
	"closed":         "🟣",
	"resolved":       "🟣",
	"in progress":    "🟢",
	"at risk":        "🟡",
	"off track":      "🔴",
	"blocked":        "🔴",
	"not started":    "⚪",
	"ready for work": "⚪",
	"vetting":        "⚪",
	"new":            "⚪",
}

// statusOrder defines the sort priority for statuses
var statusOrder = []string{
	"done",
	"closed",
	"resolved",
	"in progress",
	"at risk",
	"off track",
	"blocked",
	"not started",
	"ready for work",
	"vetting",
	"new",
}

// statusPriority maps status name to sort priority (O(1) lookup)
var statusPriority = func() map[string]int {
	m := make(map[string]int, len(statusOrder))
	for i, s := range statusOrder {
		m[s] = i
	}
	return m
}()

// Custom fields to resolve by name
var customFields = map[string]string{
	"Target end": "",
}

// IssueData represents extracted issue data
type IssueComment struct {
	Url     string
	Created string
}
type IssueData struct {
	Key           string
	URL           string
	Summary       string
	StatusName    string
	Assignee      string
	Priority      string
	Created       string
	Updated       string
	TargetEnd     string
	ParentKey     string
	ParentSummary string
	ParentURL     string
	Trending      string
	Emoji         string
	Comment       IssueComment
}

// ExtractIssueData extracts relevant data from a Jira issue API response
func ExtractIssueData(issue map[string]any, serverURL string, parentKey, parentSummary string) *IssueData {
	fields := getMap(issue, "fields")
	issueKey := getString(issue, "key")

	// Get status
	statusObj := getMap(fields, "status")
	statusName := getString(statusObj, "name")
	if statusName == "" {
		statusName = "Unknown"
	}
	statusName = strings.ToLower(strings.TrimSpace(statusName))

	// Get assignee
	assigneeObj := getMap(fields, "assignee")
	assignee := getString(assigneeObj, "displayName")
	if assignee == "" {
		assignee = "Unassigned"
	}

	// Get priority
	priorityObj := getMap(fields, "priority")
	priority := getString(priorityObj, "name")
	if priority == "" {
		priority = "None"
	}

	// Get dates
	created := getString(fields, "created")
	updated := getString(fields, "updated")

	// Get target end from custom field
	targetEnd := ""
	if customFields["Target end"] != "" {
		targetEnd = getString(fields, customFields["Target end"])
	}

	// Get summary
	summary := getString(fields, "summary")

	// Build issue URL
	issueURL := fmt.Sprintf("%s/browse/%s", serverURL, issueKey)

	// Get trending
	trending := "on track"
	switch statusName {
	case "done", "closed", "resolved":
		trending = "done"
	case "not started", "ready for work", "vetting", "new":
		trending = "not started"
	default:
		trending = statusName
	}
	emoji := GetStatusEmoji(statusName)

	// override with due date info no matter what status
	if IsOverdue(statusName, targetEnd) {
		emoji = "🔴"
		trending = "overdue"
	}

	// Handle parent info
	if parentKey == "" {
		parentKey = issueKey
	}
	if parentSummary == "" {
		parentSummary = summary
	}
	parentURL := issueURL
	if parentKey != issueKey {
		parentURL = fmt.Sprintf("%s/browse/%s", serverURL, parentKey)
	}

	return &IssueData{
		Key:           issueKey,
		URL:           issueURL,
		Summary:       summary,
		StatusName:    statusName,
		Assignee:      assignee,
		Priority:      priority,
		Created:       created,
		Updated:       updated,
		TargetEnd:     targetEnd,
		ParentKey:     parentKey,
		ParentSummary: parentSummary,
		ParentURL:     parentURL,
		Trending:      trending,
		Emoji:         emoji,
	}
}

// GetIssueDetails fetches issue details from Jira
func GetIssue(client *JiraClient, issueKey, parentKey, parentSummary string) (*IssueData, error) {
	logInfo("Fetching one issue: %s", issueKey)
	issue, err := client.GetIssue(issueKey)
	if err != nil {
		logError("  - Failed to fetch issue %s: %v", issueKey, err)
		return nil, err
	}
	data := ExtractIssueData(issue, client.Server, parentKey, parentSummary)
	logInfo("  - Fetched one issue: %s", issueKey)
	return data, nil
}

func GetIssuesFromQuery(client *JiraClient, jqlQuery string) ([]*IssueData, error) {
	logInfo("Executing JQL query: %s", jqlQuery)

	issues := []*IssueData{} // we don't know how many there will be

	jsonBlobs, err := client.SearchIssues(jqlQuery, 1000)
	if err != nil {
		logError("JQL query failed: %v", err)
		return nil, err
	}

	for _, issueJsonBlob := range jsonBlobs {
		issueData := ExtractIssueData(issueJsonBlob, client.Server, "", "")
		issues = append(issues, issueData)
	}

	logInfo("Found %d issues from JQL query", len(issues))
	return issues, nil
}

// GetSubtasks fetches subtasks for a parent issue
func GetSubtasks(client *JiraClient, parentKey, parentSummary string) []*IssueData {
	var subtasks []*IssueData

	parentIssue, err := client.GetIssue(parentKey)
	if err != nil {
		logError("Failed to get subtasks for %s: %v", parentKey, err)
		return subtasks
	}

	fields := getMap(parentIssue, "fields")
	if parentSummary == "" {
		parentSummary = getString(fields, "summary")
	}

	subtaskRefs := getMapList(fields, "subtasks")
	for _, ref := range subtaskRefs {
		subtaskKey := getString(ref, "key")
		if subtaskKey != "" {
			data, err := GetIssue(client, subtaskKey, parentKey, parentSummary)
			if err == nil && data != nil {
				subtasks = append(subtasks, data)
			}
		}
	}

	logInfo("  Found %d subtasks for %s", len(subtasks), parentKey)
	return subtasks
}

// GetLinkedIssues fetches linked issues for a parent issue
func GetLinkedIssues(client *JiraClient, parentKey, parentSummary string) []*IssueData {
	var linked []*IssueData

	parentIssue, err := client.GetIssue(parentKey)
	if err != nil {
		logError("Failed to get linked issues for %s: %v", parentKey, err)
		return linked
	}

	fields := getMap(parentIssue, "fields")
	if parentSummary == "" {
		parentSummary = getString(fields, "summary")
	}

	issueLinks := getMapList(fields, "issuelinks")
	for _, link := range issueLinks {
		linkedIssue := getMap(link, "outwardIssue")
		if linkedIssue == nil {
			linkedIssue = getMap(link, "inwardIssue")
		}
		if linkedIssue != nil {
			linkedKey := getString(linkedIssue, "key")
			if linkedKey != "" {
				data, err := GetIssue(client, linkedKey, parentKey, parentSummary)
				if err == nil && data != nil {
					linked = append(linked, data)
				}
			}
		}
	}

	logInfo("  Found %d linked issues for %s", len(linked), parentKey)
	return linked
}

// GetStatusEmoji returns the emoji for a status name
func GetStatusEmoji(statusName string) string {
	status := strings.ToLower(strings.TrimSpace(statusName))
	if emoji, ok := statusCategories[status]; ok {
		return emoji
	}
	return "❓"
}

// ParseJiraDate parses a Jira date string
func ParseJiraDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}

	// Try various formats
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

	// Try regex fix for +0000 format
	re := regexp.MustCompile(`(\d{2})(\d{2})$`)
	fixed := re.ReplaceAllString(dateStr, "$1:$2")
	if t, err := time.Parse(time.RFC3339, fixed); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("could not parse date: %s", dateStr)
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

// IsOverdue returns true if the issue is past its target date and not done
func IsOverdue(statusName string, targetEnd string) bool {
	status := strings.ToLower(strings.TrimSpace(statusName))
	if status == "done" || status == "resolved" {
		return false
	}

	if targetEnd == "" || targetEnd == "None" {
		return false
	}

	now := time.Now().UTC()

	// Check if it's a date-only string (no 'T')
	if !strings.Contains(targetEnd, "T") {
		dueDate, err := time.Parse("2006-01-02", targetEnd)
		if err != nil {
			return false
		}
		return now.Truncate(24 * time.Hour).After(dueDate)
	}

	dueTime, err := ParseJiraDate(targetEnd)
	if err != nil {
		return false
	}

	return now.After(dueTime.UTC())
}

// GetStatusPriority returns the sort priority for a status
func GetStatusPriority(statusName string) int {
	status := strings.ToLower(strings.TrimSpace(statusName))
	if p, ok := statusPriority[status]; ok {
		return p
	}
	return 999
}

func filterAndSortIssues(issues []*IssueData, since *time.Time) []*IssueData {
	// Filter issues
	var filteredIssues []*IssueData
	for _, issue := range issues {
		if since != nil {
			timestamp := issue.Updated
			if timestamp == "" || timestamp == "N/A" {
				continue
			}
			updateDate, err := ParseJiraDate(timestamp)
			if err != nil {
				logWarning("Could not parse date '%s': %v", timestamp, err)
				continue
			}
			if updateDate.Before(*since) {
				continue
			}
		}
		filteredIssues = append(filteredIssues, issue)
	}

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

// RenderMarkdownReport renders issues as a markdown report
func RenderMarkdownReport(issues []*IssueData, showParent bool, since *time.Time, title string) string {
	// filter and sort issues
	issues = filterAndSortIssues(issues, since)

	var result []string

	if title == "" {
		title = "Jira Status Report"
	}
	result = append(result, fmt.Sprintf("\n### %s, %s", title, time.Now().Format("2006-01-02")))

	if showParent {
		result = append(result, "\n| status | parent | issue | assignee | target date | last update |")
		result = append(result, "|---|:--|:--|:--|:--|:--|")
	} else {
		result = append(result, "\n| status | issue | assignee | target date | last update |")
		result = append(result, "|---|:--|:--|:--|:--|")
	}

	// Render rows
	for _, issue := range issues {
		issueLink := fmt.Sprintf("[%s](%s)", issue.Summary, issue.URL)
		statusWithEmoji := fmt.Sprintf("%s %s", issue.Emoji, issue.Trending)
		targetEnd := FormatDate(issue.TargetEnd)
		timestampLink := FormatTimestampWithLink(issue.Comment.Created, issue.Comment.Url, false)
		var row string
		if showParent {
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

func RenderJSONReport(issues []*IssueData, showParent bool, since *time.Time, title string) string {
	issues = filterAndSortIssues(issues, since)
	jsonData, err := json.Marshal(issues)
	if err != nil {
		logError("Failed to marshal JSON: %v", err)
		return ""
	}
	return string(jsonData)
}

// GenerateReport generates a report of issues
func GenerateReport(
	client *JiraClient,
	issueKeys []string,
	showParent,
	showSubtasks,
	showLinked bool,
	since *time.Time,
	outputFile string,
	jsonOutput bool,
	jqlQuery string) {

	var parentIssues []*IssueData
	var childIssues []*IssueData

	if jqlQuery != "" {
		issues, err := GetIssuesFromQuery(client, jqlQuery)
		if err != nil {
			logError("JQL query failed: %v", err)
			return
		}
		parentIssues = issues

		// update issue keys
		issueKeys = make([]string, len(parentIssues))
		for i, issue := range parentIssues {
			issueKeys[i] = issue.Key
		}
	} else {
		for _, key := range issueKeys {
			issue, err := GetIssue(client, key, "", "")
			if err != nil {
				logError("Failed to get issue %s: %v", key, err)
				continue
			}
			parentIssues = append(parentIssues, issue)
		}
	}

	// collect all linked issues
	for _, issue := range parentIssues {
		if showSubtasks || showLinked {
			if showSubtasks {
				subtasks := GetSubtasks(client, issue.Key, issue.Summary)
				childIssues = append(childIssues, subtasks...)
			}

			if showLinked {
				linked := GetLinkedIssues(client, issue.Key, issue.Summary)
				childIssues = append(childIssues, linked...)
			}
		}
	}

	// lookup most recent comments for all issues
	mostRecentComments, err := client.GetMostRecentComments(issueKeys)
	if err != nil {
		logError("Failed to get most recent comments: %v", err)
		return
	}
	for _, issue := range parentIssues {
		commentJson := mostRecentComments[issue.Key]
		if commentJson == nil {
			continue
		}
		issue.Comment = IssueComment{
			Url:     getString(commentJson, "self"), // not sure why it's called this!
			Created: getString(commentJson, "updated"),
		}
	}

	// Build custom title if single issue
	customTitle := ""
	if len(issueKeys) == 1 && len(parentIssues) > 0 {
		parentKey := issueKeys[0]
		parentSummary := parentIssues[0].Summary
		parentURL := fmt.Sprintf("%s/browse/%s", client.Server, parentKey)
		customTitle = fmt.Sprintf("[%s: %s](%s)", parentKey, parentSummary, parentURL)
	}

	var outputData string
	if showSubtasks || showLinked {
		if jsonOutput {
			outputData = RenderJSONReport(childIssues, showParent, since, customTitle)
		} else {
			outputData = RenderMarkdownReport(childIssues, showParent, since, customTitle)
		}
	} else {
		if jsonOutput {
			outputData = RenderJSONReport(parentIssues, false, since, customTitle)
		} else {
			outputData = RenderMarkdownReport(parentIssues, false, since, customTitle)
		}
	}

	// Output
	if outputFile != "" {
		f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logError("Error opening file %s: %v", outputFile, err)
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

func main() {
	// Define flags
	jqlQuery := flag.String("jql", "", "JQL query to fetch issues (alternative to specifying keys)")
	includeParent := flag.Bool("include-parent", false, "When showing subtasks/linked, include a parent column")
	includeSubtasks := flag.Bool("include-subtasks", false, "Include subtasks in the report output")
	includeLinked := flag.Bool("include-linked", false, "Include linked issues in the report output")
	sinceStr := flag.String("since", "", "Only include issues updated on or after this date (YYYY-MM-DD)")
	outputFile := flag.String("output-file", "", "Write/append the markdown report to this file")
	outputFileShort := flag.String("o", "", "Write/append the markdown report to this file (short)")
	individual := flag.Bool("individual", false, "Generate a separate report section for each issue")
	individualShort := flag.Bool("i", false, "Generate a separate report section for each issue (short)")
	useStdin := flag.Bool("stdin", false, "Read issue keys from stdin (one per line)")
	useStdinShort := flag.Bool("s", false, "Read issue keys from stdin (short)")
	verbose := flag.Bool("verbose", false, "Enable verbose debug logging")
	verboseShort := flag.Bool("v", false, "Enable verbose debug logging (short)")
	quiet := flag.Bool("quiet", false, "Suppress non-essential output")
	quietShort := flag.Bool("q", false, "Suppress non-essential output (short)")
	jsonOutput := flag.Bool("json", false, "Output in JSON format")
	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: snippets [options] <issue_keys...>

Generate a status report for Jira issues (and optional subtasks/linked issues)

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environment variables:
  JIRA_SERVER     - Jira server URL (required)
  JIRA_API_TOKEN  - API token or Personal Access Token (required)
  JIRA_EMAIL      - Your email/username (required for Cloud, optional for Server)

For Jira Cloud (*.atlassian.net):
  export JIRA_SERVER="https://mycompany.atlassian.net"
  export JIRA_EMAIL="you@company.com"
  export JIRA_API_TOKEN="<token from id.atlassian.com>"

For Jira Server/Data Center:
  export JIRA_SERVER="https://jira.company.com"
  export JIRA_API_TOKEN="<Personal Access Token from Jira profile>"

Examples:
  snippets PROJECT-123 PROJECT-456
  snippets --jql "project = MYPROJ AND status != Done"
  snippets --include-subtasks --since 2025-01-01 PROJECT-123
`)
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("snippets %s (built %s)\n", Version, BuildDate)
		os.Exit(0)
	}

	// Merge short flags
	if *outputFileShort != "" && *outputFile == "" {
		*outputFile = *outputFileShort
	}
	if *individualShort {
		*individual = true
	}
	if *useStdinShort {
		*useStdin = true
	}
	if *verboseShort {
		*verbose = true
	}
	if *quietShort {
		*quiet = true
	}

	// Set log level
	if *verbose {
		logLevel = LogLevelDebug
	} else if *quiet {
		logLevel = LogLevelError
	} else {
		logLevel = LogLevelWarning
	}

	// Collect issue keys
	issueKeys := flag.Args()

	// Read from stdin if requested or if no args and stdin has data
	if *useStdin {
		logInfo("Reading issue keys from stdin...")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			key := strings.TrimSpace(scanner.Text())
			if key != "" {
				issueKeys = append(issueKeys, key)
			}
		}
	}

	// Validate input
	if len(issueKeys) == 0 && *jqlQuery == "" {
		flag.Usage()
		logError("\nNo issue keys or JQL query provided.")
		os.Exit(1)
	}

	logInfo("Processing %d issues...", len(issueKeys))

	// Parse since date
	var since *time.Time
	if *sinceStr != "" {
		t, err := time.Parse("2006-01-02", *sinceStr)
		if err != nil {
			logError("Invalid date format '%s'. Expected YYYY-MM-DD.", *sinceStr)
			os.Exit(1)
		}
		t = t.UTC()
		since = &t
		logInfo("Filtering issues updated after %s", since)
	}

	// Remove existing output file
	if *outputFile != "" {
		if _, err := os.Stat(*outputFile); err == nil {
			if err := os.Remove(*outputFile); err != nil {
				logWarning("Could not remove existing file %s: %v", *outputFile, err)
			} else {
				logInfo("Removed existing file: %s", *outputFile)
			}
		}
	}

	// parse args
	server := os.Getenv("JIRA_SERVER")
	if server == "" {
		logError("JIRA_SERVER environment variable is not set.\nExample: export JIRA_SERVER=https://mycompany.atlassian.net")
		os.Exit(1)
	}
	apiToken := os.Getenv("JIRA_API_TOKEN")
	if apiToken == "" {
		logError("JIRA_API_TOKEN environment variable is not set.\nExample: export JIRA_API_TOKEN=your-token")
		os.Exit(1)
	}
	email := os.Getenv("JIRA_EMAIL")
	if email == "" {
		logError("JIRA_EMAIL environment variable is not set.\nExample: export JIRA_EMAIL=you@company.com")
		os.Exit(1)
	}

	// Connect to Jira
	client, err := GetJiraClient(server, email, apiToken)
	if err != nil {
		logError("%v", err)
		os.Exit(1)
	}

	// Generate report(s)
	if *individual {
		for _, issueKey := range issueKeys {
			GenerateReport(client, []string{issueKey},
				*includeParent,
				*includeSubtasks,
				*includeLinked,
				since,
				*outputFile,
				*jsonOutput,
				*jqlQuery)
		}
	} else {
		GenerateReport(client, issueKeys,
			*includeParent,
			*includeSubtasks,
			*includeLinked,
			since,
			*outputFile,
			*jsonOutput,
			*jqlQuery)
	}
}
