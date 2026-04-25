package jira

import (
	"path/filepath"
	"testing"

	"github.com/zachariahcox/snippets/internal/cache"
)

func TestCacheKey_deterministic(t *testing.T) {
	cfg1 := &ReportConfig{JQLQuery: "project = X"}
	k1 := CacheKey(cfg1, nil)
	k2 := CacheKey(&ReportConfig{JQLQuery: "project = X"}, nil)
	if k1 != k2 {
		t.Errorf("same inputs gave different keys: %q vs %q", k1, k2)
	}
	k3 := CacheKey(&ReportConfig{}, []string{"A-1", "B-2"})
	k4 := CacheKey(&ReportConfig{}, []string{"B-2", "A-1"})
	if k3 != k4 {
		t.Errorf("same keys different order should match: %q vs %q", k3, k4)
	}
	if CacheKey(&ReportConfig{JQLQuery: "jql1"}, nil) == CacheKey(&ReportConfig{JQLQuery: "jql2"}, nil) {
		t.Error("different JQL should give different keys")
	}
	if CacheKey(&ReportConfig{JQLQuery: "project = X", IncludeChildren: false}, nil) == CacheKey(&ReportConfig{JQLQuery: "project = X", IncludeChildren: true}, nil) {
		t.Error("includeChildren flag should affect cache key")
	}
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

func TestFetchReportIssues_usesCache(t *testing.T) {
	dir := t.TempDir()
	oldHook := cache.DirHook
	cache.DirHook = func() (string, error) { return dir, nil }
	defer func() { cache.DirHook = oldHook }()

	if err := cache.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	cfg := &ReportConfig{
		Title:    "Test",
		JQLQuery: "",
	}
	issueKeys := []string{"P-1"}

	key := CacheKey(cfg, issueKeys)
	path, err := cache.PathForKey(key)
	if err != nil {
		t.Fatalf("PathForKey: %v", err)
	}
	parent := []*IssueData{
		{Key: "P-1", Summary: "Cached issue", Status: "done", TrendingEmoji: "🟣"},
	}
	if err := WriteCache(path, parent); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	gotParent, err := FetchReportIssues(nil, issueKeys, cfg)
	if err != nil {
		t.Fatalf("FetchReportIssues (cache hit): %v", err)
	}
	if len(gotParent) != 1 || gotParent[0].Key != "P-1" || gotParent[0].Summary != "Cached issue" {
		t.Errorf("cache hit: got parent %+v", gotParent)
	}
}
