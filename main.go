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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Default configuration values
const defaultPageSize = 50

// credsFileName is the name of the optional shell script that can export JIRA_* vars.
const credsFileName = ".snippets/creds.sh"

// loadJiraCreds returns JIRA_SERVER, JIRA_API_TOKEN, and JIRA_EMAIL from the environment
// and optionally from a creds shell script (sourced in a subprocess). Env vars take precedence.
// credsFilePath: if non-empty, use this path; otherwise use ~/.snippets/creds.sh. Pass "" in production.
// Returns an error if JIRA_SERVER or JIRA_API_TOKEN are missing.
func loadJiraCreds(credsFilePath string) (server, apiToken, email string, err error) {
	server = os.Getenv("JIRA_SERVER")
	apiToken = os.Getenv("JIRA_API_TOKEN")
	email = os.Getenv("JIRA_EMAIL")

	// Resolve creds file path
	var credsPath string
	if credsFilePath != "" {
		credsPath = credsFilePath
	} else {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			credsPath = ""
		} else {
			credsPath = filepath.Join(home, credsFileName)
		}
	}
	if credsPath != "" {
		if _, statErr := os.Stat(credsPath); statErr == nil {
			script := fmt.Sprintf(`. %q 2>/dev/null; echo "JIRA_SERVER=$JIRA_SERVER"; echo "JIRA_API_TOKEN=$JIRA_API_TOKEN"; echo "JIRA_EMAIL=$JIRA_EMAIL"`, credsPath)
			cmd := exec.Command("sh", "-c", script)
			cmd.Env = os.Environ()
			out, cmdErr := cmd.Output()
			if cmdErr == nil {
				for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					i := strings.Index(line, "=")
					if i <= 0 {
						continue
					}
					key, val := line[:i], line[i+1:]
					val = strings.Trim(val, "\"")
					switch key {
					case "JIRA_SERVER":
						if server == "" && val != "" {
							server = val
						}
					case "JIRA_API_TOKEN":
						if apiToken == "" && val != "" {
							apiToken = val
						}
					case "JIRA_EMAIL":
						if email == "" && val != "" {
							email = val
						}
					}
				}
			}
		}
	}

	// Validate required fields
	if server == "" {
		return "", "", "", fmt.Errorf("JIRA_SERVER is not set (set env var or export from ~/.snippets/creds.sh)")
	}
	if apiToken == "" {
		return "", "", "", fmt.Errorf("JIRA_API_TOKEN is not set (set env var or export from ~/.snippets/creds.sh)")
	}
	return server, apiToken, email, nil
}

// Version and BuildDate are set via ldflags when building with make
var (
	Version   = "dev"
	BuildDate = "unknown"
)

// ReportConfig holds options for report generation
type ReportConfig struct {
	Title          string
	ShowChildren   bool
	UpdatedAfter   *time.Time
	NoCommentAfter *time.Time
	OutputFile     string
	JSONOutput     bool
	CSVOutput      bool
	SlackOutput    bool
	URLOutput      bool
	SimpleOutput   bool
	JQLQuery       string
}

func (c *ReportConfig) String() string {
	if c == nil {
		return "nil"
	}
	since := ""
	if c.UpdatedAfter != nil {
		since = c.UpdatedAfter.Format("2006-01-02")
	}
	noComment := ""
	if c.NoCommentAfter != nil {
		noComment = c.NoCommentAfter.Format("2006-01-02")
	}
	return fmt.Sprintf("title=%q jql=%q children=%t since=%q noCommentAfter=%q out=%q json=%t csv=%t slack=%t url=%t simple=%t",
		c.Title, c.JQLQuery, c.ShowChildren, since, noComment, c.OutputFile,
		c.JSONOutput, c.CSVOutput, c.SlackOutput, c.URLOutput, c.SimpleOutput)
}

// ParseSince parses --since: YYYY-MM-DD or numeric days ago (e.g. 14 = now - 14 days).
// now is used for the "days ago" calculation (pass time.Now().UTC() in production).
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

// IsStale returns true if the issue is past its target date and not done
func IsStale(status string, targetEnd string) bool {
	if status == "done" || status == "closed" || status == "resolved" {
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

	targetTime, err := ParseJiraDate(targetEnd)
	if err != nil {
		return false
	}

	return now.After(targetTime.UTC())
}

// IsDueWithinNextMonth returns true if the issue has a target end date within the next calendar month and is not done.
func IsDueWithinNextMonth(targetEnd string) bool {
	if targetEnd == "" || targetEnd == "None" {
		return false
	}
	now := time.Now().UTC()
	var dueDate time.Time
	if !strings.Contains(targetEnd, "T") {
		d, err := time.Parse("2006-01-02", targetEnd)
		if err != nil {
			return false
		}
		dueDate = d.UTC()
	} else {
		t, err := ParseJiraDate(targetEnd)
		if err != nil {
			return false
		}
		dueDate = t.UTC()
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dueDay := time.Date(dueDate.Year(), dueDate.Month(), dueDate.Day(), 0, 0, 0, 0, time.UTC)
	oneMonthFromNow := today.AddDate(0, 1, 0)
	return !dueDay.Before(today) && !dueDay.After(oneMonthFromNow)
}

// FetchReportIssues generates a report of issues. It tries the cache first; on hit it returns
// cached data (client may be nil for cache-only lookup). On cache miss with client == nil it
// returns ErrCacheMiss. On cache miss with client != nil it fetches from Jira, writes the
// cache, and returns the result.
func FetchReportIssues(client *JiraClient, issueKeys []string, cfg *ReportConfig) ([]*IssueData, []*IssueData, error) {
	logInfo("Fetching issues for configuration: %v", cfg)

	key := CacheKey(cfg.JQLQuery, issueKeys, cfg.ShowChildren)
	if err := EnsureCacheDir(); err != nil {
		logWarning("Cache dir unavailable: %v", err)
	} else {
		// Check cache first without pruning (pruning does ReadDir+Stat on every file and can be slow).
		path, err := cachePath(key)
		if err == nil && CacheValid(path, cacheTTL) {
			parentIssues, childIssues, err := ReadCache(path)
			if err == nil {
				logInfo("Using cached results at %s.", path)
				return parentIssues, childIssues, nil
			}
			logDebug("Cache read failed: %v", err)
		}
	}

	if client == nil {
		return nil, nil, ErrCacheMiss
	}

	// Prune only when we're about to fetch (and possibly write); avoids slow ReadDir+Stat on cache-hit path.
	_ = PruneCache(cacheTTL)

	var parentIssues []*IssueData
	if cfg.JQLQuery != "" {
		issues, err := client.GetIssuesFromQuery(cfg.JQLQuery)
		if err != nil {
			logError("JQL query failed: %v", err)
			return nil, nil, err
		}
		parentIssues = issues

		// update issue keys
		issueKeys = make([]string, len(parentIssues))
		for i, issue := range parentIssues {
			issueKeys[i] = issue.Key
		}
	} else {
		for _, key := range issueKeys {
			issue, err := client.GetIssue(key, "", "")
			if err != nil {
				logError("Failed to get issue %s: %v", key, err)
				continue
			}
			parentIssues = append(parentIssues, issue)
		}
	}

	// collect all child issues
	var childIssues []*IssueData
	if cfg.ShowChildren {
		for _, issue := range parentIssues {
			subtasks := client.GetSubtasks(issue.Key, issue.Summary)
			childIssues = append(childIssues, subtasks...)
			linked := client.GetLinkedIssues(issue.Key, issue.Summary)
			childIssues = append(childIssues, linked...)
		}
	}

	// Collect all issue keys we'll display (for comment fetch)
	allKeys := make([]string, 0, len(parentIssues)+len(childIssues))
	for _, p := range parentIssues {
		allKeys = append(allKeys, p.Key)
	}
	for _, c := range childIssues {
		allKeys = append(allKeys, c.Key)
	}

	// Lookup most recent comments for all displayed issues
	mostRecentComments, err := client.GetMostRecentComments(allKeys)
	if err != nil {
		logError("Failed to get most recent comments: %v", err)
	} else {
		for _, s := range [][]*IssueData{parentIssues, childIssues} {
			for _, issue := range s {
				commentJson := mostRecentComments[issue.Key]
				if commentJson == nil {
					continue
				}
				commentId := getString(commentJson, "id", "")
				issue.Comment = IssueComment{
					Url:     fmt.Sprintf("%s?focusedId=%s&page=com.atlassian.jira.plugin.system.issuetabpanels%%3Acomment-tabpanel#comment-%s", issue.URL, commentId, commentId),
					Created: getString(commentJson, "updated", ""),
				}
			}
		}
	}

	// Write cache for next run
	if path, err := cachePath(key); err == nil {
		if wErr := WriteCache(path, parentIssues, childIssues); wErr != nil {
			logWarning("Failed to write cache: %v", wErr)
		} else {
			logDebug("Cached results to %s", path)
		}
	}

	return parentIssues, childIssues, nil
}

func main() {
	// Define flags
	jqlQuery := flag.String("jql", "", "JQL query to fetch issues (alternative to specifying keys)")
	children := flag.Bool("children", false, "Render children of directly referenced issues")
	sinceStr := flag.String("since", "", "Only include issues updated on or after: YYYY-MM-DD, or N (days ago, e.g. 14)")
	needsUpdate := flag.Int("needs-update", 0, "Exclude issues with a comment in the past N days (0=disabled)")
	title := flag.String("title", "", "Custom title for the report")
	outputFile := flag.String("output-file", "", "Write/append the markdown report to this file")
	outputFileShort := flag.String("o", "", "Write/append the markdown report to this file (short)")
	individual := flag.Bool("individual", false, "Generate a separate report section for each issue")
	individualShort := flag.Bool("i", false, "Generate a separate report section for each issue (short)")
	useStdin := flag.Bool("stdin", false, "Read issue keys from stdin (one per line)")
	useStdinShort := flag.Bool("s", false, "Read issue keys from stdin (short)")
	verbose := flag.Bool("verbose", false, "Enable verbose debug logging")
	verboseShort := flag.Bool("v", false, "Enable verbose debug logging (short)")
	jsonOutput := flag.Bool("json", false, "Output in JSON format")
	csvOutput := flag.Bool("csv", false, "Output in CSV format ('cat separated value': 🐱)")
	slackOutput := flag.Bool("slack", false, "Output as Slack-formatted numbered list")
	urlOutput := flag.Bool("url", false, "Output a single Jira issues URL with filtered keys as JQL")
	simpleOutput := flag.Bool("simple", false, "Output simple text: emoji status key summary (no URLs)")
	clearCache := flag.Bool("clear-cache", false, "Clear the cache at ~/.snippets/cache and exit")
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

Examples:
  snippets PROJECT-123 PROJECT-456
  snippets --jql "project = MYPROJ AND status != Done"
  snippets --include-subtasks --since 2025-01-01 PROJECT-123
  snippets --title "Weekly Status" PROJECT-123 PROJECT-456
`)
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("snippets %s (built %s)\n", Version, BuildDate)
		os.Exit(0)
	}

	if *clearCache {
		if err := ClearCache(); err != nil {
			logError("Failed to clear cache: %v", err)
			os.Exit(1)
		}
		logInfo("Cache cleared.")
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

	// Set log level
	if *verbose {
		logLevel = LogLevelDebug
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

	if *jqlQuery != "" {
		logInfo("Running JQL query: %s", *jqlQuery)
	} else {
		logInfo("Processing %d issues...", len(issueKeys))
	}

	// Parse since date: YYYY-MM-DD or numeric days ago (e.g. 14 = now - 14 days)
	var since *time.Time
	if *sinceStr != "" {
		t, err := ParseSince(*sinceStr, time.Now().UTC())
		if err != nil {
			logError("%v", err)
			os.Exit(1)
		}
		since = t
		if since != nil {
			logInfo("since filter: include issues updated after %s", since.Format("2006-01-02"))
		}
	}

	// parse no comment since date
	var noCommentAfter *time.Time
	if *needsUpdate > 0 {
		t := time.Now().UTC().AddDate(0, 0, -*needsUpdate)
		noCommentAfter = &t
		logInfo("needs-update filter: include issues with a comment after %s", noCommentAfter)
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

	if *title == "" {
		*title = "Snippets!"
	}

	cfg := &ReportConfig{
		Title:          *title,
		ShowChildren:   *children,
		UpdatedAfter:   since,
		NoCommentAfter: noCommentAfter,
		OutputFile:     *outputFile,
		JSONOutput:     *jsonOutput,
		CSVOutput:      *csvOutput,
		SlackOutput:    *slackOutput,
		URLOutput:      *urlOutput,
		SimpleOutput:   *simpleOutput,
		JQLQuery:       *jqlQuery,
	}

	// Try cache first when not in individual mode (skip Jira entirely on hit)
	if !*individual {
		parentIssues, childIssues, err := FetchReportIssues(nil, issueKeys, cfg)
		if err == nil {
			RenderReport(parentIssues, childIssues, cfg)
			os.Exit(0)
		}
		if err != ErrCacheMiss {
			logError("%v", err)
			os.Exit(1)
		}
	}

	// Load credentials and connect to Jira
	server, apiToken, email, err := loadJiraCreds("")
	if err != nil {
		logError("%v", err)
		os.Exit(1)
	}
	if email == "" {
		logDebug("JIRA_EMAIL is not set. Set the env var or export it from ~/.snippets/creds.sh for Cloud.")
	}
	var client *JiraClient
	client, err = NewJiraClient(server, apiToken, email)
	if err != nil {
		logError("%v", err)
		os.Exit(1)
	}

	if *individual {
		for _, issueKey := range issueKeys {
			parentIssues, childIssues, err := FetchReportIssues(client, []string{issueKey}, cfg)
			if err != nil {
				logError("%v", err)
				os.Exit(1)
			}
			RenderReport(parentIssues, childIssues, cfg)
		}
	} else {
		parentIssues, childIssues, err := FetchReportIssues(client, issueKeys, cfg)
		if err != nil {
			logError("%v", err)
			os.Exit(1)
		}
		RenderReport(parentIssues, childIssues, cfg)
	}
}
