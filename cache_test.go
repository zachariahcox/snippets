package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheKey_deterministic(t *testing.T) {
	k1 := CacheKey("project = X", nil, false)
	k2 := CacheKey("project = X", nil, false)
	if k1 != k2 {
		t.Errorf("same inputs gave different keys: %q vs %q", k1, k2)
	}
	// Key order should not matter
	k3 := CacheKey("", []string{"A-1", "B-2"}, false)
	k4 := CacheKey("", []string{"B-2", "A-1"}, false)
	if k3 != k4 {
		t.Errorf("same keys different order should match: %q vs %q", k3, k4)
	}
	// Different query -> different key
	if CacheKey("jql1", nil, false) == CacheKey("jql2", nil, false) {
		t.Error("different JQL should give different keys")
	}
	// --children on vs off must not share cache
	if CacheKey("project = X", nil, false) == CacheKey("project = X", nil, true) {
		t.Error("includeChildren flag should affect cache key")
	}
}

func TestWriteReadCache_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	parent := []*IssueData{
		{
			Key: "P-1", Summary: "One", Status: "done", TrendingEmoji: "🟣",
			Children: []*IssueData{
				{Key: "C-1", Summary: "Sub", Status: "in progress"},
			},
		},
	}

	if err := WriteCache(path, parent); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}
	p2, err := ReadCache(path)
	if err != nil {
		t.Fatalf("ReadCache: %v", err)
	}
	if len(p2) != 1 || p2[0].Key != "P-1" || p2[0].Summary != "One" {
		t.Errorf("parent mismatch: got %+v", p2)
	}
	if len(p2[0].Children) != 1 || p2[0].Children[0].Key != "C-1" {
		t.Errorf("nested children mismatch: got %+v", p2[0].Children)
	}
}

func TestCacheValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if CacheValid(path, time.Hour) {
		t.Error("nonexistent file should be invalid")
	}
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if !CacheValid(path, time.Hour) {
		t.Error("recent file should be valid")
	}
}

// TestFetchReportIssues_usesCache verifies that FetchReportIssues returns cached data when the
// cache is primed, so tests (and the real binary) can rely on cache hits.
func TestFetchReportIssues_usesCache(t *testing.T) {
	dir := t.TempDir()
	oldFn := cacheDirFn
	cacheDirFn = func() (string, error) { return dir, nil }
	defer func() { cacheDirFn = oldFn }()

	if err := EnsureCacheDir(); err != nil {
		t.Fatalf("EnsureCacheDir: %v", err)
	}

	cfg := &ReportConfig{
		Title:    "Test",
		JQLQuery: "",
	}
	issueKeys := []string{"P-1"}

	// Prime the cache with the same key FetchReportIssues would use
	key := CacheKey(cfg.JQLQuery, issueKeys, cfg.IncludeChildren)
	path, err := cachePath(key)
	if err != nil {
		t.Fatalf("cachePath: %v", err)
	}
	parent := []*IssueData{
		{Key: "P-1", Summary: "Cached issue", Status: "done", TrendingEmoji: "🟣"},
	}
	if err := WriteCache(path, parent); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	// FetchReportIssues with nil client should hit the cache
	gotParent, err := FetchReportIssues(nil, issueKeys, cfg)
	if err != nil {
		t.Fatalf("FetchReportIssues (cache hit): %v", err)
	}
	if len(gotParent) != 1 || gotParent[0].Key != "P-1" || gotParent[0].Summary != "Cached issue" {
		t.Errorf("cache hit: got parent %+v", gotParent)
	}
}
