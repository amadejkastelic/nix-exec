package config

import (
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Sandbox  SandboxConfig  `yaml:"sandbox"`
	Executor ExecutorConfig `yaml:"executor"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Name string `yaml:"name"`
}

type SandboxConfig struct {
	Timeout         time.Duration `yaml:"timeout"`
	MaxOutputBytes  int64         `yaml:"max_output_bytes"`
	WorkspacePath   string        `yaml:"workspace_path"`
	PackageDenylist []string      `yaml:"package_denylist"`
}

type ExecutorConfig struct {
	CacheDir     string   `yaml:"cache_dir"`
	TempDir      string   `yaml:"temp_dir"`
	NixpkgsURL   string   `yaml:"nixpkgs_url"`
	Substituters []string `yaml:"substituters"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Default() *Config {
	home, _ := os.UserHomeDir()

	return &Config{
		Server: ServerConfig{
			Name: "nix-exec",
		},
		Sandbox: SandboxConfig{
			Timeout:         30 * time.Second,
			MaxOutputBytes:  1 << 20,
			WorkspacePath:   "",
			PackageDenylist: []string{},
		},
		Executor: ExecutorConfig{
			CacheDir:   filepath.Join(home, ".cache", "nix-exec"),
			TempDir:    os.TempDir(),
			NixpkgsURL: "github:NixOS/nixpkgs/nixpkgs-unstable",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	if path == "" {
		path = findConfig()
	}

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func findConfig() string {
	for _, p := range configPaths() {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func configPaths() []string {
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" && home != "" {
		xdg = filepath.Join(home, ".config")
	}

	paths := []string{}
	if xdg != "" {
		paths = append(paths, filepath.Join(xdg, "nix-exec", "config.yaml"))
	}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".nix-exec.yaml"))
	}
	paths = append(paths, "/etc/nix-exec/config.yaml")
	return paths
}

func (c *Config) LogLevel() slog.Level {
	switch c.Logging.Level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type flagPtrs struct {
	name            *string
	timeout         *time.Duration
	maxOutputBytes  *int64
	workspacePath   *string
	packageDenylist *string
	cacheDir        *string
	tempDir         *string
	nixpkgsURL      *string
	substituters    *string
	logLevel        *string
	logFormat       *string
}

func (c *Config) RegisterFlags(fs *flag.FlagSet) *flagPtrs {
	fp := &flagPtrs{
		name: fs.String("name", c.Server.Name, "Server name"),
		timeout: fs.Duration(
			"timeout",
			c.Sandbox.Timeout,
			"Maximum execution time per run (e.g. 30s, 1m)",
		),
		maxOutputBytes: fs.Int64(
			"max-output-bytes",
			c.Sandbox.MaxOutputBytes,
			"Maximum bytes captured from stdout/stderr",
		),
		workspacePath: fs.String(
			"workspace-path",
			c.Sandbox.WorkspacePath,
			"Workspace path to expose read-only inside the sandbox",
		),
		packageDenylist: fs.String(
			"package-denylist",
			"",
			"Comma-separated list of packages that are never allowed",
		),
		cacheDir: fs.String(
			"cache-dir",
			c.Executor.CacheDir,
			"Directory for caching built Nix environments",
		),
		tempDir: fs.String(
			"temp-dir",
			c.Executor.TempDir,
			"Base directory for temporary files",
		),
		nixpkgsURL: fs.String(
			"nixpkgs-url",
			c.Executor.NixpkgsURL,
			"Nixpkgs URL or flake reference",
		),
		substituters: fs.String("substituters", "", "Comma-separated list of Nix substituters"),
		logLevel: fs.String(
			"log-level",
			c.Logging.Level,
			"Log level: debug, info, warn, error",
		),
		logFormat: fs.String("log-format", c.Logging.Format, "Log format: json or text"),
	}
	return fp
}

func (c *Config) ApplyFlags(fs *flag.FlagSet, fp *flagPtrs) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "name":
			c.Server.Name = *fp.name
		case "timeout":
			c.Sandbox.Timeout = *fp.timeout
		case "max-output-bytes":
			c.Sandbox.MaxOutputBytes = *fp.maxOutputBytes
		case "workspace-path":
			c.Sandbox.WorkspacePath = *fp.workspacePath
		case "package-denylist":
			if *fp.packageDenylist != "" {
				c.Sandbox.PackageDenylist = splitCSV(*fp.packageDenylist)
			}
		case "cache-dir":
			c.Executor.CacheDir = *fp.cacheDir
		case "temp-dir":
			c.Executor.TempDir = *fp.tempDir
		case "nixpkgs-url":
			c.Executor.NixpkgsURL = *fp.nixpkgsURL
		case "substituters":
			if *fp.substituters != "" {
				c.Executor.Substituters = splitCSV(*fp.substituters)
			}
		case "log-level":
			c.Logging.Level = *fp.logLevel
		case "log-format":
			c.Logging.Format = *fp.logFormat
		}
	})
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
