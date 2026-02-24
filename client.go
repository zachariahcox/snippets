package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// JiraClient is a simple Jira REST API client
type JiraClient struct {
	Server     string
	Email      string
	APIToken   string
	APIVersion string
	IsCloud    bool
	HTTPClient *http.Client
}

// NewJiraClient creates a new Jira client
func NewJiraClient(server, apiToken, email string) (*JiraClient, error) {
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
		logDebug("Using Jira Server/Data Center authentication (API v%s)", apiVersion)
	}

	return &JiraClient{
		Server:     server,
		Email:      email,
		APIToken:   apiToken,
		APIVersion: apiVersion,
		IsCloud:    isCloud,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
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

// Get makes a GET request and returns JSON data
func (c *JiraClient) Get(endpoint string, params map[string]string) (map[string]any, error) {
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

// GetList makes a GET request and returns a JSON array
func (c *JiraClient) GetList(endpoint string, params map[string]string) ([]map[string]any, error) {
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

// GetIssue fetches a single issue by key
func (c *JiraClient) GetIssue(issueKey string) (map[string]any, error) {
	fields := "summary,status,assignee,priority,created,updated,subtasks,issuelinks"
	// Add custom field IDs
	for _, id := range customFields {
		if id != "" {
			fields += "," + id
		}
	}
	return c.Get(fmt.Sprintf("issue/%s", issueKey), map[string]string{"fields": fields})
}

// LoadCustomFields resolves custom field names to IDs
func (c *JiraClient) LoadCustomFields(fieldNames map[string]string) error {
	fields, err := c.GetList("field", nil)
	if err != nil {
		return err
	}

	for name := range fieldNames {
		for _, f := range fields {
			if getString(f, "name") == name {
				fieldNames[name] = getString(f, "id")
				break
			}
		}
	}

	return nil
}

// SearchIssues searches for issues using JQL with pagination
func (c *JiraClient) SearchIssues(jql string, maxResults int) ([]map[string]any, error) {
	fields := "summary,status,assignee,priority,created,updated"

	// Load custom fields first
	if err := c.LoadCustomFields(customFields); err != nil {
		logWarning("Could not load custom fields: %v", err)
	}

	// Add custom field IDs
	for _, id := range customFields {
		if id != "" {
			fields += "," + id
		}
	}

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
		response, err := c.Get("search", params)
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

// TestConnection tests the connection to Jira
func (c *JiraClient) TestConnection() bool {
	_, err := c.Get("myself", nil)
	if err != nil {
		logError("Connection test failed: %v", err)
		return false
	}
	return true
}

// GetJiraClient creates a Jira client from environment variables
func GetJiraClient() (*JiraClient, error) {
	server := os.Getenv("JIRA_SERVER")
	email := os.Getenv("JIRA_EMAIL")
	apiToken := os.Getenv("JIRA_API_TOKEN")

	if server == "" {
		return nil, fmt.Errorf("JIRA_SERVER environment variable is not set.\nExample: export JIRA_SERVER=https://mycompany.atlassian.net")
	}

	if apiToken == "" {
		isCloud := strings.Contains(strings.ToLower(server), ".atlassian.net")
		if isCloud {
			return nil, fmt.Errorf("JIRA_API_TOKEN environment variable is not set.\nFor Jira Cloud: Generate token at https://id.atlassian.com/manage-profile/security/api-tokens")
		}
		return nil, fmt.Errorf("JIRA_API_TOKEN environment variable is not set.\nFor Jira Server/Data Center: Generate a Personal Access Token (PAT) in your Jira profile")
	}

	isCloud := strings.Contains(strings.ToLower(server), ".atlassian.net")
	if isCloud && email == "" {
		return nil, fmt.Errorf("JIRA_EMAIL environment variable is required for Jira Cloud.\nSet it to your Atlassian account email")
	}

	client, err := NewJiraClient(server, apiToken, email)
	if err != nil {
		return nil, err
	}

	if !client.TestConnection() {
		if !isCloud {
			return nil, fmt.Errorf("failed to connect to Jira. Check your credentials and server URL.\nFor Jira Server/Data Center, ensure you're using a valid Personal Access Token (PAT)")
		}
		return nil, fmt.Errorf("failed to connect to Jira. Check your credentials and server URL")
	}

	logDebug("Connected to Jira server: %s", server)
	return client, nil
}

// Helper functions for parsing JSON responses

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
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
