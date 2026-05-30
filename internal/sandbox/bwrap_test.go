package sandbox

import (
	"log/slog"
	"testing"

	"github.com/amadejkastelic/nix-exec/internal/config"
)

func TestBuildBwrapArgs(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	b := &BwrapBackend{config: cfg, logger: logger}

	args := b.buildBwrapArgs(
		[]string{"/env/bin/bash", "/tmp/script.sh"},
		"/nix/store/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-env",
		"/tmp/nix-exec-test",
		nil,
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

	capDropFound := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--cap-drop" && args[i+1] == "ALL" {
			capDropFound = true
			break
		}
	}
	if !capDropFound {
		t.Error("expected --cap-drop ALL in args")
	}

	last := args[len(args)-1]
	if last != "/tmp/script.sh" {
		t.Errorf("expected last arg /tmp/script.sh, got %s", last)
	}
}

func TestBuildBwrapArgsWithWorkspace(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	b := &BwrapBackend{config: cfg, logger: logger}

	args := b.buildBwrapArgs(
		[]string{"/env/bin/bash", "/tmp/script.sh"},
		"/nix/store/abc-env",
		"/tmp/sandbox",
		nil,
		&WorkspaceMount{Path: "/home/user/project", Writable: true},
	)

	found := false
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--bind" && args[i+1] == "/home/user/project" &&
			args[i+2] == "/workspace" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected workspace bind mount not found")
	}
}

func TestBuildBwrapArgsWithWorkspaceReadOnly(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	b := &BwrapBackend{config: cfg, logger: logger}

	args := b.buildBwrapArgs(
		[]string{"/env/bin/bash", "/tmp/script.sh"},
		"/nix/store/abc-env",
		"/tmp/sandbox",
		nil,
		&WorkspaceMount{Path: "/home/user/project", Writable: false},
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
		t.Error("expected read-only workspace mount not found")
	}
}

func TestBuildBwrapArgsWithFileMounts(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	b := &BwrapBackend{config: cfg, logger: logger}

	mounts := []FileMount{
		{HostPath: "/host/data.csv", Writable: false},
		{HostPath: "/host/output", Writable: true},
	}

	args := b.buildBwrapArgs(
		[]string{"/env/bin/bash", "/tmp/script.sh"},
		"/nix/store/abc-env",
		"/tmp/sandbox",
		mounts,
		nil,
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
