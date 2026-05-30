package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type mcpClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	nextID    int
	rootsPath string
}

type jsonrpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func newMCPClient() (*mcpClient, error) {
	configPath := os.Getenv("NIX_EXEC_TEST_CONFIG")
	if configPath == "" {
		return nil, fmt.Errorf("NIX_EXEC_TEST_CONFIG not set")
	}

	workspacePath, err := os.MkdirTemp("", "nix-exec-workspace-*")
	if err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}

	cmd := exec.Command("nix-exec", "--config", configPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}

	return &mcpClient{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReaderSize(stdoutPipe, 1<<20),
		nextID:    1,
		rootsPath: workspacePath,
	}, nil
}

func (c *mcpClient) close() {
	_ = c.stdin.Close()
	_ = c.cmd.Process.Kill()
	_ = c.cmd.Wait()
	if c.rootsPath != "" {
		_ = os.RemoveAll(c.rootsPath)
	}
}

func (c *mcpClient) send(msg map[string]any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func (c *mcpClient) notify(method string) {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal notify: %v\n", err)
		return
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "write notify: %v\n", err)
	}
}

func (c *mcpClient) recv() jsonrpcMessage {
	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "read response: %v\n", err)
		os.Exit(1)
	}
	var msg jsonrpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal: %v\nraw: %s\n", err, line)
		os.Exit(1)
	}
	return msg
}

func (c *mcpClient) call(method string, params any) jsonrpcMessage {
	id := c.nextID
	c.nextID++
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	if err := c.send(msg); err != nil {
		fmt.Fprintf(os.Stderr, "send: %v\n", err)
		os.Exit(1)
	}
	for {
		resp := c.recv()
		if resp.Method != "" && resp.ID != nil {
			c.handleServerRequest(resp)
			continue
		}
		if resp.ID != nil && *resp.ID == id {
			return resp
		}
	}
}

func (c *mcpClient) handleServerRequest(msg jsonrpcMessage) {
	switch msg.Method {
	case "roots/list":
		roots := []any{}
		if c.rootsPath != "" {
			roots = append(roots, map[string]any{
				"uri":  "file://" + c.rootsPath,
				"name": "test-workspace",
			})
		}
		_ = c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      *msg.ID,
			"result":  map[string]any{"roots": roots},
		})
	default:
		_ = c.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      *msg.ID,
			"error":   map[string]any{"code": -32601, "message": "method not found"},
		})
	}
}

func (c *mcpClient) callTool(name string, arguments map[string]any) jsonrpcMessage {
	return c.call("tools/call", map[string]any{"name": name, "arguments": arguments})
}

type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func parseToolResult(resp jsonrpcMessage) (toolResult, error) {
	if resp.Error != nil {
		return toolResult{}, fmt.Errorf("RPC error: %d %s", resp.Error.Code, resp.Error.Message)
	}
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return toolResult{}, fmt.Errorf("unmarshal: %w", err)
	}
	return result, nil
}

type testCase struct {
	name string
	fn   func(*mcpClient) error
}

func main() {
	client, err := newMCPClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
	defer client.close()

	tests := []testCase{
		{"initialize", testInitialize},
		{"tools_list", testToolsList},
		{"bash_echo", testBashEcho},
		{"bash_caching", testBashCaching},
		{"bash_with_jq", testBashWithJq},
		{"python_execution", testPythonExecution},
		{"python_with_pandas", testPythonWithPandas},
		{"node_execution", testNodeExecution},
		{"haskell_execution", testHaskellExecution},
		{"haskell_with_vector", testHaskellWithVector},
		{"lua_execution", testLuaExecution},
		{"lua_with_dkjson", testLuaWithDkjson},
		{"ruby_execution", testRubyExecution},
		{"ruby_with_pg", testRubyWithPg},
		{"perl_execution", testPerlExecution},
		{"perl_with_json", testPerlWithJSON},
		{"octave_execution", testOctaveExecution},
		{"filesystem_isolation", testFilesystemIsolation},
		{"network_isolation", testNetworkIsolation},
		{"timeout_enforcement", testTimeoutEnforcement},
		{"env_vars", testEnvVars},
		{"unsupported_language", testUnsupportedLanguage},
		{"exit_code_propagation", testExitCodePropagation},
		{"read_only_file_mount", testReadOnlyFileMount},
		{"writable_file_mount", testWritableFileMount},
		{"workspace_root_mount", testWorkspaceRootMount},
	}

	var passed, failed int
	for _, tt := range tests {
		if err := tt.fn(client); err != nil {
			fmt.Printf("FAIL: %s: %s\n", tt.name, err)
			failed++
		} else {
			fmt.Printf("PASS: %s\n", tt.name)
			passed++
		}
	}

	fmt.Printf("\n%d passed, %d failed\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func testInitialize(c *mcpClient) error {
	resp := c.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"roots": map[string]any{"listChanged": true},
		},
		"clientInfo": map[string]any{"name": "test", "version": "1.0.0"},
	})
	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}
	var result struct {
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	if result.ServerInfo.Name != "nix-exec-test" {
		return fmt.Errorf("expected server name 'nix-exec-test', got %q", result.ServerInfo.Name)
	}
	if _, ok := result.Capabilities["tools"]; !ok {
		return fmt.Errorf("missing tools capability")
	}
	c.notify("notifications/initialized")
	return nil
}

func testToolsList(c *mcpClient) error {
	resp := c.call("tools/list", nil)
	if resp.Error != nil {
		return fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}
	var result struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	for _, tool := range result.Tools {
		if tool.Name == "run_code" {
			props, _ := tool.InputSchema["properties"].(map[string]any)
			if _, ok := props["language"]; !ok {
				return fmt.Errorf("missing language property")
			}
			if _, ok := props["code"]; !ok {
				return fmt.Errorf("missing code property")
			}
			return nil
		}
	}
	return fmt.Errorf("run_code tool not found")
}

func testBashEcho(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "echo 'hello from nix-exec'",
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "hello from nix-exec") {
		return fmt.Errorf("missing output in: %s", text)
	}
	if !strings.Contains(text, "Exit code: 0") {
		return fmt.Errorf("wrong exit code in: %s", text)
	}
	return nil
}

func testBashCaching(c *mcpClient) error {
	start := time.Now()
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "echo 'cached call'",
	})
	elapsed := time.Since(start)
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "cached call") {
		return fmt.Errorf("missing output: %s", result.Content[0].Text)
	}
	if elapsed > 10*time.Second {
		return fmt.Errorf("cached call took %v, caching may not work", elapsed)
	}
	return nil
}

func testPythonExecution(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "python",
		"code":     "import sys; print(f'python {sys.version_info.major}.{sys.version_info.minor}')",
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "python 3") {
		return fmt.Errorf("missing python version: %s", text)
	}
	if !strings.Contains(text, "Exit code: 0") {
		return fmt.Errorf("wrong exit code: %s", text)
	}
	return nil
}

func testFilesystemIsolation(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "touch /nix/store/test-write-$$ 2>&1; echo \"exit=$?\"",
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "Permission denied") &&
		!strings.Contains(text, "Read-only") &&
		!strings.Contains(text, "exit=1") {
		return fmt.Errorf("filesystem not isolated: %s", text)
	}
	return nil
}

func testNetworkIsolation(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "bash -c 'echo test > /dev/tcp/8.8.8.8/53' 2>/dev/null && echo 'NETWORK_ACCESSIBLE' || echo 'NETWORK_BLOCKED'",
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if !strings.Contains(result.Content[0].Text, "NETWORK_BLOCKED") {
		return fmt.Errorf("network not isolated: %s", result.Content[0].Text)
	}
	return nil
}

func testTimeoutEnforcement(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "echo 'starting'; i=0; while true; do i=$((i+1)); done",
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if !strings.Contains(result.Content[0].Text, "TIMED OUT") {
		return fmt.Errorf("timeout not enforced: %s", result.Content[0].Text)
	}
	return nil
}

func testEnvVars(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "echo \"MY_VAR=$MY_VAR\"",
		"env":      map[string]any{"MY_VAR": "test_value_123"},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "MY_VAR=test_value_123") {
		return fmt.Errorf("env var not passed: %s", result.Content[0].Text)
	}
	return nil
}

func testUnsupportedLanguage(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "rust",
		"code":     "fn main() {}",
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if !result.IsError {
		return fmt.Errorf("should fail for unsupported language")
	}
	if !strings.Contains(strings.ToLower(result.Content[0].Text), "unsupported language") {
		return fmt.Errorf("wrong error: %s", result.Content[0].Text)
	}
	return nil
}

func testExitCodePropagation(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "exit 42",
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if !strings.Contains(result.Content[0].Text, "Exit code: 42") {
		return fmt.Errorf("exit code not propagated: %s", result.Content[0].Text)
	}
	return nil
}

func testBashWithJq(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     `echo '{"name":"nix-exec","status":"ok"}' | jq -r .name`,
		"packages": []any{"jq"},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "nix-exec") {
		return fmt.Errorf("jq output missing: %s", result.Content[0].Text)
	}
	return nil
}

func testPythonWithPandas(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "python",
		"code": `import pandas as pd
df = pd.DataFrame({"name": ["alice", "bob"], "score": [95, 87]})
print(df.to_string(index=False))
print(f"mean={df['score'].mean()}")`,
		"packages": []any{
			"python3Packages.pandas",
		},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "alice") || !strings.Contains(text, "bob") {
		return fmt.Errorf("dataframe output missing: %s", text)
	}
	if !strings.Contains(text, "mean=91.0") {
		return fmt.Errorf("mean calculation wrong: %s", text)
	}
	return nil
}

func testReadOnlyFileMount(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "while IFS= read -r line; do echo \"$line\"; done < /workspace/files/test-input.txt",
		"files":    []any{"/tmp/test-input.txt"},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "hello from file") {
		return fmt.Errorf("file content not found: %s", result.Content[0].Text)
	}
	return nil
}

func testWritableFileMount(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language":       "bash",
		"code":           "echo 'written output' > /workspace/files/test-output-dir/result.txt && echo 'ok'",
		"writable_files": []any{"/tmp/test-output-dir"},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "ok") {
		return fmt.Errorf("writable file output missing: %s", result.Content[0].Text)
	}
	return nil
}

func testWorkspaceRootMount(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "bash",
		"code":     "echo 'hello from workspace' > /workspace/test-write.txt && read -r line < /workspace/test-write.txt && echo \"$line\"",
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "hello from workspace") {
		return fmt.Errorf("workspace write/read failed: %s", result.Content[0].Text)
	}

	data, err := os.ReadFile(c.rootsPath + "/test-write.txt")
	if err != nil {
		return fmt.Errorf("file not found on host: %w", err)
	}
	if !strings.Contains(string(data), "hello from workspace") {
		return fmt.Errorf("file content mismatch on host: %s", string(data))
	}
	return nil
}

func testNodeExecution(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "node",
		"code":     `const v = process.versions.node.split('.')[0]; console.log("node " + v);`,
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "node ") {
		return fmt.Errorf("node output missing: %s", result.Content[0].Text)
	}
	return nil
}

func testHaskellExecution(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "haskell",
		"code":     `main = putStrLn "hello from haskell"`,
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "hello from haskell") {
		return fmt.Errorf("haskell output missing: %s", text)
	}
	if !strings.Contains(text, "Exit code: 0") {
		return fmt.Errorf("wrong exit code: %s", text)
	}
	return nil
}

func testHaskellWithVector(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "haskell",
		"code": `import qualified Data.Vector as V
main = print (V.sum (V.fromList [1,2,3,4,5] :: V.Vector Int))`,
		"packages": []any{
			"haskellPackages.vector",
		},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "15") {
		return fmt.Errorf("vector sum output missing: %s", text)
	}
	return nil
}

func testLuaExecution(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "lua",
		"code":     `print("hello from lua " .. _VERSION)`,
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "hello from lua") {
		return fmt.Errorf("lua output missing: %s", text)
	}
	if !strings.Contains(text, "Exit code: 0") {
		return fmt.Errorf("wrong exit code: %s", text)
	}
	return nil
}

func testLuaWithDkjson(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "lua",
		"code": `local dkjson = require "dkjson"
local data = {name = "test", value = 42}
local str = dkjson.encode(data)
print(str)`,
		"packages": []any{
			"lua5_4Packages.dkjson",
		},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "test") || !strings.Contains(text, "42") {
		return fmt.Errorf("json output missing: %s", text)
	}
	return nil
}

func testRubyExecution(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "ruby",
		"code":     `puts "hello from ruby #{RUBY_VERSION}"`,
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "hello from ruby") {
		return fmt.Errorf("ruby output missing: %s", text)
	}
	if !strings.Contains(text, "Exit code: 0") {
		return fmt.Errorf("wrong exit code: %s", text)
	}
	return nil
}

func testRubyWithPg(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "ruby",
		"code":     `require 'pg'; puts PG::version_string`,
		"packages": []any{
			"rubyPackages.pg",
		},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "PG") && !strings.Contains(text, "pg") {
		return fmt.Errorf("pg gem output missing: %s", text)
	}
	return nil
}

func testPerlExecution(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "perl",
		"code":     `print "hello from perl $]\n";`,
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "hello from perl") {
		return fmt.Errorf("perl output missing: %s", text)
	}
	if !strings.Contains(text, "Exit code: 0") {
		return fmt.Errorf("wrong exit code: %s", text)
	}
	return nil
}

func testPerlWithJSON(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "perl",
		"code": `use JSON;
my $json = encode_json({name => "test", value => 42});
print "$json\n";`,
		"packages": []any{
			"perlPackages.JSON",
		},
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "test") || !strings.Contains(text, "42") {
		return fmt.Errorf("json output missing: %s", text)
	}
	return nil
}

func testOctaveExecution(c *mcpClient) error {
	resp := c.callTool("run_code", map[string]any{
		"language": "octave",
		"code":     `disp("hello from octave"); disp(version);`,
	})
	result, err := parseToolResult(resp)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("tool error: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "hello from octave") {
		return fmt.Errorf("octave output missing: %s", text)
	}
	if !strings.Contains(text, "Exit code: 0") {
		return fmt.Errorf("wrong exit code: %s", text)
	}
	return nil
}
