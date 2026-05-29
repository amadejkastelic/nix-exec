# nix-exec

An MCP server for secure, sandboxed code execution using [Nix Flakes](https://nix.dev/concepts/flakes) for dependency management and [Bubblewrap](https://github.com/containers/bubblewrap) for isolation.

Designed for AI agents that need to run arbitrary code safely — each execution gets a fresh, minimal sandbox with only the declared dependencies available.

## Features

- **`run_code` tool** — execute Python, Bash, or Node.js code from any MCP client
- **Reproducible environments** — Nix flake-based dependency resolution with built-in caching
- **Sandboxed execution** — Bubblewrap isolates every run (separate PID/IPC/net mount namespaces, read-only nix store)
- **Configurable** — YAML config file with sensible defaults
- **NixOS module** — declarative deployment via `programs.nix-exec`

## Usage

### As an MCP server

Configure your MCP client (e.g. Claude Desktop, opencode) to run:

```json
{
  "mcpServers": {
    "nix-exec": {
      "command": "nix-exec"
    }
  }
}
```

A config file is optional — sensible defaults are used if none is found. When no `-config` flag or `NIX_EXEC_CONFIG` env var is set, the following locations are searched in order:

1. `$XDG_CONFIG_HOME/nix-exec/config.yaml`
2. `~/.nix-exec.yaml`
3. `/etc/nix-exec/config.yaml`

### The `run_code` tool

| Parameter  | Type     | Required | Description                                                              |
|------------|----------|----------|--------------------------------------------------------------------------|
| `language` | string   | yes      | `python`, `bash`, or `node`                                              |
| `code`     | string   | yes      | Source code to execute                                                   |
| `packages` | string[] | no       | Nix packages to include (e.g. `"ripgrep"`, `"python3Packages.pandas"`)  |
| `env`      | object   | no       | Environment variables to set in the sandbox                              |

Example — Python with pandas:

```json
{
  "language": "python",
  "code": "import pandas as pd; print(pd.__version__)",
  "packages": ["python3Packages.pandas"]
}
```

### Configuration

See [`config.example.yaml`](config.example.yaml) for all options with defaults.

| Setting                     | Default                                 | Description                                  |
|-----------------------------|-----------------------------------------|----------------------------------------------|
| `sandbox.timeout`           | `30s`                                   | Max execution time per run                   |
| `sandbox.max_output_bytes`  | `1048576`                               | Max stdout/stderr captured (bytes)           |
| `sandbox.workspace_path`    | `""`                                    | Host path mounted read-only at `/workspace`  |
| `sandbox.package_denylist`  | `[]`                                    | Nix packages that are never allowed          |
| `executor.cache_dir`        | `~/.cache/nix-exec`                     | Cached Nix environment store                 |
| `executor.nixpkgs_url`      | `github:NixOS/nixpkgs/nixpkgs-unstable` | Nixpkgs flake URL for resolving packages     |
| `executor.substituters`     | `null`                                  | Nix substituters (`null` = system defaults)  |
| `logging.level`             | `info`                                  | Log level: `debug`, `info`, `warn`, `error`  |
| `logging.format`            | `json`                                  | Log format: `json` or `text`                 |

## Installing

Add as a flake input:

```nix
{
  inputs = {
    nix-exec.url = "github:amadejkastelic/nix-exec";
  };

  outputs = { nix-exec, ... }: {
    # nix-exec.packages.${system}.default
    # nix-exec.nixosModules.default
  };
}
```

### Cachix

Binary builds are pushed to [cachix.org/amadejkastelic](https://amadejkastelic.cachix.org) on every push. To avoid building from source:

```nix
nix.settings = {
  extra-substituters = [ "https://amadejkastelic.cachix.org" ];
  extra-trusted-public-keys = [
    "amadejkastelic.cachix.org-1:EiQfTbiT0UKsynF4q3nbNYjNH6/l7zuhrNkQTuXmyOs="
  ];
};
```

## NixOS Module

```nix
{
  inputs.nix-exec.url = "github:amadejkastelic/nix-exec";

  outputs = { nix-exec, ... }: {
    nixosConfigurations.my-host = lib.nixosSystem {
      modules = [
        nix-exec.nixosModules.default
        {
          programs.nix-exec = {
            enable = true;
            settings = {
              sandbox.timeout = "60s";
              executor.nixpkgs_url = "github:NixOS/nixpkgs/nixos-25.05";
            };
          };
        }
      ];
    };
  };
}
```

This adds `nix-exec` and `bubblewrap` to `environment.systemPackages`, enables flakes, and generates `/etc/nix-exec/config.yaml`.

## Building

### With Nix

```bash
nix build                        # server binary
nix build .#test                 # integration test binary
nix flake check -L               # lint + unit tests + VM integration tests
nix develop                      # dev shell with pre-commit hooks
```

### With Go

```bash
go build -o nix-exec ./cmd/nix-exec
go test ./...
```

> **Note:** Bubblewrap and Nix (with flakes) must be available on the host at runtime.

## How it works

1. The executor resolves the language to an interpreter and generates a Nix flake that builds a `buildEnv` with the requested packages.
2. For Python packages (`python3Packages.*`), the flake uses `python3.withPackages` so dependencies are properly wired into `site-packages`.
3. The flake is built with `nix build`, and the resulting store path is cached (keyed by language + sorted package list).
4. The sandbox spawns Bubblewrap with the built environment mounted at `/env`, the script in `/tmp`, and full namespace isolation.
5. Output is captured, truncated to `max_output_bytes`, and returned as MCP tool result text.

## Requirements

- Linux (sandboxing uses Bubblewrap)
- Nix with flakes enabled
- [Bubblewrap](https://github.com/containers/bubblewrap)

## License

[MIT](LICENSE)
