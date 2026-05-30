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
	"regexp"
	"runtime"
	"slices"
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
	fileMounts []sandbox.FileMount,
) (*ExecutionResult, error) {
	interpreter, err := resolveInterpreter(lang)
	if err != nil {
		return nil, err
	}

	for _, pkg := range packages {
		if !validPackageName(pkg) {
			return nil, fmt.Errorf("invalid package name %q", pkg)
		}
		for _, denied := range e.config.Sandbox.PackageDenylist {
			if pkg == denied {
				return nil, fmt.Errorf("package %q is not allowed", pkg)
			}
		}
	}

	allPackages := withInterpreterPackage(lang, packages)

	envPath, err := e.buildEnvironment(ctx, lang, allPackages)
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

	result, err := e.sandbox.Run(
		ctx,
		command,
		envPath,
		tmpDir,
		sandboxEnv,
		fileMounts,
	)
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

func (e *Executor) buildEnvironment(
	ctx context.Context,
	lang string,
	packages []string,
) (string, error) {
	if len(packages) == 0 {
		packages = []string{"bash"}
	}

	sort.Strings(packages)
	key := cacheKey(lang, packages, e.config.Executor.NixpkgsURL)

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

	flakeContent := generateFlake(lang, packages, e.config.Executor.NixpkgsURL)
	if err := os.WriteFile(
		filepath.Join(flakeDir, "flake.nix"),
		[]byte(flakeContent),
		0o644,
	); err != nil {
		return "", fmt.Errorf("write flake: %w", err)
	}

	e.logger.Info("building nix environment", "packages", packages)

	args := []string{
		"--extra-experimental-features", "nix-command flakes",
	}
	if subs := e.config.Executor.Substituters; subs != nil {
		args = append(args, "--option", "substituters", strings.Join(subs, " "))
	}
	args = append(args, "build", "--no-link", "--print-out-paths", ".")

	cmd := exec.CommandContext(ctx, "nix", args...)
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

type langConfig struct {
	interpreter     string
	interpreterPath string
	pkgSetPrefix    string
	extension       string
}

var langConfigs = map[string]langConfig{
	"python": {
		interpreter:     "python3",
		interpreterPath: "python3",
		pkgSetPrefix:    "python3Packages",
		extension:       ".py",
	},
	"bash": {
		interpreter:     "bash",
		interpreterPath: "bash",
		extension:       ".sh",
	},
	"node": {
		interpreter:     "node",
		interpreterPath: "nodejs",
		extension:       ".js",
	},
	"haskell": {
		interpreter:     "runhaskell",
		interpreterPath: "haskellPackages.ghc",
		pkgSetPrefix:    "haskellPackages",
		extension:       ".hs",
	},
	"lua": {
		interpreter:     "lua",
		interpreterPath: "lua5_4",
		pkgSetPrefix:    "lua5_4Packages",
		extension:       ".lua",
	},
	"ruby": {
		interpreter:     "ruby",
		interpreterPath: "ruby",
		pkgSetPrefix:    "rubyPackages",
		extension:       ".rb",
	},
	"perl": {
		interpreter:     "perl",
		interpreterPath: "perl5",
		pkgSetPrefix:    "perlPackages",
		extension:       ".pl",
	},
	"octave": {
		interpreter:     "octave",
		interpreterPath: "octave",
		pkgSetPrefix:    "octavePackages",
		extension:       ".m",
	},
}

func generateFlake(lang string, packages []string, nixpkgsURL string) string {
	system := nixSystem()
	cfg, ok := langConfigs[lang]
	if !ok {
		return generateFlakeDefault(system, packages, nixpkgsURL)
	}

	if cfg.pkgSetPrefix == "" {
		return generateFlakeDefault(system, packages, nixpkgsURL)
	}

	var langPkgs []string
	var otherPkgs []string

	for _, pkg := range packages {
		if pkg == cfg.interpreterPath {
			// handled by withPackages below
		} else if after, found := strings.CutPrefix(pkg, cfg.pkgSetPrefix+"."); found {
			langPkgs = append(langPkgs, after)
		} else {
			otherPkgs = append(otherPkgs, pkg)
		}
	}

	var pathsBuilder strings.Builder

	if len(langPkgs) > 0 {
		fmt.Fprintf(&pathsBuilder, "      (pkgs.%s.withPackages (ps: [\n", cfg.interpreterPath)
		for _, p := range langPkgs {
			fmt.Fprintf(&pathsBuilder, "        ps.%s\n", p)
		}
		fmt.Fprintf(&pathsBuilder, "      ]))\n")
	} else {
		fmt.Fprintf(&pathsBuilder, "      pkgs.%s\n", cfg.interpreterPath)
	}

	for _, pkg := range otherPkgs {
		fmt.Fprintf(&pathsBuilder, "      pkgs.%s\n", pkg)
	}

	return formatFlake(nixpkgsURL, system, pathsBuilder.String())
}

func generateFlakeDefault(system string, packages []string, nixpkgsURL string) string {
	var pathsBuilder strings.Builder
	for _, pkg := range packages {
		fmt.Fprintf(&pathsBuilder, "      pkgs.%s\n", pkg)
	}
	return formatFlake(nixpkgsURL, system, pathsBuilder.String())
}

func formatFlake(nixpkgsURL, system, paths string) string {
	return fmt.Sprintf(`{
  inputs.nixpkgs.url = "%s";

  outputs = { nixpkgs, ... }:
    let pkgs = nixpkgs.legacyPackages.%s; in {
    packages.%s.default = pkgs.buildEnv {
      name = "nix-exec-env";
      paths = [
%s      ];
    };
  };
}
`, nixpkgsURL, system, system, paths)
}

func nixSystem() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "x86_64-linux"
	case "linux/arm64":
		return "aarch64-linux"
	case "linux/arm":
		return "armv7l-linux"
	case "darwin/amd64":
		return "x86_64-darwin"
	case "darwin/arm64":
		return "aarch64-darwin"
	default:
		return "x86_64-linux"
	}
}

var packageNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func validPackageName(pkg string) bool {
	return packageNameRe.MatchString(pkg)
}

func cacheKey(lang string, packages []string, nixpkgsURL string) string {
	h := sha256.New()
	h.Write([]byte(lang + ":" + nixpkgsURL + ":" + strings.Join(packages, ",")))
	return hex.EncodeToString(h.Sum(nil))
}

func resolveInterpreter(lang string) (string, error) {
	cfg, ok := langConfigs[lang]
	if !ok {
		return "", fmt.Errorf("unsupported language: %s", lang)
	}
	return cfg.interpreter, nil
}

func withInterpreterPackage(lang string, packages []string) []string {
	cfg, ok := langConfigs[lang]
	if !ok {
		return packages
	}

	pkg := cfg.interpreterPath
	if slices.Contains(packages, pkg) {
		return packages
	}

	return append([]string{pkg}, packages...)
}

func scriptExtension(lang string) string {
	cfg, ok := langConfigs[lang]
	if !ok {
		return ""
	}
	return cfg.extension
}
