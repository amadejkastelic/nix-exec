package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/amadejkastelic/nix-exec/internal/config"
	"github.com/amadejkastelic/nix-exec/internal/executor"
	"github.com/amadejkastelic/nix-exec/internal/sandbox"
)

var version = "dev"

func main() {
	cfg := config.Default()
	fp := cfg.RegisterFlags(flag.CommandLine)
	configPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	if envPath := os.Getenv("NIX_EXEC_CONFIG"); envPath != "" && *configPath == "" {
		*configPath = envPath
	}

	if *configPath != "" {
		loaded, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
			os.Exit(1)
		}
		*cfg = *loaded
	}

	cfg.ApplyFlags(flag.CommandLine, fp)

	cwd, _ := os.Getwd()
	logger := setupLogger(cfg)

	sb := sandbox.New(cfg, logger)
	ex := executor.New(cfg, sb, logger)

	s := server.NewMCPServer(
		cfg.Server.Name,
		version,
		server.WithToolCapabilities(false),
		server.WithRoots(),
	)

	runCodeTool := mcp.NewTool(
		"run_code",
		mcp.WithDescription(
			"Execute code or commands in a secure, sandboxed Nix environment. Use this tool for ALL code execution and shell commands — including bash scripts, one-liners, running system commands, and executing code in any supported language. Each execution runs in an isolated sandbox with namespace-level isolation (PID, IPC, network, mount) and a read-only Nix store. Supports Python, Bash, Node.js, Haskell, Lua, Ruby, Perl, and Octave. Declare Nix packages for any dependencies you need (e.g. 'ripgrep', 'python3Packages.pandas', 'nodejs'). Environments are cached, so repeated runs with the same packages are fast. The current working directory is automatically mounted read-write at /workspace (detected via MCP roots, with cwd as fallback). Use 'files' or 'writable_files' to mount additional host paths under /workspace/files/.",
		),
		mcp.WithString(
			"language",
			mcp.Required(),
			mcp.Description(
				"Language/runtime to use: python, bash, node, haskell, lua, ruby, perl, or octave. Use 'bash' for shell commands, system utilities, and scripting.",
			),
			mcp.Enum("python", "bash", "node", "haskell", "lua", "ruby", "perl", "octave"),
		),
		mcp.WithString(
			"code",
			mcp.Required(),
			mcp.Description(
				"Source code or shell commands to execute. For bash, this can be a single command, a pipeline, or a multi-line script.",
			),
		),
		mcp.WithArray(
			"packages",
			mcp.Description(
				"Nix packages to include in the sandbox environment. Examples: 'ripgrep' for rg, 'python3Packages.pandas' for Python libraries, 'nodejs' for Node.js. Package sets: python3Packages, haskellPackages, lua5_4Packages, rubyPackages, perlPackages, octavePackages.",
			),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithObject(
			"env",
			mcp.Description(
				"Environment variables to set inside the sandbox (key-value pairs, values must be strings).",
			),
		),
		mcp.WithArray("files",
			mcp.Description(
				"Absolute host paths to mount read-only inside the sandbox under /workspace/files/. Use this to give the sandbox access to specific files or directories.",
			),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithArray("writable_files",
			mcp.Description(
				"Absolute host paths to mount read-write inside the sandbox under /workspace/files/. Use this when the code needs to modify or create files on the host.",
			),
			mcp.Items(map[string]any{"type": "string"}),
		),
	)

	s.AddTool(
		runCodeTool,
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			language, err := request.RequireString("language")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			code, err := request.RequireString("code")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			packages := request.GetStringSlice("packages", nil)
			envVars := make(map[string]string)
			if args := request.GetArguments(); args != nil {
				if env, ok := args["env"].(map[string]any); ok {
					for k, v := range env {
						if s, ok := v.(string); ok {
							envVars[k] = s
						}
					}
				}
			}

			var fileMounts []sandbox.FileMount
			for _, p := range request.GetStringSlice("files", nil) {
				fileMounts = append(
					fileMounts,
					sandbox.FileMount{HostPath: p},
				)
			}
			for _, p := range request.GetStringSlice("writable_files", nil) {
				fileMounts = append(
					fileMounts,
					sandbox.FileMount{
						HostPath: p,
						Writable: true,
					},
				)
			}

			var workspace *sandbox.WorkspaceMount
			if cfg.Sandbox.WorkspacePath != "" {
				workspace = &sandbox.WorkspaceMount{
					Path:     cfg.Sandbox.WorkspacePath,
					Writable: false,
				}
			} else {
				workspace = resolveWorkspace(ctx, s, cwd, logger)
			}

			logger.Info("executing code", "language", language, "packages", packages)

			ctx, cancel := context.WithTimeout(ctx, cfg.Sandbox.Timeout)
			defer cancel()

			result, err := ex.RunCode(
				ctx,
				language,
				code,
				packages,
				envVars,
				fileMounts,
				workspace,
			)
			if err != nil {
				logger.Error("execution failed", "error", err)
				return mcp.NewToolResultError(fmt.Sprintf("Execution failed: %v", err)), nil
			}

			return mcp.NewToolResultText(formatOutput(result)), nil
		},
	)

	listLangsTool := mcp.NewTool(
		"list_languages",
		mcp.WithDescription(
			"List all supported programming languages with their interpreter commands and Nix package set prefixes. Use this to discover available languages and how to reference their packages in the 'packages' parameter of run_code.",
		),
	)

	s.AddTool(
		listLangsTool,
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			langs := executor.ListLanguages()
			data, err := json.MarshalIndent(langs, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	logger.Info("starting MCP server", "name", cfg.Server.Name, "version", version)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	stdio := server.NewStdioServer(s)

	errCh := make(chan error, 1)
	go func() {
		errCh <- stdio.Listen(ctx, os.Stdin, os.Stdout)
	}()

	select {
	case <-ctx.Done():
		logger.Info("received shutdown signal, waiting for in-flight requests to finish")
		if err := <-errCh; err != nil {
			logger.Error("server error during shutdown", "error", err)
			os.Exit(1)
		}
		logger.Info("server stopped")
	case err := <-errCh:
		if err != nil {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}
}

func resolveWorkspace(
	ctx context.Context,
	s *server.MCPServer,
	fallback string,
	logger *slog.Logger,
) *sandbox.WorkspaceMount {
	rootsResult, err := s.RequestRoots(ctx, mcp.ListRootsRequest{
		Request: mcp.Request{Method: string(mcp.MethodListRoots)},
	})
	if err != nil {
		logger.Debug("roots not available, using cwd fallback", "error", err)
		if fallback != "" {
			return &sandbox.WorkspaceMount{Path: fallback, Writable: true}
		}
		return nil
	}

	if len(rootsResult.Roots) == 0 {
		logger.Debug("no roots provided, using cwd fallback")
		if fallback != "" {
			return &sandbox.WorkspaceMount{Path: fallback, Writable: true}
		}
		return nil
	}

	rootURI := rootsResult.Roots[0].URI
	parsed, err := url.Parse(rootURI)
	if err != nil {
		logger.Debug("failed to parse root URI, using cwd fallback", "uri", rootURI, "error", err)
		if fallback != "" {
			return &sandbox.WorkspaceMount{Path: fallback, Writable: true}
		}
		return nil
	}

	path := parsed.Path
	if path == "" {
		logger.Debug("root URI has no path, using cwd fallback", "uri", rootURI)
		if fallback != "" {
			return &sandbox.WorkspaceMount{Path: fallback, Writable: true}
		}
		return nil
	}

	logger.Info("using workspace root from MCP client", "path", path)
	return &sandbox.WorkspaceMount{Path: path, Writable: true}
}

func formatOutput(r *executor.ExecutionResult) string {
	var b strings.Builder

	if r.TimedOut {
		b.WriteString("[TIMED OUT]\n")
	}

	fmt.Fprintf(&b, "Exit code: %d\n", r.ExitCode)
	b.WriteString("\n--- stdout ---\n")
	b.WriteString(r.Stdout)
	b.WriteString("\n\n--- stderr ---\n")
	b.WriteString(r.Stderr)

	return b.String()
}

func setupLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel()}

	var handler slog.Handler
	if cfg.Logging.Format == "text" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}
