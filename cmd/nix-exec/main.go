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
	ex := executor.New(cfg, sb, logger)

	s := server.NewMCPServer(
		cfg.Server.Name,
		cfg.Server.Version,
		server.WithToolCapabilities(false),
	)

	runCodeTool := mcp.NewTool("run_code",
		mcp.WithDescription("Execute code in a secure, sandboxed Nix environment. Supports Python, Bash, and Node.js. Specify packages for dependencies."),
		mcp.WithString("language",
			mcp.Required(),
			mcp.Description("Programming language to execute"),
			mcp.Enum("python", "bash", "node"),
		),
		mcp.WithString("code",
			mcp.Required(),
			mcp.Description("Source code to execute"),
		),
		mcp.WithArray("packages",
			mcp.Description("Nix packages to include in the environment (e.g. 'ripgrep', 'python3Packages.pandas')"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithObject("env",
			mcp.Description("Environment variables to set in the sandbox"),
		),
	)

	s.AddTool(runCodeTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		logger.Info("executing code", "language", language, "packages", packages)

		result, err := ex.RunCode(ctx, language, code, packages, envVars)
		if err != nil {
			logger.Error("execution failed", "error", err)
			return mcp.NewToolResultError(fmt.Sprintf("Execution failed: %v", err)), nil
		}

		return mcp.NewToolResultText(formatOutput(result)), nil
	})

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
