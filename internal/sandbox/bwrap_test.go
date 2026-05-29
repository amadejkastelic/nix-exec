package sandbox

import (
	"log/slog"
	"testing"

	"github.com/amadejkastelic/nix-exec/internal/config"
)

func TestBuildBwrapArgs(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	sb := New(cfg, logger)

	args := sb.buildBwrapArgs(
		[]string{"/env/bin/bash", "/tmp/script.sh"},
		"/nix/store/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-env",
		"/tmp/nix-exec-test",
		nil,
	)

	expectedBinds := []struct {
		src, dst string
	}{
		{"/nix/store", "/nix/store"},
		{"/nix/store/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-env", "/env"},
		{"/tmp/nix-exec-test", "/tmp"},
	}

	for _, bind := range expectedBinds {
		found := false
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "--ro-bind" && args[i+1] == bind.src && args[i+2] == bind.dst {
				found = true
				break
			}
			if args[i] == "--bind" && args[i+1] == bind.src && args[i+2] == bind.dst {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected bind %s -> %s not found in args", bind.src, bind.dst)
		}
	}

	if args[0] != "--unshare-all" {
		t.Errorf("expected first arg --unshare-all, got %s", args[0])
	}

	last := args[len(args)-1]
	if last != "/tmp/script.sh" {
		t.Errorf("expected last arg /tmp/script.sh, got %s", last)
	}
}

func TestBuildBwrapArgsWithWorkspace(t *testing.T) {
	cfg := config.Default()
	cfg.Sandbox.WorkspacePath = "/home/user/project"
	logger := slog.Default()
	sb := New(cfg, logger)

	args := sb.buildBwrapArgs(
		[]string{"/env/bin/bash", "/tmp/script.sh"},
		"/nix/store/abc-env",
		"/tmp/sandbox",
		nil,
	)

	found := false
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--ro-bind" && args[i+1] == "/home/user/project" &&
			args[i+2] == "/workspace" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected workspace bind mount not found")
	}
}

func TestBuildBwrapArgsWithFileMounts(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	sb := New(cfg, logger)

	mounts := []FileMount{
		{HostPath: "/host/data.csv", Writable: false},
		{HostPath: "/host/output", Writable: true},
	}

	args := sb.buildBwrapArgs(
		[]string{"/env/bin/bash", "/tmp/script.sh"},
		"/nix/store/abc-env",
		"/tmp/sandbox",
		mounts,
	)

	roFound := false
	rwFound := false
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--ro-bind" && args[i+1] == "/host/data.csv" &&
			args[i+2] == "/workspace/files/data.csv" {
			roFound = true
		}
		if args[i] == "--bind" && args[i+1] == "/host/output" &&
			args[i+2] == "/workspace/files/output" {
			rwFound = true
		}
	}
	if !roFound {
		t.Error("expected read-only file mount not found")
	}
	if !rwFound {
		t.Error("expected writable file mount not found")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxBytes int64
		want     string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello\n[OUTPUT TRUNCATED]"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxBytes)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxBytes, got, tt.want)
		}
	}
}
