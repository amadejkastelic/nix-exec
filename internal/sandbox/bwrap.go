package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/amadejkastelic/nix-exec/internal/config"
)

type BwrapBackend struct {
	config *config.Config
	logger *slog.Logger
}

func (b *BwrapBackend) Run(
	ctx context.Context,
	command []string,
	envPath string,
	tmpDir string,
	envVars []string,
	fileMounts []FileMount,
	workspace *WorkspaceMount,
	gpu GPUVendor,
) (*RunResult, error) {
	args, gpuEnv, err := b.buildBwrapArgs(command, envPath, tmpDir, fileMounts, workspace, gpu)
	if err != nil {
		return nil, fmt.Errorf("build sandbox args: %w", err)
	}

	finalEnv := envVars
	for k, v := range gpuEnv {
		finalEnv = append(finalEnv, k+"="+v)
	}

	b.logger.Debug("running sandboxed command",
		"args", args,
		"env_vars", finalEnv,
	)

	cmd := exec.CommandContext(ctx, "bwrap", args...)
	cmd.Env = finalEnv

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	result := &RunResult{
		Stdout:   truncate(stdout.String(), b.config.Sandbox.MaxOutputBytes),
		Stderr:   truncate(stderr.String(), b.config.Sandbox.MaxOutputBytes),
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

func (b *BwrapBackend) buildBwrapArgs(
	command []string,
	envPath string,
	tmpDir string,
	fileMounts []FileMount,
	workspace *WorkspaceMount,
	gpu GPUVendor,
) ([]string, map[string]string, error) {
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

	if workspace != nil && workspace.Path != "" {
		if workspace.Writable {
			args = append(args, "--bind", workspace.Path, "/workspace")
		} else {
			args = append(args, "--ro-bind", workspace.Path, "/workspace")
		}
	}

	for _, fm := range fileMounts {
		dest := filepath.Join("/workspace/files", filepath.Base(fm.HostPath))
		if fm.Writable {
			args = append(args, "--bind", fm.HostPath, dest)
		} else {
			args = append(args, "--ro-bind", fm.HostPath, dest)
		}
	}

	gpuEnv := make(map[string]string)
	if gpu != GPUNone {
		resolved, err := ResolveGPU(gpu)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve gpu: %w", err)
		}

		paths, err := GetGPUDriverPaths(resolved)
		if err != nil {
			return nil, nil, fmt.Errorf("gpu driver paths: %w", err)
		}

		driNeeded := false
		for _, dev := range paths.Devices {
			if strings.HasPrefix(dev, "/dev/dri/") {
				driNeeded = true
			}
		}
		if driNeeded {
			args = append(args, "--dir", "/dev/dri")
		}

		for _, dev := range paths.Devices {
			args = append(args, "--dev-bind", dev, dev)
		}

		for _, dir := range paths.LibDirs {
			mountPoint := dir
			args = append(args, "--ro-bind", dir, mountPoint)
		}

		if _, err := os.Stat("/sys"); err == nil {
			args = append(args, "--ro-bind", "/sys", "/sys")
		}

		for k, v := range paths.EnvVars {
			gpuEnv[k] = v
		}
	}

	args = append(args, "--")
	args = append(args, command...)

	return args, gpuEnv, nil
}
