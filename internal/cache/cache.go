// Package cache manages the shared on-disk cache directory under ~/.snippets/cache.
package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zachariahcox/snippets/internal/logging"
)

// DirHook, when non-nil, overrides the cache directory (for tests).
var DirHook func() (string, error)

// Dir returns the cache directory (~/.snippets/cache by default).
func Dir() (string, error) {
	if DirHook != nil {
		return DirHook()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".snippets", "cache"), nil
}

// PathForKey returns the full path for a filename-safe cache key (hex digest, etc.).
func PathForKey(key string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key+".json"), nil
}

// EnsureDir creates the cache directory if it does not exist.
func EnsureDir() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// Prune removes cache files older than maxAge from the cache directory.
func Prune(maxAge time.Duration) error {
	dir, err := Dir()
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
				logging.Warning("Failed to prune cache file %s: %v", path, err)
			} else {
				logging.Debug("Pruned old cache: %s", e.Name())
			}
		}
	}
	return nil
}

// Clear removes all regular files in the cache directory.
func Clear() error {
	dir, err := Dir()
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
			logging.Warning("Failed to remove cache file %s: %v", path, err)
		}
	}
	return nil
}

// Valid returns true if path exists and its mtime is within maxAge.
func Valid(path string, maxAge time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) <= maxAge
}
