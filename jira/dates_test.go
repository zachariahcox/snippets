package jira

import (
	"testing"
	"time"
)

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
