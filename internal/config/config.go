package config

import (
	"log/slog"
	"os"
	"path/filepath"
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
