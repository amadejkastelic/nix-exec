# nix-exec

An MCP server for secure, sandboxed code execution using [Nix Flakes](https://nix.dev/concepts/flakes) for dependency management and OS-native sandboxing for isolation.

Designed for AI agents that need to run arbitrary code safely - each execution gets a fresh, minimal sandbox with only the declared dependencies available.

## Features

- **`run_code` tool** - execute Python, Bash, Node.js, Haskell, Lua, Ruby, Perl, or Octave code from any MCP client
- **Reproducible environments** - Nix flake-based dependency resolution with built-in caching
- **Sandboxed execution** - Bubblewrap on Linux (namespace isolation), Seatbelt on macOS (policy-based sandboxing via `sandbox-exec`)
- **Auto workspace mount** - project directory detected via MCP Roots and mounted read-write inside the sandbox
- **Configurable** - YAML config file, environment variables, and CLI flags with sensible defaults
- **NixOS module** - declarative deployment via `programs.nix-exec` (Linux only)

## Usage

### As an MCP server

Configure your MCP client (e.g. Claude Desktop, opencode) to run:

```json
{
  "mcpServers": {
    "nix-exec": {
      "command": "nix-exec",
      "args": ["-log-level", "debug", "-timeout", "60s"]
    }
  }
}
```

A config file is optional - sensible defaults are used if none is found. Configuration is loaded in this order (later sources override earlier ones):

1. **Built-in defaults**
2. **Config file** - set via `-config` flag or `NIX_EXEC_CONFIG` env var. When neither is set, the following locations are searched:
   - `$XDG_CONFIG_HOME/nix-exec/config.yaml`
   - `~/.nix-exec.yaml`
   - `/etc/nix-exec/config.yaml`
3. **CLI flags** - override all other sources

### The `run_code` tool

| Parameter        | Type     | Required | Description                                                              |
|------------------|----------|----------|--------------------------------------------------------------------------|
| `language`       | string   | yes      | `python`, `bash`, `node`, `haskell`, `lua`, `ruby`, `perl`, or `octave` |
| `code`           | string   | yes      | Source code to execute                                                   |
| `packages`       | string[] | no       | Nix packages to include (e.g. `"ripgrep"`, `"python3Packages.pandas"`)  |
| `env`            | object   | no       | Environment variables to set in the sandbox                              |
| `files`          | string[] | no       | Host paths to mount read-only inside the sandbox                         |
| `writable_files` | string[] | no       | Host paths to mount read-write inside the sandbox                        |

The project directory is automatically detected via MCP Roots and mounted read-write inside the sandbox. On Linux it appears at `/workspace`; on macOS it is accessible at its real path.

Example - Python with pandas:

```json
{
  "language": "python",
  "code": "import pandas as pd; print(pd.__version__)",
  "packages": ["python3Packages.pandas"]
}
```

### Supported Languages

| Language   | `language`  | Interpreter       | Package set prefix     | Example package              |
|------------|-------------|-------------------|------------------------|------------------------------|
| Python     | `python`    | `python3`         | `python3Packages`      | `python3Packages.pandas`    |
| Bash       | `bash`      | `bash`            | *(none)*               | `ripgrep`                   |
| Node.js    | `node`      | `node`            | *(none)*               | `nodejs`                    |
| Haskell    | `haskell`   | `runhaskell`      | `haskellPackages`      | `haskellPackages.lens`      |
| Lua        | `lua`       | `lua`             | `lua5_4Packages`       | `lua5_4Packages.dkjson`     |
| Ruby       | `ruby`      | `ruby`            | `rubyPackages`         | `rubyPackages.pry`          |
| Perl       | `perl`      | `perl`            | `perlPackages`         | `perlPackages.JSON`         |
| Octave     | `octave`    | `octave`          | `octavePackages`       | `octavePackages.signal`     |

Languages with a package set prefix use `{interpreter}.withPackages(...)` internally, so libraries are properly registered with the runtime (e.g. Python's `site-packages`, GHC's package database, Lua's `LUA_PATH`).

See [`config.example.yaml`](config.example.yaml) for all options with defaults.

#### CLI Flags

All settings can also be set via command-line flags, which take precedence over the config file:

| Flag                      | Default                                 | Description                                  |
|---------------------------|-----------------------------------------|----------------------------------------------|
| `-config`                 | `""`                                    | Path to config file                          |
| `-name`                   | `nix-exec`                              | Server name                                  |
| `-timeout`                | `30s`                                   | Max execution time per run                   |
| `-max-output-bytes`       | `1048576`                               | Max stdout/stderr captured (bytes)           |
| `-workspace-path`         | `""`                                    | Host path mounted read-only at `/workspace`  |
| `-package-denylist`       | `""`                                    | Comma-separated list of denied packages      |
| `-cache-dir`              | `~/.cache/nix-exec`                     | Cached Nix environment store                 |
| `-temp-dir`               | `/tmp`                                  | Base directory for temporary files           |
| `-nixpkgs-url`            | `github:NixOS/nixpkgs/nixpkgs-unstable` | Nixpkgs flake URL for resolving packages     |
| `-substituters`           | `""`                                    | Comma-separated list of Nix substituters     |
| `-log-level`              | `info`                                  | Log level: `debug`, `info`, `warn`, `error`  |
| `-log-format`             | `json`                                  | Log format: `json` or `text`                 |

#### Config File Settings

| Setting                     | Default                                 | Description                                  |
|-----------------------------|-----------------------------------------|----------------------------------------------|
| `server.name`               | `nix-exec`                              | Server name                                  |
| `sandbox.timeout`           | `30s`                                   | Max execution time per run                   |
| `sandbox.max_output_bytes`  | `1048576`                               | Max stdout/stderr captured (bytes)           |
| `sandbox.workspace_path`    | `""`                                    | Host path mounted read-only at `/workspace`  |
| `sandbox.package_denylist`  | `[]`                                    | Nix packages that are never allowed          |
| `executor.cache_dir`        | `~/.cache/nix-exec`                     | Cached Nix environment store                 |
| `executor.temp_dir`         | `/tmp`                                  | Base directory for temporary files           |
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

> **Note:** On Linux, [Bubblewrap](https://github.com/containers/bubblewrap) and Nix (with flakes) must be available at runtime. On macOS, `sandbox-exec` is built-in and Nix (with flakes) must be installed.

## How it works

1. The executor resolves the language to an interpreter and generates a Nix flake that builds a `buildEnv` with the requested packages.
2. For languages with package sets (Python, Haskell, Lua, Ruby, Perl, Octave), packages matching the set prefix (e.g. `python3Packages.*`, `haskellPackages.*`) are grouped and installed via `{interpreter}.withPackages` so dependencies are properly wired (e.g. into `site-packages`, GHC's package db, Lua's `LUA_PATH`, etc.).
3. The flake is built with `nix build`, and the resulting store path is cached (keyed by language + sorted package list).
4. The sandbox is launched with the built environment:
   - **Linux** - [Bubblewrap](https://github.com/containers/bubblewrap) creates isolated PID/IPC/network/mount namespaces. The environment is mounted at `/env`, the workspace at `/workspace`, and all capabilities are dropped.
   - **macOS** - [Seatbelt](https://developer.apple.com/library/archive/documentation/Security/Conceptual/AppSandboxDesignGuide/AppSandboxInDepth/AppSandboxInDepth.html) (`sandbox-exec`) enforces a deny-by-default policy allowing only reads from system paths and the Nix store, read-write to the workspace and temp directory, and no network access. The environment is accessed at its real Nix store path.
5. Output is captured, truncated to `max_output_bytes`, and returned as MCP tool result text.

## Requirements

- Linux or macOS
- Nix with flakes enabled
- [Bubblewrap](https://github.com/containers/bubblewrap) (Linux only; `sandbox-exec` is built into macOS)

## License

[MIT](LICENSE)
