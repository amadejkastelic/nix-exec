package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/amadejkastelic/nix-exec/internal/config"
)

type SeatbeltBackend struct {
	config *config.Config
	logger *slog.Logger
}

func (s *SeatbeltBackend) Run(
	ctx context.Context,
	command []string,
	envPath string,
	tmpDir string,
	envVars []string,
	fileMounts []FileMount,
	workspace *WorkspaceMount,
) (*RunResult, error) {
	profile := s.buildSeatbeltProfile(tmpDir, fileMounts, workspace)

	args := []string{"-p", profile, "--"}
	args = append(args, command...)

	s.logger.Debug("running seatbelt sandbox",
		"args", args,
		"env_vars", envVars,
	)

	cmd := exec.CommandContext(ctx, "sandbox-exec", args...)
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

func (s *SeatbeltBackend) buildSeatbeltProfile(
	tmpDir string,
	fileMounts []FileMount,
	workspace *WorkspaceMount,
) string {
	var b strings.Builder

	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n\n")
	b.WriteString("(import \"system.sb\")\n\n")

	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow signal (target self))\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n")
	b.WriteString("(allow ipc-posix-shm)\n\n")

	b.WriteString("(allow file-read*\n")
	for _, p := range systemReadPaths {
		fmt.Fprintf(&b, "    (subpath %q)\n", p)
	}
	fmt.Fprintf(&b, "    (subpath %q)\n", tmpDir)
	if workspace != nil && workspace.Path != "" {
		fmt.Fprintf(&b, "    (subpath %q)\n", workspace.Path)
	}
	for _, fm := range fileMounts {
		fmt.Fprintf(&b, "    (subpath %q)\n", fm.HostPath)
	}
	b.WriteString(")\n\n")

	b.WriteString("(allow file-write*\n")
	fmt.Fprintf(&b, "    (subpath %q)\n", tmpDir)
	if workspace != nil && workspace.Path != "" && workspace.Writable {
		fmt.Fprintf(&b, "    (subpath %q)\n", workspace.Path)
	}
	for _, fm := range fileMounts {
		if fm.Writable {
			fmt.Fprintf(&b, "    (subpath %q)\n", fm.HostPath)
		}
	}
	b.WriteString(")\n\n")

	b.WriteString("(deny network*)\n")

	return b.String()
}

var systemReadPaths = []string{
	"/nix/store",
	"/nix/var",
	"/usr",
	"/bin",
	"/sbin",
	"/System",
	"/dev",
	"/etc",
	"/tmp",
	"/private/tmp",
	"/var",
	"/private/var",
	"/Applications",
	"/Library",
}
