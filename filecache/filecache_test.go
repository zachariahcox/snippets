package filecache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKeyFromString_deterministic(t *testing.T) {
	a := KeyFromString("hello")
	b := KeyFromString("hello")
	if a != b {
		t.Fatalf("same input: %q vs %q", a, b)
	}
	if a == KeyFromString("world") {
		t.Fatal("different input should not collide")
	}
}

func TestWriteReadJSON_roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	type payload struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	want := payload{Name: "a", N: 42}
	if err := WriteJSON(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadJSON[payload](path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

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

func TestStore_Prune(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: func() (string, error) { return dir, nil }}
	path := filepath.Join(dir, "old.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune(time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("old file should be pruned")
	}
}
