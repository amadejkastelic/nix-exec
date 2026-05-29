//go:build integration

package integration_test

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type mcpClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int
	t      *testing.T
}

type jsonrpcMessage struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method,omitempty"`
	Params  any    `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func newMCPClient(t *testing.T) *mcpClient {
	t.Helper()

	configPath := os.Getenv("NIX_EXEC_TEST_CONFIG")
	if configPath == "" {
		t.Fatal("NIX_EXEC_TEST_CONFIG environment variable must be set")
	}

	cmd := exec.Command("nix-exec", "--config", configPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}

	t.Cleanup(func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	})

	return &mcpClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReaderSize(stdoutPipe, 1<<20),
		nextID: 1,
		t:      t,
	}
}

func (c *mcpClient) send(method string, params any) int {
	c.t.Helper()
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

	data, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatalf("marshal message: %v", err)
	}

	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		c.t.Fatalf("write message: %v", err)
	}

	return id
}

func (c *mcpClient) notify(method string) {
	c.t.Helper()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatalf("marshal notification: %v", err)
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		c.t.Fatalf("write notification: %v", err)
	}
}

func (c *mcpClient) recv() jsonrpcMessage {
	c.t.Helper()

	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("read response: %v", err)
	}

	var msg jsonrpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		c.t.Fatalf("unmarshal response: %v\nraw: %s", err, line)
	}

	return msg
}

func (c *mcpClient) call(method string, params any) jsonrpcMessage {
	c.t.Helper()

	id := c.send(method, params)
	for {
		msg := c.recv()
		if msg.ID != nil && *msg.ID == id {
			return msg
		}
	}
}

func (c *mcpClient) callTool(name string, arguments map[string]any) jsonrpcMessage {
	c.t.Helper()
	return c.call("tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
}

type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func parseToolResult(t *testing.T, resp jsonrpcMessage) toolResult {
	t.Helper()

	if resp.Error != nil {
		t.Fatalf("RPC error: %d %s", resp.Error.Code, resp.Error.Message)
	}

	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal tool result: %v\nraw: %s", err, resp.Result)
	}

	return result
}

func TestIntegration(t *testing.T) {
	client := newMCPClient(t)

	t.Run("initialize", func(t *testing.T) {
		resp := client.call("initialize", map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test",
				"version": "1.0.0",
			},
		})

		if resp.Error != nil {
			t.Fatalf("initialize error: %s", resp.Error.Message)
		}

		var result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
			Capabilities map[string]any `json:"capabilities"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("unmarshal init result: %v", err)
		}

		if result.ServerInfo.Name != "nix-exec-test" {
			t.Errorf("expected server name 'nix-exec-test', got %q", result.ServerInfo.Name)
		}
		if _, ok := result.Capabilities["tools"]; !ok {
			t.Error("missing tools capability")
		}

		client.notify("notifications/initialized")
	})

	t.Run("tools_list", func(t *testing.T) {
		resp := client.call("tools/list", nil)
		if resp.Error != nil {
			t.Fatalf("tools/list error: %s", resp.Error.Message)
		}

		var result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("unmarshal tools result: %v", err)
		}

		var found bool
		for _, tool := range result.Tools {
			if tool.Name == "run_code" {
				found = true
				props, _ := tool.InputSchema["properties"].(map[string]any)
				if _, ok := props["language"]; !ok {
					t.Error("missing language property")
				}
				if _, ok := props["code"]; !ok {
					t.Error("missing code property")
				}
				required, _ := tool.InputSchema["required"].([]any)
				var hasLang, hasCode bool
				for _, r := range required {
					s, _ := r.(string)
					if s == "language" {
						hasLang = true
					}
					if s == "code" {
						hasCode = true
					}
				}
				if !hasLang {
					t.Error("language not required")
				}
				if !hasCode {
					t.Error("code not required")
				}
			}
		}
		if !found {
			t.Fatal("run_code tool not found")
		}
	})

	t.Run("bash_echo", func(t *testing.T) {
		resp := client.callTool("run_code", map[string]any{
			"language": "bash",
			"code":     "echo 'hello from nix-exec'",
		})
		result := parseToolResult(t, resp)
		if result.IsError {
			t.Fatalf("tool error: %s", result.Content[0].Text)
		}
		text := result.Content[0].Text
		if !strings.Contains(text, "hello from nix-exec") {
			t.Errorf("missing output in: %s", text)
		}
		if !strings.Contains(text, "Exit code: 0") {
			t.Errorf("wrong exit code in: %s", text)
		}
	})

	t.Run("bash_caching", func(t *testing.T) {
		start := time.Now()
		resp := client.callTool("run_code", map[string]any{
			"language": "bash",
			"code":     "echo 'cached call'",
		})
		elapsed := time.Since(start)

		result := parseToolResult(t, resp)
		if result.IsError {
			t.Fatalf("tool error: %s", result.Content[0].Text)
		}
		if !strings.Contains(result.Content[0].Text, "cached call") {
			t.Errorf("missing output: %s", result.Content[0].Text)
		}
		if elapsed > 10*time.Second {
			t.Errorf("cached call took %v, caching may not work", elapsed)
		}
	})

	t.Run("python_execution", func(t *testing.T) {
		resp := client.callTool("run_code", map[string]any{
			"language": "python",
			"code":     "import sys; print(f'python {sys.version_info.major}.{sys.version_info.minor}')",
		})
		result := parseToolResult(t, resp)
		if result.IsError {
			t.Fatalf("tool error: %s", result.Content[0].Text)
		}
		text := result.Content[0].Text
		if !strings.Contains(text, "python 3") {
			t.Errorf("missing python version: %s", text)
		}
		if !strings.Contains(text, "Exit code: 0") {
			t.Errorf("wrong exit code: %s", text)
		}
	})

	t.Run("filesystem_isolation", func(t *testing.T) {
		resp := client.callTool("run_code", map[string]any{
			"language": "bash",
			"code":     "touch /nix/store/test-write-$$ 2>&1; echo \"exit=$?\"",
		})
		result := parseToolResult(t, resp)
		text := result.Content[0].Text
		isolated := strings.Contains(text, "Permission denied") ||
			strings.Contains(text, "Read-only") ||
			strings.Contains(text, "exit=1")
		if !isolated {
			t.Errorf("filesystem not isolated: %s", text)
		}
	})

	t.Run("network_isolation", func(t *testing.T) {
		resp := client.callTool("run_code", map[string]any{
			"language": "bash",
			"code":     "bash -c 'echo test > /dev/tcp/8.8.8.8/53' 2>/dev/null && echo 'NETWORK_ACCESSIBLE' || echo 'NETWORK_BLOCKED'",
		})
		result := parseToolResult(t, resp)
		if !strings.Contains(result.Content[0].Text, "NETWORK_BLOCKED") {
			t.Errorf("network not isolated: %s", result.Content[0].Text)
		}
	})

	t.Run("timeout_enforcement", func(t *testing.T) {
		resp := client.callTool("run_code", map[string]any{
			"language": "bash",
			"code":     "echo 'starting'; sleep 30; echo 'should not reach'",
		})
		result := parseToolResult(t, resp)
		if !strings.Contains(result.Content[0].Text, "TIMED OUT") {
			t.Errorf("timeout not enforced: %s", result.Content[0].Text)
		}
	})

	t.Run("env_vars", func(t *testing.T) {
		resp := client.callTool("run_code", map[string]any{
			"language": "bash",
			"code":     "echo \"MY_VAR=$MY_VAR\"",
			"env": map[string]any{
				"MY_VAR": "test_value_123",
			},
		})
		result := parseToolResult(t, resp)
		if result.IsError {
			t.Fatalf("tool error: %s", result.Content[0].Text)
		}
		if !strings.Contains(result.Content[0].Text, "MY_VAR=test_value_123") {
			t.Errorf("env var not passed: %s", result.Content[0].Text)
		}
	})

	t.Run("unsupported_language", func(t *testing.T) {
		resp := client.callTool("run_code", map[string]any{
			"language": "rust",
			"code":     "fn main() {}",
		})
		result := parseToolResult(t, resp)
		if !result.IsError {
			t.Fatal("should fail for unsupported language")
		}
		if !strings.Contains(strings.ToLower(result.Content[0].Text), "unsupported language") {
			t.Errorf("wrong error: %s", result.Content[0].Text)
		}
	})

	t.Run("exit_code_propagation", func(t *testing.T) {
		resp := client.callTool("run_code", map[string]any{
			"language": "bash",
			"code":     "exit 42",
		})
		result := parseToolResult(t, resp)
		if !strings.Contains(result.Content[0].Text, "Exit code: 42") {
			t.Errorf("exit code not propagated: %s", result.Content[0].Text)
		}
	})
}
