package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if Valid(path, time.Hour) {
		t.Error("nonexistent file should be invalid")
	}
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if !Valid(path, time.Hour) {
		t.Error("recent file should be valid")
	}
}

func TestPathForKey(t *testing.T) {
	DirHook = func() (string, error) { return t.TempDir(), nil }
	defer func() { DirHook = nil }()
	p, err := PathForKey("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "abc123.json" {
		t.Errorf("basename = %s, want abc123.json", filepath.Base(p))
	}
}
