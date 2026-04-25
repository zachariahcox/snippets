package jira

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/zachariahcox/snippets/internal/cache"
	"github.com/zachariahcox/snippets/internal/logging"
)

// ErrCacheMiss is returned by FetchReportIssues when client is nil and no valid cache entry exists.
var ErrCacheMiss = errors.New("cache miss")

const issueCacheTTL = 30 * time.Minute

type issueCacheEntry struct {
	ParentIssues []*IssueData `json:"parent_issues"`
}

// CacheKey returns a deterministic filename-safe key for the query (JQL or sorted issue keys),
// whether child issues were loaded, and due-date / trending field configuration.
func CacheKey(cfg *ReportConfig, issueKeys []string) string {
	if cfg == nil {
		cfg = &ReportConfig{}
	}
	var parts []string
	if cfg.JQLQuery != "" {
		parts = []string{"jql:", cfg.JQLQuery}
	} else {
		k := make([]string, len(issueKeys))
		copy(k, issueKeys)
		sort.Strings(k)
		parts = []string{"keys:", strings.Join(k, ",")}
	}
	if cfg.IncludeChildren {
		parts = append(parts, "|children:1")
	} else {
		parts = append(parts, "|children:0")
	}
	parts = append(parts, "|dueField:", strings.TrimSpace(cfg.DueDateFieldName), "|trendField:", strings.TrimSpace(cfg.TrendingStatusFieldName))
	// Namespace so other subtools can share ~/.snippets/cache without key collisions.
	parts = append([]string{"jira:v1"}, parts...)
	h := sha256.Sum256([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(h[:])
}

// ReadCache reads a cache file and returns parent issues (including nested Children when present).
func ReadCache(path string) (parentIssues []*IssueData, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ent issueCacheEntry
	if err := json.Unmarshal(data, &ent); err != nil {
		return nil, err
	}
	return ent.ParentIssues, nil
}

// WriteCache writes parent issues to a cache file.
func WriteCache(path string, parentIssues []*IssueData) error {
	if parentIssues == nil {
		parentIssues = []*IssueData{}
	}
	ent := issueCacheEntry{ParentIssues: parentIssues}
	data, err := json.MarshalIndent(ent, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// FetchReportIssues generates a report of issues. It tries the cache first; on hit it returns
// cached data (client may be nil for cache-only lookup). On cache miss with client == nil it
// returns ErrCacheMiss. On cache miss with client != nil it fetches from Jira, writes the
// cache, and returns the result.
func FetchReportIssues(client *JiraClient, issueKeys []string, cfg *ReportConfig) ([]*IssueData, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg is nil")
	}
	logging.Info("Fetching issues for configuration: %v", cfg)

	key := CacheKey(cfg, issueKeys)
	if err := cache.EnsureDir(); err != nil {
		logging.Warning("Cache dir unavailable: %v", err)
	} else {
		path, err := cache.PathForKey(key)
		if err == nil && cache.Valid(path, issueCacheTTL) {
			parentIssues, err := ReadCache(path)
			if err == nil {
				logging.Info("Using cached results at %s.", path)
				return parentIssues, nil
			}
			logging.Debug("Cache read failed: %v", err)
		}
	}

	if client == nil {
		return nil, ErrCacheMiss
	}

	client.prepareFieldResolution(cfg)

	_ = cache.Prune(issueCacheTTL)

	var parentIssues []*IssueData
	if cfg.JQLQuery != "" {
		issues, err := client.FetchIssuesFromQuery(cfg.JQLQuery)
		if err != nil {
			logging.Error("JQL query failed: %v", err)
			return nil, err
		}
		parentIssues = issues

		issueKeys = make([]string, len(parentIssues))
		for i, issue := range parentIssues {
			issueKeys[i] = issue.Key
		}
	} else {
		issues, err := client.FetchIssuesByKeys(issueKeys)
		if err != nil {
			logging.Error("Failed to fetch issues: %v", err)
			return nil, err
		}
		parentIssues = issues
	}

	if cfg.IncludeChildren {
		client.loadChildren(parentIssues)
	}

	client.loadComments(parentIssues)

	for _, issue := range parentIssues {
		computeTrending(issue)
	}

	if path, err := cache.PathForKey(key); err == nil {
		if wErr := WriteCache(path, parentIssues); wErr != nil {
			logging.Warning("Failed to write cache: %v", wErr)
		} else {
			logging.Debug("Cached results to %s", path)
		}
	}

	return parentIssues, nil
}
