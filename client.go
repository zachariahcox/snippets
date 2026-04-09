package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// defaultJiraConcurrency is used when MaxConcurrent is unset or invalid.
const defaultJiraConcurrency = 8

// JiraClient is a simple Jira REST API client
type JiraClient struct {
	Server     string
	Email      string
	APIToken   string
	APIVersion string
	IsCloud    bool
	HTTPClient *http.Client

	// MaxConcurrent caps parallel REST calls (issue fetch, child JQL, comment batches). If < 1, defaultJiraConcurrency is used.
	MaxConcurrent int

	muCustomFields     sync.Mutex
	customFieldsLoaded bool
}

// Status categories mapped to emojis
var trendingEmojis = map[string]string{
	"done":        "🟣",
	"on track":    "🟢",
	"at risk":     "🟡",
	"off track":   "🔴",
	"not started": "⚪",
}

// statusEmojis maps status to action/semantic icons (play, check, stop, etc.)
var statusEmojis = map[string]string{
	"closed":         "❎",
	"resolved":       "🎉",
	"in progress":    "🚀",
	"blocked":        "🛑",
	"ready for work": "🪏",
	"vetting":        "🤔",
	"new":            "✨",
}

// statusOrder defines the sort priority for statuses
var statusOrder = []string{
	"closed",
	"resolved",
	"in progress",
	"blocked",
	"ready for work",
	"vetting",
	"new",
}

// Custom fields to resolve by name
var customFields = map[string]string{
	"Target end": "",
}

// IssueData represents extracted issue data
type IssueComment struct {
	Url     string `json:"url"`
	Created string `json:"created"`
}
type IssueData struct {
	Key             string       `json:"key"`
	URL             string       `json:"url"`
	Summary         string       `json:"summary"`
	Status          string       `json:"status"`
	StatusEmoji     string       `json:"status_emoji"`
	Assignee        string       `json:"assignee"`
	Priority        string       `json:"priority"`
	Created         string       `json:"created"`
	Updated         string       `json:"updated"`
	TargetEnd       string       `json:"target_end"`
	Trending        string       `json:"trending"`
	TrendingEmoji   string       `json:"Emoji"` // cache compat: historical key name
	TrendingComment string       `json:"trending_comment"`
	Comment         IssueComment `json:"comment"`
	Type            string       `json:"type"` // initiative, epic, story, subtask, …
	Children        []*IssueData `json:"children"`
}

// NewJiraClient creates a new Jira client
func NewJiraClient(server, apiToken, email string) (*JiraClient, error) {
	if server == "" || apiToken == "" {
		return nil, fmt.Errorf("failed to connect to Jira. Check your credentials and server URL.\nFor Jira Server/Data Center, ensure you're using a valid Personal Access Token (PAT)")
	}
	server = strings.TrimRight(server, "/")
	isCloud := strings.Contains(strings.ToLower(server), ".atlassian.net")

	apiVersion := "2"
	if isCloud {
		if email == "" {
			return nil, fmt.Errorf("JIRA_EMAIL is required for Jira Cloud authentication")
		}
		apiVersion = "3"
		logDebug("Using Jira Cloud authentication (API v%s)", apiVersion)
	} else {
		logDebug("Using Jira on-prem authentication (API v%s)", apiVersion)
	}

	client := &JiraClient{
		Server:     server,
		Email:      email,
		APIToken:   apiToken,
		APIVersion: apiVersion,
		IsCloud:    isCloud,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}

	if !client.TestConnection() {
		return nil, fmt.Errorf("failed to connect to Jira. Check your credentials and server URL.\nFor Jira Server/Data Center, ensure you're using a valid Personal Access Token (PAT)")
	}

	logDebug("Connected to Jira server: %s", server)
	return client, nil
}

func (c *JiraClient) concurrencyCap() int {
	if c.MaxConcurrent < 1 {
		return defaultJiraConcurrency
	}
	return c.MaxConcurrent
}

// ensureCustomFieldsLoaded resolves custom field IDs once; safe for concurrent FetchIssuesFromQuery/searchIssues.
func (c *JiraClient) ensureCustomFieldsLoaded() {
	c.muCustomFields.Lock()
	defer c.muCustomFields.Unlock()
	if c.customFieldsLoaded {
		return
	}
	if err := c.loadCustomFields(customFields); err != nil {
		logWarning("Could not load custom fields: %v", err)
	}
	c.customFieldsLoaded = true
}

// computeTrending computes the trending status for an issue and its children
func computeTrending(issue *IssueData) {
	if issue.Trending != "" {
		return // already computed
	}
	if issue.Status == "unknown" {
		issue.Trending = "unknown"
		issue.TrendingEmoji = "❓"
		return
	}

	// approximate trending based on status
	trending := "not started" // default to not started

	switch issue.Status {
	case "closed", "resolved", "done":
		trending = "done"
		issue.TrendingComment = "🎉"
	case "blocked":
		trending = "off track"
	case "not started", "new", "vetting", "ready for work":
		trending = "not started"
		if isDueWithinDays(issue.TargetEnd, 30) {
			trending = "at risk"
			issue.TrendingComment = fmt.Sprintf("due within %d days but not started.", 30)
		}
	case "in progress":
		trending = "on track"
	default:
		trending = "INCONCLUSIVE"
	}

	// past target end -> off track (unless already done)
	if trending != "done" && isStale(issue.TargetEnd) {
		trending = "off track"
	}

	// consider the children
	if len(issue.Children) > 0 {
		if trending != "done" && trending != "blocked" && trending != "at risk" && trending != "off track" {
			for _, child := range issue.Children {
				// what's their status? (recursive)
				computeTrending(child)

				// if children are in any of the following statuses, set trending to that status
				for _, t := range []string{"blocked", "off track", "at risk"} {
					if child.Trending == t {
						trending = t
						issue.TrendingComment = fmt.Sprintf("child %s is '%s'", child.Key, t)
						break
					}
				}
			}

			// if all children are done, set trending to done
			if trending != "done" {
				allDone := true
				for _, child := range issue.Children {
					computeTrending(child)
					if child.Trending != "done" {
						allDone = false
						break
					}
				}
				if allDone {
					trending = "done"
					issue.TrendingComment = "All children are done. What's left?"
				}
			}
		}
	}

	// get emoji
	trendingEmoji := "❓"
	if value, ok := trendingEmojis[trending]; ok {
		trendingEmoji = value
	}

	// save values
	issue.Trending = trending
	issue.TrendingEmoji = trendingEmoji
}

// extractIssueData extracts relevant data from a Jira issue API response.
// Parent/child relationships are represented only via IssueData.Children after loadChildren.
func extractIssueData(issue map[string]any, serverURL string) *IssueData {
	fields := getMap(issue, "fields")
	issueKey := getString(issue, "key", "")

	// Get type
	typeObj := getMap(fields, "issuetype")
	typeRaw := getString(typeObj, "name", "Unknown")
	typeNormalized := strings.ToLower(strings.TrimSpace(typeRaw))

	// Get status
	statusObj := getMap(fields, "status")
	statusRaw := getString(statusObj, "name", "Unknown")
	statusNormalized := strings.ToLower(strings.TrimSpace(statusRaw))
	statusEmoji := "❓"
	if value, ok := statusEmojis[statusNormalized]; ok {
		statusEmoji = value
	}

	// Get assignee
	assigneeObj := getMap(fields, "assignee")
	assignee := getString(assigneeObj, "displayName", "N/A")

	// Get priority
	priorityObj := getMap(fields, "priority")
	priority := getString(priorityObj, "name", "None")

	// Get dates
	created := getString(fields, "created", "")
	updated := getString(fields, "updated", "")

	// Get target end from custom field
	targetEnd := getString(fields, customFields["Target end"], "")

	// Get summary
	summary := getString(fields, "summary", "")

	// Build issue URL
	issueURL := fmt.Sprintf("%s/browse/%s", serverURL, issueKey)

	return &IssueData{
		Type:        typeNormalized,
		Key:         issueKey,
		URL:         issueURL,
		Summary:     summary,
		Status:      statusNormalized,
		Assignee:    assignee,
		Priority:    priority,
		Created:     created,
		Updated:     updated,
		TargetEnd:   targetEnd,
		StatusEmoji: statusEmoji,
	}
}

// FetchIssue fetches issue details from Jira
func (client *JiraClient) FetchIssue(issueKey string) (*IssueData, error) {
	issues, err := client.FetchIssuesByKeys([]string{issueKey})
	if err != nil {
		return nil, err
	}
	return issues[0], nil
}

func (client *JiraClient) FetchIssuesFromQuery(jqlQuery string) ([]*IssueData, error) {
	logInfo("Executing JQL query: %s", jqlQuery)

	issues := []*IssueData{} // we don't know how many there will be

	jsonBlobs, err := client.searchIssues(jqlQuery, 1000)
	if err != nil {
		logError("JQL query failed: %v", err)
		return nil, err
	}

	for _, issueJsonBlob := range jsonBlobs {
		issueData := extractIssueData(issueJsonBlob, client.Server)
		issues = append(issues, issueData)
	}

	logInfo("Found %d issues from JQL query", len(issues))
	return issues, nil
}

// FetchIssuesByKeys loads issues in parallel (bounded by MaxConcurrent), preserving input key order; failed keys are skipped.
func (client *JiraClient) FetchIssuesByKeys(issueKeys []string) ([]*IssueData, error) {
	if len(issueKeys) == 0 {
		return nil, nil
	}

	// for batches of 50, generate a jql query to fetch the issues
	issues := []*IssueData{}
	for i := 0; i < len(issueKeys); i += 50 {
		end := i + 50
		if end > len(issueKeys) {
			end = len(issueKeys)
		}
		batch := issueKeys[i:end]
		jql := fmt.Sprintf("key in (%s)", strings.Join(batch, ","))

		// fetch the issues
		result, err := client.FetchIssuesFromQuery(jql)
		if err != nil {
			logError("Failed to fetch issues: %v", err)
			return nil, err
		}
		issues = append(issues, result...)
	}
	return issues, nil
}

func (client *JiraClient) loadChildren(parents []*IssueData) {
	lim := client.concurrencyCap()
	var wg sync.WaitGroup
	sem := make(chan struct{}, lim)
	for _, parent := range parents {
		if parent == nil {
			continue
		}
		wg.Add(1)
		go func(p *IssueData) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			jql := fmt.Sprintf(`issue in linkedIssues(%s, "is parent of") or issue in childIssuesOf(%s)`, p.Key, p.Key)
			logInfo("Loading children for %s: %s", p.Key, jql)
			jsonBlobs, err := client.searchIssues(jql, 1000)
			if err != nil {
				logError("Failed to load children for %s: %v", p.Key, err)
				p.Children = nil
				return
			}
			var children []*IssueData
			for _, blob := range jsonBlobs {
				children = append(children, extractIssueData(blob, client.Server))
			}
			p.Children = children
			logInfo("  Found %d children for %s", len(children), p.Key)
		}(parent)
	}
	wg.Wait()
}

// resolves custom field names to IDs
func (c *JiraClient) loadCustomFields(fieldNames map[string]string) error {
	fields, err := c.getJsonList("field", nil)
	if err != nil {
		return err
	}

	for name := range fieldNames {
		for _, f := range fields {
			if getString(f, "name", "") == name {
				fieldNames[name] = getString(f, "id", "")
				break
			}
		}
	}

	return nil
}

// searchIssues searches for issues using JQL with pagination
func (c *JiraClient) searchIssues(jql string, maxResults int) ([]map[string]any, error) {
	var b strings.Builder
	b.WriteString("summary,status,issuetype,assignee,priority,created,updated")

	c.ensureCustomFieldsLoaded()

	// Add custom field IDs
	for _, id := range customFields {
		if id != "" {
			b.WriteString(",")
			b.WriteString(id)
		}
	}
	fields := b.String()

	var allIssues []map[string]any
	startAt := 0
	pageSize := min(defaultPageSize, maxResults)

	for {
		params := map[string]string{
			"jql":        jql,
			"fields":     fields,
			"startAt":    fmt.Sprintf("%d", startAt),
			"maxResults": fmt.Sprintf("%d", pageSize),
		}

		logDebug("Fetching issues: startAt=%d, maxResults=%d", startAt, pageSize)
		response, err := c.getJson("search", params)
		if err != nil {
			return nil, err
		}

		issues := getMapList(response, "issues")
		total := getInt(response, "total")

		allIssues = append(allIssues, issues...)
		logDebug("Fetched %d issues (total so far: %d, server total: %d)", len(issues), len(allIssues), total)

		if len(allIssues) >= total || len(allIssues) >= maxResults {
			break
		}

		if len(issues) < pageSize {
			break
		}

		startAt += pageSize
		remaining := maxResults - len(allIssues)
		pageSize = min(defaultPageSize, remaining)
	}

	logInfo("Fetched %d issues total", len(allIssues))
	if len(allIssues) > maxResults {
		return allIssues[:maxResults], nil
	}
	return allIssues, nil
}

// GetComments fetches all comments for an issue
func (c *JiraClient) GetComments(issueKey string) ([]map[string]any, error) {
	resp, err := c.getJson(fmt.Sprintf("issue/%s/comment", issueKey), nil)
	if err != nil {
		return nil, err
	}
	return getMapList(resp, "comments"), nil
}

const commentBatchSize = 50

// GetMostRecentComments returns a map of issue key to the most recent comment (as a JSON blob).
// Issues with no comments are omitted from the result.
// Fetches in batches of at most commentBatchSize issue keys.
func (c *JiraClient) loadComments(issues []*IssueData) error {
	issueCount := len(issues)
	result := make(map[string]map[string]any, issueCount)
	lim := c.concurrencyCap()
	var (
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	sem := make(chan struct{}, lim)
	for i := 0; i < issueCount; i += commentBatchSize {
		end := i + commentBatchSize
		if end > issueCount {
			end = issueCount
		}
		batch := append([]*IssueData(nil), issues[i:end]...)
		wg.Add(1)
		go func(batch []*IssueData) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			batchResult, err := c.getMostRecentCommentsBulk(batch)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			for k, comment := range batchResult {
				result[k] = comment
			}
			mu.Unlock()
		}(batch)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}

	// map comments into issues
	for _, issue := range issues {
		commentJson := result[issue.Key]
		if commentJson == nil {
			continue
		}
		commentId := getString(commentJson, "id", "")
		issue.Comment = IssueComment{
			Url:     fmt.Sprintf("%s?focusedId=%s&page=com.atlassian.jira.plugin.system.issuetabpanels%%3Acomment-tabpanel#comment-%s", issue.URL, commentId, commentId),
			Created: getString(commentJson, "updated", ""),
		}
	}
	return nil
}

// getMostRecentCommentsBulk fetches issues via search API with comment field (one request).
func (c *JiraClient) getMostRecentCommentsBulk(issues []*IssueData) (map[string]map[string]any, error) {
	issueCount := len(issues)
	result := make(map[string]map[string]any, issueCount)

	// Build JQL: key in (A, B, C)
	quoted := make([]string, issueCount)
	for i, k := range issues {
		quoted[i] = fmt.Sprintf("%q", k.Key)
	}
	jql := "key in (" + strings.Join(quoted, ",") + ")"

	params := map[string]string{
		"jql":        jql,
		"fields":     "comment",
		"maxResults": fmt.Sprintf("%d", issueCount),
	}

	response, err := c.getJson("search", params)
	if err != nil {
		return nil, err
	}

	responseIssues := getMapList(response, "issues")
	for _, issue := range responseIssues {
		key := getString(issue, "key", "")
		fields := getMap(issue, "fields")
		commentObj := getMap(fields, "comment")
		comments := getMapList(commentObj, "comments")
		if latest := findLatestComment(comments); latest != nil {
			result[key] = latest
		}
	}

	return result, nil
}

func findLatestComment(comments []map[string]any) map[string]any {
	if len(comments) == 0 {
		return nil
	}
	latest := comments[0]
	latestCreated := getString(latest, "created", "")
	for i := 1; i < len(comments); i++ {
		created := getString(comments[i], "created", "")
		if created > latestCreated {
			latest = comments[i]
			latestCreated = created
		}
	}
	return latest
}

// TestConnection tests the connection to Jira
func (c *JiraClient) TestConnection() bool {
	_, err := c.getJson("myself", nil)
	if err != nil {
		logError("Connection test failed: %v", err)
		return false
	}
	return true
}

// doRequest makes an authenticated request to the Jira API
func (c *JiraClient) doRequest(method, endpoint string, params map[string]string) ([]byte, error) {
	baseURL := fmt.Sprintf("%s/rest/api/%s/%s", c.Server, c.APIVersion, strings.TrimLeft(endpoint, "/"))

	// Add query params
	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		baseURL += "?" + values.Encode()
	}

	logDebug("Request: %s %s", method, baseURL)

	req, err := http.NewRequest(method, baseURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	if c.IsCloud {
		// Basic auth with email:token
		auth := base64.StdEncoding.EncodeToString([]byte(c.Email + ":" + c.APIToken))
		req.Header.Set("Authorization", "Basic "+auth)
	} else {
		// Bearer token (PAT)
		req.Header.Set("Authorization", "Bearer "+c.APIToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	logDebug("Response: %d", resp.StatusCode)

	if resp.StatusCode >= 400 {
		logError("API error: %d - %s", resp.StatusCode, truncate(string(body), 500))
		return nil, fmt.Errorf("API error: %d", resp.StatusCode)
	}

	return body, nil
}

// getJson makes a GET request and returns JSON data
func (c *JiraClient) getJson(endpoint string, params map[string]string) (map[string]any, error) {
	body, err := c.doRequest("GET", endpoint, params)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// getJsonList makes a GET request and returns a JSON array
func (c *JiraClient) getJsonList(endpoint string, params map[string]string) ([]map[string]any, error) {
	body, err := c.doRequest("GET", endpoint, params)
	if err != nil {
		return nil, err
	}

	var result []map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// isStale returns true if the issue is past its target date and not done
func isStale(targetEnd string) bool {
	days, ok := DaysFromNow(targetEnd)
	if !ok {
		return false
	}
	return days < 0
}

// isDueWithinDays returns true if the issue has a target end date within the next n days and is not done.
func isDueWithinDays(targetEnd string, days int) bool {
	leftToGo, ok := DaysFromNow(targetEnd)
	if !ok {
		return false
	}
	return leftToGo > 0 && leftToGo <= days
}

// Helper functions for parsing JSON responses

func getString(m map[string]any, key string, defaultValue string) string {
	if m == nil {
		return defaultValue
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultValue
}

func getInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

func getMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if sub, ok := v.(map[string]any); ok {
			return sub
		}
	}
	return nil
}

func getMapList(m map[string]any, key string) []map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if list, ok := v.([]any); ok {
			result := make([]map[string]any, 0, len(list))
			for _, item := range list {
				if sub, ok := item.(map[string]any); ok {
					result = append(result, sub)
				}
			}
			return result
		}
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
