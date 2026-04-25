package jira

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zachariahcox/snippets/internal/logging"
)

// Run executes the jira subcommand. args should be everything after `snippets jira` on the command line.
func Run(args []string) int {
	fs := flag.NewFlagSet("jira", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	jqlQuery := fs.String("jql", "", "JQL query to fetch issues (alternative to specifying keys)")
	sinceStr := fs.String("since", "", "Only include issues updated on or after: YYYY-MM-DD, or N (days ago, e.g. 14)")
	needsUpdate := fs.Int("needs-update", 0, "Exclude issues with a comment in the past N days (0=disabled)")
	title := fs.String("title", "", "Custom title for the report")
	outputFile := fs.String("output-file", "", "Write/append the markdown report to this file")
	outputFileShort := fs.String("o", "", "Write/append the markdown report to this file (short)")
	individual := fs.Bool("individual", false, "Generate a separate report section for each issue")
	individualShort := fs.Bool("i", false, "Generate a separate report section for each issue (short)")
	useStdin := fs.Bool("stdin", false, "Read issue keys from stdin (one per line)")
	useStdinShort := fs.Bool("s", false, "Read issue keys from stdin (short)")
	verbose := fs.Bool("verbose", false, "Enable verbose debug logging")
	verboseShort := fs.Bool("v", false, "Enable verbose debug logging (short)")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")
	csvOutput := fs.Bool("csv", false, "Output in CSV format ('cat separated value': 🐱)")
	slackOutput := fs.Bool("slack", false, "Output as Slack-formatted numbered list")
	urlOutput := fs.Bool("url", false, "Output a single Jira issues URL with filtered keys as JQL")
	simpleOutput := fs.Bool("simple", false, "Output simple text: emoji status key summary (no URLs)")
	summaryOutput := fs.Bool("summary", false, "Output markdown: counts and percents by status (filtered list)")
	children := fs.Bool("children", false, "Fetch child/linked issues and use them when computing trending")
	jiraConcurrency := fs.Int("jira-concurrency", 0, "Max parallel Jira API requests (0=use JIRA_CONCURRENCY env or 8)")
	dueDateFieldFlag := fs.String("due-date-field", "", "Jira custom field display name for due/due date (overrides JIRA_DUE_DATE_FIELD; empty = native Due Date)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: snippets jira [options] <issue_keys...>

Generate a status report for Jira issues (and optional subtasks/linked issues)

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environment variables:
  JIRA_SERVER     - Jira server URL (required)
  JIRA_API_TOKEN  - API token or Personal Access Token (required)
  JIRA_EMAIL      - Your email/username (required for Cloud, optional for Server)
  JIRA_CONCURRENCY - Optional max parallel API calls (default 8; overridden by --jira-concurrency)
  JIRA_DUE_DATE_FIELD - Optional custom field display name for due date (empty = native Due Date)
  JIRA_TRENDING_STATUS_FIELD - Optional custom field; when set, non-empty values override computed trending

Examples:
  snippets jira PROJECT-123 PROJECT-456
  snippets jira --jql "project = MYPROJ AND status != Done"
  snippets jira --since 2025-01-01 --children PROJECT-123
  snippets jira --title "Weekly Status" PROJECT-123 PROJECT-456
`)
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fs.Usage()
		return 2
	}

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

	if *verbose {
		logging.SetLevel(logging.LevelDebug)
	}

	issueKeys := fs.Args()

	if *useStdin {
		logging.Info("Reading issue keys from stdin...")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			key := strings.TrimSpace(scanner.Text())
			if key != "" {
				issueKeys = append(issueKeys, key)
			}
		}
	}

	if len(issueKeys) == 0 && *jqlQuery == "" {
		fs.Usage()
		logging.Error("\nNo issue keys or JQL query provided.")
		return 1
	}

	if *jqlQuery != "" {
		logging.Info("Running JQL query: %s", *jqlQuery)
	} else {
		logging.Info("Processing %d issues...", len(issueKeys))
	}

	var since *time.Time
	if *sinceStr != "" {
		t, err := ParseSince(*sinceStr, time.Now().UTC())
		if err != nil {
			logging.Error("%v", err)
			return 1
		}
		since = t
		if since != nil {
			logging.Info("since filter: include issues updated after %s", since.Format("2006-01-02"))
		}
	}

	var noCommentAfter *time.Time
	if *needsUpdate > 0 {
		t := time.Now().UTC().AddDate(0, 0, -*needsUpdate)
		noCommentAfter = &t
		logging.Info("needs-update filter: include issues with a comment after %s", noCommentAfter)
	}

	if *outputFile != "" {
		if _, err := os.Stat(*outputFile); err == nil {
			if err := os.Remove(*outputFile); err != nil {
				logging.Warning("Could not remove existing file %s: %v", *outputFile, err)
			} else {
				logging.Info("Removed existing file: %s", *outputFile)
			}
		}
	}

	if *title == "" {
		*title = "Snippets!"
	}

	dueFromEnv, trendFromEnv := LoadJiraCustomFieldNames("")
	dueDateFieldName := strings.TrimSpace(*dueDateFieldFlag)
	if dueDateFieldName == "" {
		dueDateFieldName = dueFromEnv
	}

	cfg := &ReportConfig{
		Title:                   *title,
		UpdatedAfter:            since,
		NoCommentAfter:          noCommentAfter,
		OutputFile:              *outputFile,
		JSONOutput:              *jsonOutput,
		CSVOutput:               *csvOutput,
		SlackOutput:             *slackOutput,
		URLOutput:               *urlOutput,
		SimpleOutput:            *simpleOutput,
		SummaryOutput:           *summaryOutput,
		JQLQuery:                *jqlQuery,
		IncludeChildren:         *children,
		DueDateFieldName:        dueDateFieldName,
		TrendingStatusFieldName: trendFromEnv,
	}

	if !*individual {
		parentIssues, err := FetchReportIssues(nil, issueKeys, cfg)
		if err == nil {
			RenderReport(parentIssues, cfg)
			return 0
		}
		if err != ErrCacheMiss {
			logging.Error("%v", err)
			return 1
		}
	}

	server, apiToken, email, err := LoadJiraCreds("")
	if err != nil {
		logging.Error("%v", err)
		return 1
	}
	if email == "" {
		logging.Debug("JIRA_EMAIL is not set. Set the env var or export it from ~/.snippets/creds.sh for Cloud.")
	}
	var client *JiraClient
	client, err = NewJiraClient(server, apiToken, email)
	if err != nil {
		logging.Error("%v", err)
		return 1
	}
	client.MaxConcurrent = ResolveJiraConcurrency(*jiraConcurrency, os.Getenv("JIRA_CONCURRENCY"))
	logging.Debug("Jira max concurrent requests: %d", client.concurrencyCap())

	if *individual {
		for _, issueKey := range issueKeys {
			parentIssues, err := FetchReportIssues(client, []string{issueKey}, cfg)
			if err != nil {
				logging.Error("%v", err)
				return 1
			}
			RenderReport(parentIssues, cfg)
		}
	} else {
		parentIssues, err := FetchReportIssues(client, issueKeys, cfg)
		if err != nil {
			logging.Error("%v", err)
			return 1
		}
		RenderReport(parentIssues, cfg)
	}
	return 0
}

// PrintUsage writes jira command help to w.
func PrintUsage(w interface{ Write([]byte) (int, error) }) {
	fmt.Fprint(w, `Usage: snippets jira [options] <issue_keys...>

Generate a status report for Jira issues. Run snippets jira -h for full flag list.
`)
}
