package sandbox

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/amadejkastelic/nix-exec/internal/config"
)

func TestSeatbeltProfileBasic(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	s := &SeatbeltBackend{config: cfg, logger: logger}

	profile := s.buildSeatbeltProfile(
		"/tmp/nix-exec-test",
		nil,
		nil,
	)

	if !strings.Contains(profile, "(version 1)") {
		t.Error("missing version header")
	}
	if !strings.Contains(profile, "(deny default)") {
		t.Error("missing deny default")
	}
	if !strings.Contains(profile, `(subpath "/tmp/nix-exec-test")`) {
		t.Error("missing tmpDir read path")
	}
	if !strings.Contains(profile, "(deny network*)") {
		t.Error("missing network deny")
	}
	if !strings.Contains(profile, "(allow process-exec)") {
		t.Error("missing process-exec allow")
	}
}

func TestSeatbeltProfileWithWorkspace(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	s := &SeatbeltBackend{config: cfg, logger: logger}

	profile := s.buildSeatbeltProfile(
		"/tmp/nix-exec-test",
		nil,
		&WorkspaceMount{Path: "/Users/me/project", Writable: true},
	)

	if !strings.Contains(profile, `(subpath "/Users/me/project")`) {
		t.Error("missing workspace read path")
	}

	writeSection := profile[strings.LastIndex(profile, "(allow file-write*"):]
	if !strings.Contains(writeSection, `(subpath "/Users/me/project")`) {
		t.Error("missing workspace write path")
	}
}

func TestSeatbeltProfileWorkspaceReadOnly(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	s := &SeatbeltBackend{config: cfg, logger: logger}

	profile := s.buildSeatbeltProfile(
		"/tmp/nix-exec-test",
		nil,
		&WorkspaceMount{Path: "/Users/me/project", Writable: false},
	)

	if !strings.Contains(profile, `(subpath "/Users/me/project")`) {
		t.Error("missing workspace read path")
	}

	writeSection := profile[strings.LastIndex(profile, "(allow file-write*"):]
	if strings.Contains(writeSection, `/Users/me/project`) {
		t.Error("workspace should not appear in write section when not writable")
	}
}

func TestSeatbeltProfileWithFileMounts(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	s := &SeatbeltBackend{config: cfg, logger: logger}

	mounts := []FileMount{
		{HostPath: "/Users/me/data.csv", Writable: false},
		{HostPath: "/Users/me/output", Writable: true},
	}

	profile := s.buildSeatbeltProfile(
		"/tmp/nix-exec-test",
		mounts,
		nil,
	)

	if !strings.Contains(profile, `(subpath "/Users/me/data.csv")`) {
		t.Error("missing read-only file mount")
	}
	if !strings.Contains(profile, `(subpath "/Users/me/output")`) {
		t.Error("missing writable file mount in read section")
	}

	writeSection := profile[strings.LastIndex(profile, "(allow file-write*"):]
	if strings.Contains(writeSection, `/Users/me/data.csv`) {
		t.Error("read-only mount should not appear in write section")
	}
	if !strings.Contains(writeSection, `/Users/me/output`) {
		t.Error("writable mount should appear in write section")
	}
}
