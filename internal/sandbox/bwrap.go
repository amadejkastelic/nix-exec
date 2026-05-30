package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"unicode/utf8"

	"github.com/amadejkastelic/nix-exec/internal/config"
)

type FileMount struct {
	HostPath string
	Writable bool
}

type Sandbox struct {
	config *config.Config
	logger *slog.Logger
}

type RunResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

func New(cfg *config.Config, logger *slog.Logger) *Sandbox {
	return &Sandbox{
		config: cfg,
		logger: logger,
	}
}

func (s *Sandbox) Run(
	ctx context.Context,
	command []string,
	envPath string,
	tmpDir string,
	envVars []string,
	fileMounts []FileMount,
) (*RunResult, error) {
	args := s.buildBwrapArgs(command, envPath, tmpDir, fileMounts)

	s.logger.Debug("running sandboxed command",
		"args", args,
		"env_vars", envVars,
	)

	cmd := exec.CommandContext(ctx, "bwrap", args...)
	cmd.Env = envVars

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &RunResult{
		Stdout:   truncate(stdout.String(), s.config.Sandbox.MaxOutputBytes),
		Stderr:   truncate(stderr.String(), s.config.Sandbox.MaxOutputBytes),
		ExitCode: 0,
		TimedOut: false,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.Stderr += "\n[TIMEOUT: execution exceeded time limit]"
			result.ExitCode = -1
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("sandbox execution failed: %w", err)
		}
	}

	return result, nil
}

func (s *Sandbox) buildBwrapArgs(
	command []string,
	envPath string,
	tmpDir string,
	fileMounts []FileMount,
) []string {
	args := []string{
		"--unshare-all",
		"--die-with-parent",
		"--cap-drop", "ALL",
		"--ro-bind", "/nix/store", "/nix/store",
		"--ro-bind", envPath, "/env",
		"--bind", tmpDir, "/tmp",
		"--dev", "/dev",
		"--proc", "/proc",
		"--dir", "/workspace",
		"--dir", "/workspace/files",
	}

	if s.config.Sandbox.WorkspacePath != "" {
		args = append(args, "--ro-bind", s.config.Sandbox.WorkspacePath, "/workspace")
	}

	for _, fm := range fileMounts {
		dest := filepath.Join("/workspace/files", filepath.Base(fm.HostPath))
		if fm.Writable {
			args = append(args, "--bind", fm.HostPath, dest)
		} else {
			args = append(args, "--ro-bind", fm.HostPath, dest)
		}
	}

	args = append(args, "--")
	args = append(args, command...)

	return args
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
