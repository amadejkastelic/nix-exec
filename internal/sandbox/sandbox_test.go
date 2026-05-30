package sandbox

import (
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxBytes int64
		want     string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello\n[OUTPUT TRUNCATED]"},
		{"", 5, ""},
		{"héllo", 5, "héll\n[OUTPUT TRUNCATED]"},
		{"hello", 3, "hel\n[OUTPUT TRUNCATED]"},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxBytes)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxBytes, got, tt.want)
		}
	}
}

func TestTruncatePreservesValidUTF8(t *testing.T) {
	input := "hello world"
	got := truncate(input, 5)
	for _, r := range got {
		if r == utf8.RuneError {
			t.Error("truncated output contains invalid UTF-8 runes")
		}
	}
}
