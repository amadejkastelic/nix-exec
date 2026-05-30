package config

import (
	"flag"
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
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
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

func TestApplyFlagsExplicit(t *testing.T) {
	cfg := Default()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fp := cfg.RegisterFlags(fs)

	args := []string{
		"-name", "flag-server",
		"-timeout", "45s",
		"-max-output-bytes", "2048",
		"-workspace-path", "/tmp/ws",
		"-package-denylist", "bash,python",
		"-cache-dir", "/tmp/cache",
		"-temp-dir", "/var/tmp",
		"-nixpkgs-url", "github:user/nixpkgs",
		"-substituters", "https://cache1,https://cache2",
		"-log-level", "debug",
		"-log-format", "text",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	cfg.ApplyFlags(fs, fp)

	if cfg.Server.Name != "flag-server" {
		t.Errorf("expected name 'flag-server', got %s", cfg.Server.Name)
	}
	if cfg.Sandbox.Timeout != 45*time.Second {
		t.Errorf("expected timeout 45s, got %v", cfg.Sandbox.Timeout)
	}
	if cfg.Sandbox.MaxOutputBytes != 2048 {
		t.Errorf("expected max output 2048, got %d", cfg.Sandbox.MaxOutputBytes)
	}
	if cfg.Sandbox.WorkspacePath != "/tmp/ws" {
		t.Errorf("expected workspace '/tmp/ws', got %s", cfg.Sandbox.WorkspacePath)
	}
	if len(cfg.Sandbox.PackageDenylist) != 2 || cfg.Sandbox.PackageDenylist[0] != "bash" ||
		cfg.Sandbox.PackageDenylist[1] != "python" {
		t.Errorf("expected denylist [bash, python], got %v", cfg.Sandbox.PackageDenylist)
	}
	if cfg.Executor.CacheDir != "/tmp/cache" {
		t.Errorf("expected cache '/tmp/cache', got %s", cfg.Executor.CacheDir)
	}
	if cfg.Executor.TempDir != "/var/tmp" {
		t.Errorf("expected temp '/var/tmp', got %s", cfg.Executor.TempDir)
	}
	if cfg.Executor.NixpkgsURL != "github:user/nixpkgs" {
		t.Errorf("expected nixpkgs URL 'github:user/nixpkgs', got %s", cfg.Executor.NixpkgsURL)
	}
	if len(cfg.Executor.Substituters) != 2 || cfg.Executor.Substituters[0] != "https://cache1" ||
		cfg.Executor.Substituters[1] != "https://cache2" {
		t.Errorf("expected substituters [cache1, cache2], got %v", cfg.Executor.Substituters)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level 'debug', got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("expected format 'text', got %s", cfg.Logging.Format)
	}
}

func TestApplyFlagsUnsetDoesNotOverride(t *testing.T) {
	cfg := Default()
	cfg.Sandbox.Timeout = 99 * time.Second
	cfg.Logging.Level = "warn"

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fp := cfg.RegisterFlags(fs)

	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	cfg.ApplyFlags(fs, fp)

	if cfg.Sandbox.Timeout != 99*time.Second {
		t.Errorf("unset flag should not override, got %v", cfg.Sandbox.Timeout)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("unset flag should not override, got %s", cfg.Logging.Level)
	}
}

func TestApplyFlagsOverridesFileConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  name: "file-server"
sandbox:
  timeout: 60s
logging:
  level: "debug"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fp := loaded.RegisterFlags(fs)

	if err := fs.Parse([]string{"-name", "flag-server", "-log-level", "error"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	loaded.ApplyFlags(fs, fp)

	if loaded.Server.Name != "flag-server" {
		t.Errorf("expected name 'flag-server', got %s", loaded.Server.Name)
	}
	if loaded.Sandbox.Timeout != 60*time.Second {
		t.Errorf("unset timeout flag should keep file value 60s, got %v", loaded.Sandbox.Timeout)
	}
	if loaded.Logging.Level != "error" {
		t.Errorf("expected log level 'error', got %s", loaded.Logging.Level)
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"", nil},
		{"a,,b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := splitCSV(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
