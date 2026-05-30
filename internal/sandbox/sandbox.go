package sandbox

import (
	"context"
	"log/slog"
	"runtime"
	"unicode/utf8"

	"github.com/amadejkastelic/nix-exec/internal/config"
)

type FileMount struct {
	HostPath string
	Writable bool
}

type WorkspaceMount struct {
	Path     string
	Writable bool
}

type RunResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

type Backend interface {
	Run(
		ctx context.Context,
		command []string,
		envPath string,
		tmpDir string,
		envVars []string,
		fileMounts []FileMount,
		workspace *WorkspaceMount,
	) (*RunResult, error)
}

type Sandbox struct {
	config  *config.Config
	logger  *slog.Logger
	backend Backend
}

func New(cfg *config.Config, logger *slog.Logger) *Sandbox {
	var backend Backend
	switch runtime.GOOS {
	case "darwin":
		backend = &SeatbeltBackend{config: cfg, logger: logger}
	default:
		backend = &BwrapBackend{config: cfg, logger: logger}
	}
	return &Sandbox{
		config:  cfg,
		logger:  logger,
		backend: backend,
	}
}

func (s *Sandbox) Run(
	ctx context.Context,
	command []string,
	envPath string,
	tmpDir string,
	envVars []string,
	fileMounts []FileMount,
	workspace *WorkspaceMount,
) (*RunResult, error) {
	return s.backend.Run(ctx, command, envPath, tmpDir, envVars, fileMounts, workspace)
}

func truncate(s string, maxBytes int64) string {
	if int64(len(s)) <= maxBytes {
		return s
	}
	for int64(len(s)) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s + "\n[OUTPUT TRUNCATED]"
}
