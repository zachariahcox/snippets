package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadJiraCreds_fromFile(t *testing.T) {
	// Create a temp creds script that exports JIRA_* vars
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "creds.sh")
	script := `export JIRA_SERVER="https://test-jira.example.com"
export JIRA_API_TOKEN="test-token-from-file"
export JIRA_EMAIL="test@example.com"
`
	if err := os.WriteFile(credsPath, []byte(script), 0700); err != nil {
		t.Fatalf("write creds file: %v", err)
	}

	// Clear env so values must come from the file
	t.Setenv("JIRA_SERVER", "")
	t.Setenv("JIRA_API_TOKEN", "")
	t.Setenv("JIRA_EMAIL", "")

	server, token, email, err := loadJiraCreds(credsPath)
	if err != nil {
		t.Fatalf("loadJiraCreds(credsPath): %v", err)
	}
	if server != "https://test-jira.example.com" {
		t.Errorf("server = %q, want https://test-jira.example.com", server)
	}
	if token != "test-token-from-file" {
		t.Errorf("token = %q, want test-token-from-file", token)
	}
	if email != "test@example.com" {
		t.Errorf("email = %q, want test@example.com", email)
	}
}

func TestLoadJiraCreds_fromEnv(t *testing.T) {
	// Use non-existent creds path so only env is used
	credsPath := filepath.Join(t.TempDir(), "nonexistent-creds.sh")

	t.Setenv("JIRA_SERVER", "https://env.example.com")
	t.Setenv("JIRA_API_TOKEN", "env-token")
	t.Setenv("JIRA_EMAIL", "env@example.com")

	server, token, email, err := loadJiraCreds(credsPath)
	if err != nil {
		t.Fatalf("loadJiraCreds: %v", err)
	}
	if server != "https://env.example.com" || token != "env-token" || email != "env@example.com" {
		t.Errorf("got server=%q token=%q email=%q", server, token, email)
	}
}

func TestOptionalJiraCustomFieldNames_fromCredsFile(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "creds.sh")
	script := `export JIRA_DUE_DATE_FIELD="Ship target"
export JIRA_TRENDING_STATUS_FIELD="Health"
`
	if err := os.WriteFile(credsPath, []byte(script), 0700); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	t.Setenv("JIRA_DUE_DATE_FIELD", "")
	t.Setenv("JIRA_TRENDING_STATUS_FIELD", "")
	due, trend := loadJiraCustomFieldNames(credsPath)
	if due != "Ship target" || trend != "Health" {
		t.Errorf("due=%q trend=%q", due, trend)
	}
}

func TestLoadJiraCreds_missingRequired(t *testing.T) {
	credsPath := filepath.Join(t.TempDir(), "creds.sh") // no file, so env only

	t.Setenv("JIRA_SERVER", "")
	t.Setenv("JIRA_API_TOKEN", "tok")
	t.Setenv("JIRA_EMAIL", "")
	_, _, _, err := loadJiraCreds(credsPath)
	if err == nil {
		t.Fatal("expected error when JIRA_SERVER missing")
	}
	if !strings.Contains(err.Error(), "JIRA_SERVER") {
		t.Errorf("error should mention JIRA_SERVER: %v", err)
	}

	t.Setenv("JIRA_SERVER", "https://x.com")
	t.Setenv("JIRA_API_TOKEN", "")
	_, _, _, err = loadJiraCreds(credsPath)
	if err == nil {
		t.Fatal("expected error when JIRA_API_TOKEN missing")
	}
	if !strings.Contains(err.Error(), "JIRA_API_TOKEN") {
		t.Errorf("error should mention JIRA_API_TOKEN: %v", err)
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2025, 2, 24, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		input    string
		wantDate string
		wantErr  bool
	}{
		{"", "", false},
		{"2026-01-02", "2026-01-02", false},
		{"2025-01-15", "2025-01-15", false},
		{"14", "2025-02-10", false},
		{"0", "2025-02-24", false},
		{"1", "2025-02-23", false},
		{" 14 ", "2025-02-10", false},
		{"invalid", "", true},
		{"2025-13-01", "", true},
	}
	for _, tt := range tests {
		got, err := ParseSince(tt.input, now)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseSince(%q) want error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSince(%q) err=%v", tt.input, err)
			continue
		}
		if tt.wantDate == "" {
			if got != nil {
				t.Errorf("ParseSince(%q) want nil, got %v", tt.input, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("ParseSince(%q) want %s, got nil", tt.input, tt.wantDate)
			continue
		}
		if gotStr := got.Format("2006-01-02"); gotStr != tt.wantDate {
			t.Errorf("ParseSince(%q) = %s, want %s", tt.input, gotStr, tt.wantDate)
		}
	}
}
