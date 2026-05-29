Project Plan: Nix GPU Sandbox (MCP Executor)
1. Project Objective

Build a production-grade, high-performance MCP Server in Go that provides secure, sandboxed code execution for AI agents.

Differentiators vs Competition:

    Language: Go (simpler concurrency model, easier distribution as a single binary).
    GPU Support (Roadmap): Designed from the ground up to eventually support GPU passthrough (CUDA/ROCm) for local inference workloads.
    Architecture: Strictly modular (Protocol / Execution / Sandbox layers).

2. Architecture

    Language: Go (1.25+)
    Interface: MCP over Standard I/O (stdio)
    Isolation Engine: bwrap (Bubblewrap)
    Dependency Engine: Nix Flakes

Project Structure

.
├── cmd/
│   └── main.go           # Entry point
├── internal/
│   ├── mcp/              # MCP Protocol handling (JSON-RPC)
│   ├── executor/         # Nix integration & script execution
│   └── sandbox/          # Bubblewrap configuration & security
├── flake.nix             # Build definition & runtime deps
└── README.md
text





## 3. Development Roadmap

### Phase 1: The Core (MVP)
**Goal:** A working MCP tool that runs CPU code safely.

1.  **MCP Protocol Layer (`internal/mcp`)**
    - Implement `initialize` handshake.
    - Implement `tools/list` (Expose `run_code`).
    - Implement `tools/call` (Execute logic).
    - *Note:* Keep this pure JSON-RPC, no business logic.

2.  **Nix Execution Layer (`internal/executor`)**
    - Function `BuildEnvironment(packages []string)`: Generates a temporary `flake.nix` or uses `nix-shell` command to resolve dependencies.
    - Function `RunCommand(env, script)`: Executes the script inside the Nix environment.

3.  **Sandbox Layer (`internal/sandbox`)**
    - Implement `GenerateBwrapCommand()`.
    - **Default Restrictive Profile:**
      - Unshare PID, Network, IPC, UTS namespaces.
      - Read-only bind for `/nix/store`.
      - Read-only bind for `/project` (workspace).
      - Writable temp directory (`/tmp`) only.
    - Handle Linux namespace setup errors gracefully.

### Phase 2: Enhanced DX
**Goal:** Make it "just work" for developers.

1.  **Session Management:**
    - Allow persistent sessions (keep a Python REPL open) using Unix sockets or named pipes if needed, or simply fast cold-starts.
2.  **Timeout Handling:**
    - Enforce strict timeouts (e.g., 30s) in Go `context` to prevent agent hangs.
3.  **Output Streaming:**
    - Stream `stdout`/`stderr` back to the model in real-time chunks (if supported by client) or return full output on exit.

### Phase 3: The "Killer Feature" (GPU Support)
**Goal:** Allow agents to run AI workloads (PyTorch/TensorFlow) securely.

1.  **Device Detection:**
    - Detect host GPU devices (`/dev/nvidiaX`, `/dev/dri`).
    - Detect host driver versions (must match Nix packages).
2.  **Sandbox Extension:**
    - Modify `bwrap` command to `--dev-bind /dev/nvidia0 /dev/nvidia0`.
    - Bind `/dev/dri` for AMD/Intel.
3.  **Nix CUDA Integration:**
    - Auto-inject `cudatoolkit` or `rocmPackages` into the Nix shell when GPU is requested.
    - Solve the "Driver vs. Library version mismatch" hell common in Nix+GPU setups.

## 4. Technical Implementation Details

### The `run_code` Tool Schema
```json
{
  "name": "run_code",
  "description": "Executes code in a secure, isolated Nix environment. Supports Python, Bash, and Node. Use this for calculations, file processing, or testing.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "language": { "type": "string", "enum": ["python", "bash", "node"] },
      "code": { "type": "string" },
      "packages": {
        "type": "array",
        "items": { "type": "string" },
        "description": "List of Nix packages (e.g., 'ripgrep', 'python3Packages.pandas')"
      },
      "gpu": {
        "type": "boolean",
        "description": "Set true to enable GPU access (if available)"
      }
    },
    "required": ["language", "code"]
  }
}


Bubblewrap Security Profile (Go pseudo-code)
go




func generateBwrapCommand(scriptPath string, nixStorePath string) []string {
    return []string{
        "bwrap",
        "--unshare-all",          // Isolate network/pid/etc
        "--die-with-parent",      // Cleanup if we crash
        "--ro-bind", "/nix/store", "/nix/store", // Read-only Nix
        "--ro-bind", nixStorePath, "/env",       // The specific env
        "--bind", "/tmp/sandbox-xyz", "/tmp",    // Writable temp
        "--dev", "/dev",          // Minimal /dev
        // Future GPU Hooks will add --dev-bind /dev/nvidia0 here
        "--", "/env/bin/interpreter", scriptPath,
    }
}


5. Immediate Next Steps (For the Agent)

    Initialize Go Module: go mod init github.com/user/nix-gpu-sandbox.
    Create internal/mcp/server.go: Set up the basic JSON-RPC decoder on os.Stdin.
    Create internal/sandbox/bubblewrap.go: Write a function that takes a command string and wraps it in bwrap --unshare-all ....
    Wire it up: Make the MCP tools/call handler trigger the sandbox function.

6. Why this wins

     Simplicity: Go's os/exec makes handling subprocesses cleaner than Rust's tokio::process for a simple daemon.
     Future-proofing: The Phase 3 GPU support is highly sought after but poorly implemented elsewhere. If you nail this, you become the standard.
