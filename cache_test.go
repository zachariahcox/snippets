package main

import (
	"path/filepath"
	"testing"
)

func TestCacheKey_deterministic(t *testing.T) {
	cfg1 := &ReportConfig{JQLQuery: "project = X"}
	k1 := CacheKey(cfg1, nil)
	k2 := CacheKey(&ReportConfig{JQLQuery: "project = X"}, nil)
	if k1 != k2 {
		t.Errorf("same inputs gave different keys: %q vs %q", k1, k2)
	}
	// Key order should not matter
	k3 := CacheKey(&ReportConfig{}, []string{"A-1", "B-2"})
	k4 := CacheKey(&ReportConfig{}, []string{"B-2", "A-1"})
	if k3 != k4 {
		t.Errorf("same keys different order should match: %q vs %q", k3, k4)
	}
	// Different query -> different key
	if CacheKey(&ReportConfig{JQLQuery: "jql1"}, nil) == CacheKey(&ReportConfig{JQLQuery: "jql2"}, nil) {
		t.Error("different JQL should give different keys")
	}
	// --children on vs off must not share cache
	if CacheKey(&ReportConfig{JQLQuery: "project = X", IncludeChildren: false}, nil) == CacheKey(&ReportConfig{JQLQuery: "project = X", IncludeChildren: true}, nil) {
		t.Error("includeChildren flag should affect cache key")
	}
	// Custom field config affects key
	base := &ReportConfig{JQLQuery: "project = X"}
	if CacheKey(base, nil) == CacheKey(&ReportConfig{JQLQuery: "project = X", DueDateFieldName: "Planned end"}, nil) {
		t.Error("DueDateFieldName should affect cache key")
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

	if err := writeIssueCache(path, parent); err != nil {
		t.Fatalf("writeIssueCache: %v", err)
	}
	p2, err := readIssueCache(path)
	if err != nil {
		t.Fatalf("readIssueCache: %v", err)
	}
	if len(p2) != 1 || p2[0].Key != "P-1" || p2[0].Summary != "One" {
		t.Errorf("parent mismatch: got %+v", p2)
	}
	if len(p2[0].Children) != 1 || p2[0].Children[0].Key != "C-1" {
		t.Errorf("nested children mismatch: got %+v", p2[0].Children)
	}
}

// TestFetchReportIssues_usesCache verifies that FetchReportIssues returns cached data when the
// cache is primed, so tests (and the real binary) can rely on cache hits.
func TestFetchReportIssues_usesCache(t *testing.T) {
	dir := t.TempDir()
	oldFn := reportCacheDirFn
	reportCacheDirFn = func() (string, error) { return dir, nil }
	defer func() { reportCacheDirFn = oldFn }()

	if err := reportCache.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	cfg := &ReportConfig{
		Title:    "Test",
		JQLQuery: "",
	}
	issueKeys := []string{"P-1"}

	// Prime the cache with the same key FetchReportIssues would use
	key := CacheKey(cfg, issueKeys)
	path, err := reportCache.Path(key)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	parent := []*IssueData{
		{Key: "P-1", Summary: "Cached issue", Status: "done", TrendingEmoji: "🟣"},
	}
	if err := writeIssueCache(path, parent); err != nil {
		t.Fatalf("writeIssueCache: %v", err)
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
