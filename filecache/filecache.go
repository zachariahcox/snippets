// Package filecache stores JSON payloads in a directory keyed by SHA-256 hex filenames,
// with optional TTL checks and pruning.
package filecache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// KeyFromString returns a fixed-length hex string suitable as a cache filename stem
// for the given logical key material.
func KeyFromString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Store manages JSON files under a root directory returned by Dir.
type Store struct {
	// Dir returns the cache root directory. Required for all methods that touch disk.
	Dir func() (string, error)

	// Warn, if set, receives non-fatal problems (e.g. failed remove during prune).
	Warn func(format string, args ...any)
	// Debug, if set, receives prune diagnostics.
	Debug func(format string, args ...any)
}

func (s *Store) root() (string, error) {
	if s == nil || s.Dir == nil {
		return "", fmt.Errorf("filecache: Store or Dir is nil")
	}
	return s.Dir()
}

// Path returns the path to the JSON file for a cache key (hex stem + ".json").
func (s *Store) Path(key string) (string, error) {
	dir, err := s.root()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key+".json"), nil
}

// EnsureDir creates the cache root if it does not exist.
func (s *Store) EnsureDir() error {
	dir, err := s.root()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// Prune removes files in the cache root whose mod time is older than maxAge.
func (s *Store) Prune(maxAge time.Duration) error {
	dir, err := s.root()
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
				if s.Warn != nil {
					s.Warn("Failed to prune cache file %s: %v", path, err)
				}
			} else if s.Debug != nil {
				s.Debug("Pruned old cache: %s", e.Name())
			}
		}
	}
	return nil
}

// Clear removes all regular files in the cache root (not subdirectories).
func (s *Store) Clear() error {
	dir, err := s.root()
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
			if s.Warn != nil {
				s.Warn("Failed to remove cache file %s: %v", path, err)
			}
		}
	}
	return nil
}

// Valid reports whether path exists and its mod time is within maxAge of now.
func Valid(path string, maxAge time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) <= maxAge
}

// ReadJSON reads and unmarshals path into T.
func ReadJSON[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return zero, err
	}
	return v, nil
}

// WriteJSON marshals v with indentation and writes it to path (0644).
func WriteJSON[T any](path string, v T) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
