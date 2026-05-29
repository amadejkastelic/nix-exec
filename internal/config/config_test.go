package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Server.Name != "nix-exec" {
		t.Errorf("expected server name 'nix-exec', got %s", cfg.Server.Name)
	}
	if cfg.Sandbox.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", cfg.Sandbox.Timeout)
	}
	if cfg.Sandbox.MaxOutputBytes != 1<<20 {
		t.Errorf("expected max output 1MB, got %d", cfg.Sandbox.MaxOutputBytes)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level 'info', got %s", cfg.Logging.Level)
	}
}

func TestLoadNonexistent(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got %v", err)
	}
	if cfg.Server.Name != "nix-exec" {
		t.Errorf("expected defaults, got %s", cfg.Server.Name)
	}
}

func TestLoadEmpty(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error for empty path, got %v", err)
	}
	if cfg.Server.Name != "nix-exec" {
		t.Errorf("expected defaults, got %s", cfg.Server.Name)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  name: "test-server"
sandbox:
  timeout: 60s
  max_output_bytes: 2097152
logging:
  level: "debug"
  format: "text"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Name != "test-server" {
		t.Errorf("expected name 'test-server', got %s", cfg.Server.Name)
	}
	if cfg.Sandbox.Timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", cfg.Sandbox.Timeout)
	}
	if cfg.Sandbox.MaxOutputBytes != 2097152 {
		t.Errorf("expected max output 2MB, got %d", cfg.Sandbox.MaxOutputBytes)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level 'debug', got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("expected format 'text', got %s", cfg.Logging.Format)
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"unknown", "INFO"},
	}

	for _, tt := range tests {
		cfg := Default()
		cfg.Logging.Level = tt.level
		got := cfg.LogLevel().String()
		if got != tt.want {
			t.Errorf("LogLevel() for %q = %q, want %q", tt.level, got, tt.want)
		}
	}
}
