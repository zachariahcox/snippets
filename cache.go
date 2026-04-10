package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrCacheMiss is returned by FetchReportIssues when client is nil and no valid cache entry exists.
var ErrCacheMiss = errors.New("cache miss")

const cacheTTL = 30 * time.Minute

// cacheEntry is the on-disk format for cached issue data.
type cacheEntry struct {
	ParentIssues []*IssueData `json:"parent_issues"`
}

// cacheDirFn is used by CacheDir; tests can override to use a temp dir.
var cacheDirFn = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".snippets", "cache"), nil
}

// CacheDir returns the cache directory (~/.snippets/cache by default).
func CacheDir() (string, error) {
	return cacheDirFn()
}

// CacheKey returns a deterministic filename-safe key for the query (JQL or sorted issue keys)
// and whether child issues were loaded (must match FetchReportIssues behavior).
func CacheKey(jql string, keys []string, includeChildren bool) string {
	var parts []string
	if jql != "" {
		parts = []string{"jql:", jql}
	} else {
		k := make([]string, len(keys))
		copy(k, keys)
		sort.Strings(k)
		parts = []string{"keys:", strings.Join(k, ",")}
	}
	if includeChildren {
		parts = append(parts, "|children:1")
	} else {
		parts = append(parts, "|children:0")
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(h[:])
}

// cachePath returns the full path for a cache key.
func cachePath(key string) (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key+".json"), nil
}

// EnsureCacheDir creates ~/.snippets/cache if it does not exist.
func EnsureCacheDir() error {
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// PruneCache removes cache files older than maxAge from the cache directory.
func PruneCache(maxAge time.Duration) error {
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				logWarning("Failed to prune cache file %s: %v", path, err)
			} else {
				logDebug("Pruned old cache: %s", e.Name())
			}
		}
	}
	return nil
}

// ClearCache removes all files in the cache directory.
func ClearCache() error {
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := os.Remove(path); err != nil {
			logWarning("Failed to remove cache file %s: %v", path, err)
		}
	}
	return nil
}

// CacheValid returns true if path exists and its mtime is within maxAge.
func CacheValid(path string, maxAge time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) <= maxAge
}

// ReadCache reads a cache file and returns parent issues (including nested Children when present).
func ReadCache(path string) (parentIssues []*IssueData, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ent cacheEntry
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
	ent := cacheEntry{ParentIssues: parentIssues}
	data, err := json.MarshalIndent(ent, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
