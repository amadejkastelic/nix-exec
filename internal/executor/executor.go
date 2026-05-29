package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/amadejkastelic/nix-exec/internal/config"
	"github.com/amadejkastelic/nix-exec/internal/sandbox"
)

type Executor struct {
	config  *config.Config
	sandbox *sandbox.Sandbox
	cache   *EnvCache
	logger  *slog.Logger
}

type ExecutionResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

func New(cfg *config.Config, sb *sandbox.Sandbox, logger *slog.Logger) *Executor {
	return &Executor{
		config:  cfg,
		sandbox: sb,
		cache:   NewEnvCache(cfg.Executor.CacheDir, logger),
		logger:  logger,
	}
}

func (e *Executor) RunCode(
	ctx context.Context,
	lang, code string,
	packages []string,
	envVars map[string]string,
) (*ExecutionResult, error) {
	interpreter, err := resolveInterpreter(lang)
	if err != nil {
		return nil, err
	}

	for _, pkg := range packages {
		for _, denied := range e.config.Sandbox.PackageDenylist {
			if pkg == denied {
				return nil, fmt.Errorf("package %q is not allowed", pkg)
			}
		}
	}

	allPackages := withInterpreterPackage(lang, packages)

	envPath, err := e.buildEnvironment(ctx, allPackages)
	if err != nil {
		return nil, fmt.Errorf("build environment: %w", err)
	}

	tmpDir, err := os.MkdirTemp(e.config.Executor.TempDir, "nix-exec-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			e.logger.Error("failed removing tmp dir", "err", err.Error())
		}
	}()

	ext := scriptExtension(lang)
	scriptPath := filepath.Join(tmpDir, "script"+ext)
	if err := os.WriteFile(scriptPath, []byte(code), 0o644); err != nil {
		return nil, fmt.Errorf("write script: %w", err)
	}

	command := []string{"/env/bin/" + interpreter, "/tmp/script" + ext}

	sandboxEnv := []string{
		"PATH=/env/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"TERM=dumb",
	}
	for k, v := range envVars {
		sandboxEnv = append(sandboxEnv, fmt.Sprintf("%s=%s", k, v))
	}

	result, err := e.sandbox.Run(ctx, command, envPath, tmpDir, sandboxEnv)
	if err != nil {
		return nil, err
	}

	return &ExecutionResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		TimedOut: result.TimedOut,
	}, nil
}

func (e *Executor) buildEnvironment(ctx context.Context, packages []string) (string, error) {
	if len(packages) == 0 {
		packages = []string{"bash"}
	}

	sort.Strings(packages)
	key := cacheKey(packages)

	if cached, ok := e.cache.Get(key); ok {
		e.logger.Debug("using cached environment", "key", key, "path", cached)
		return cached, nil
	}

	flakeDir, err := os.MkdirTemp(e.config.Executor.TempDir, "nix-exec-flake-*")
	if err != nil {
		return "", fmt.Errorf("create flake dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(flakeDir); err != nil {
			e.logger.Error("failed removing flake dir", "err", err.Error())
		}
	}()

	flakeContent := generateFlake(packages, e.config.Executor.NixpkgsURL)
	if err := os.WriteFile(
		filepath.Join(flakeDir, "flake.nix"),
		[]byte(flakeContent),
		0o644,
	); err != nil {
		return "", fmt.Errorf("write flake: %w", err)
	}

	e.logger.Info("building nix environment", "packages", packages)

	cmd := exec.CommandContext(ctx,
		"nix",
		"--extra-experimental-features", "nix-command flakes",
		"build", "--no-link", "--print-out-paths", ".",
	)
	cmd.Dir = flakeDir

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("nix build failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("nix build failed: %w", err)
	}

	storePath := strings.TrimSpace(string(output))
	e.logger.Info("environment built", "path", storePath)

	e.cache.Set(key, storePath)

	return storePath, nil
}

func generateFlake(packages []string, nixpkgsURL string) string {
	system := nixSystem()

	var pathsBuilder strings.Builder
	for _, pkg := range packages {
		fmt.Fprintf(&pathsBuilder, "      pkgs.%s\n", pkg)
	}

	return fmt.Sprintf(`{
  inputs.nixpkgs.url = "%s";

  outputs = { nixpkgs, ... }: {
    packages.%s.default = nixpkgs.legacyPackages.%s.buildEnv {
      name = "nix-exec-env";
      paths = [
%s      ];
    };
  };
}
`, nixpkgsURL, system, system, pathsBuilder.String())
}

func nixSystem() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64-linux"
	case "arm64":
		return "aarch64-linux"
	case "arm":
		return "armv7l-linux"
	default:
		return "x86_64-linux"
	}
}

func cacheKey(packages []string) string {
	h := sha256.New()
	h.Write([]byte(strings.Join(packages, ",")))
	return hex.EncodeToString(h.Sum(nil))
}

func resolveInterpreter(lang string) (string, error) {
	switch lang {
	case "python":
		return "python3", nil
	case "bash":
		return "bash", nil
	case "node":
		return "node", nil
	default:
		return "", fmt.Errorf("unsupported language: %s", lang)
	}
}

func withInterpreterPackage(lang string, packages []string) []string {
	pkg := map[string]string{
		"python": "python3",
		"bash":   "bash",
		"node":   "nodejs",
	}[lang]

	if pkg == "" {
		return packages
	}

	for _, p := range packages {
		if p == pkg {
			return packages
		}
	}

	return append([]string{pkg}, packages...)
}

func scriptExtension(lang string) string {
	switch lang {
	case "python":
		return ".py"
	case "bash":
		return ".sh"
	case "node":
		return ".js"
	default:
		return ""
	}
}
