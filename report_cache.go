package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zachariahcox/snippets/filecache"
)

const reportCacheTTL = 30 * time.Minute

// ErrCacheMiss is returned by FetchReportIssues when client is nil and no valid cache entry exists.
var ErrCacheMiss = errors.New("cache miss")

// reportCacheDirFn resolves the on-disk cache root; tests may replace it.
var reportCacheDirFn = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".snippets", "cache"), nil
}

var reportCache = &filecache.Store{
	Dir: func() (string, error) { return reportCacheDirFn() },
	Warn: func(format string, args ...any) {
		logWarning(format, args...)
	},
	Debug: func(format string, args ...any) {
		logDebug(format, args...)
	},
}

// issueCacheFile is the on-disk JSON envelope for Jira issue snapshots.
type issueCacheFile struct {
	ParentIssues []*IssueData `json:"parent_issues"`
}

// CacheDir returns the cache directory (~/.snippets/cache by default).
func CacheDir() (string, error) {
	return reportCacheDirFn()
}

// CacheKey returns a deterministic filename-safe key for the query (JQL or sorted issue keys),
// whether child issues were loaded, and due-date / trending field configuration (must match FetchReportIssues).
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
	return filecache.KeyFromString(strings.Join(parts, ""))
}

// ClearCache removes all files in the cache directory.
func ClearCache() error {
	return reportCache.Clear()
}

func readIssueCache(path string) ([]*IssueData, error) {
	ent, err := filecache.ReadJSON[issueCacheFile](path)
	if err != nil {
		return nil, err
	}
	return ent.ParentIssues, nil
}

func writeIssueCache(path string, parentIssues []*IssueData) error {
	if parentIssues == nil {
		parentIssues = []*IssueData{}
	}
	return filecache.WriteJSON(path, issueCacheFile{ParentIssues: parentIssues})
}
