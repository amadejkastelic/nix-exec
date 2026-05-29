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
	CacheDir   string `yaml:"cache_dir"`
	TempDir    string `yaml:"temp_dir"`
	NixpkgsURL string `yaml:"nixpkgs_url"`
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
