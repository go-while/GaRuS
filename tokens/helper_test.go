package tokens

import (
	"os"
	"testing"
)

func TestParseUnixTimestamp(t *testing.T) {
	// Test valid timestamp string
	ts := "1717000000"
	val, err := parseUnixTimestamp(ts)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if val != 1717000000 {
		t.Errorf("Expected 1717000000, got %d", val)
	}

	// Test invalid timestamp string
	_, err = parseUnixTimestamp("notanumber")
	if err == nil {
		t.Error("Expected error for invalid timestamp, got nil")
	}
}

func TestFileSHA256(t *testing.T) {
	// Create a temp file with known contents
	tmpfile, err := os.CreateTemp("", "sha256test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	content := []byte("hello world")
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpfile.Close()

	hash, err := fileSHA256(tmpfile.Name())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Precomputed SHA256 hash of "hello world"
	const expected = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if hash != expected {
		t.Errorf("Expected hash %s, got %s", expected, hash)
	}

	// Test file not found
	_, err = fileSHA256("/nonexistent/file")
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

func TestFormatDurationHuman(t *testing.T) {
	tests := []struct {
		seconds int64
		want    string
	}{
		{45, "45 sec"},
		{SecondsPerMinute + 5, "1 min, 5 sec"},
		{2*SecondsPerHour + 30*SecondsPerMinute, "2 hr, 30 min"},
		{3*SecondsPerDay + 4*SecondsPerHour, "3 days, 4 hr"},
		{SecondsPerHour, "1 hr, 0 min"},
		{SecondsPerDay, "1 day, 0 hr"},
	}

	for _, tt := range tests {
		got := FormatDurationHuman(tt.seconds)
		if got != tt.want {
			t.Errorf("FormatDurationHuman(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if plural(1) != "" {
		t.Errorf("plural(1) = %q, want \"\"", plural(1))
	}
	if plural(0) != "s" {
		t.Errorf("plural(0) = %q, want \"s\"", plural(0))
	}
	if plural(2) != "s" {
		t.Errorf("plural(2) = %q, want \"s\"", plural(2))
	}
}
