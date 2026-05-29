package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/amadejkastelic/nix-exec/internal/config"
	"github.com/amadejkastelic/nix-exec/internal/executor"
	"github.com/amadejkastelic/nix-exec/internal/sandbox"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type RunCodeArgs struct {
	Language string            `json:"language" jsonschema:"required,enum=python,enum=bash,enum=node" jsonschema_description:"Programming language to execute"`
	Code     string            `json:"code" jsonschema:"required" jsonschema_description:"Source code to execute"`
	Packages []string          `json:"packages,omitempty" jsonschema_description:"Nix packages to include in the environment"`
	Env      map[string]string `json:"env,omitempty" jsonschema_description:"Environment variables to set in the sandbox"`
}

func main() {
	configPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	if envPath := os.Getenv("NIX_EXEC_CONFIG"); envPath != "" && *configPath == "" {
		*configPath = envPath
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg)

	sb := sandbox.New(cfg, logger)
	exec := executor.New(cfg, sb, logger)

	s := server.NewMCPServer(
		cfg.Server.Name,
		cfg.Server.Version,
		server.WithToolCapabilities(false),
	)

	runCodeTool := mcp.NewTool("run_code",
		mcp.WithDescription("Execute code in a secure, sandboxed Nix environment. Supports Python, Bash, and Node.js. Specify packages for dependencies."),
		mcp.WithInputSchema[RunCodeArgs](),
	)

	s.AddTool(runCodeTool, mcp.NewTypedToolHandler(func(
		ctx context.Context,
		request mcp.CallToolRequest,
		args RunCodeArgs,
	) (*mcp.CallToolResult, error) {
		logger.Info("executing code", "language", args.Language, "packages", args.Packages)

		result, err := exec.RunCode(ctx, args.Language, args.Code, args.Packages, args.Env)
		if err != nil {
			logger.Error("execution failed", "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("Execution failed: %v", err)), nil
		}

		output := formatOutput(result)
		return mcp.NewToolResultText(output), nil
	}))

	logger.Info("starting MCP server", "name", cfg.Server.Name, "version", cfg.Server.Version)

	if err := server.ServeStdio(s); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
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
