#!/usr/bin/env python3
"""Integration tests for nix-exec MCP server over stdio."""

import json
import os
import select
import subprocess
import sys

CONFIG = "/etc/nix-exec/test-config.yaml"


class MCPClient:
    def __init__(self, timeout=120):
        self.proc = subprocess.Popen(
            ["nix-exec", "--config", CONFIG],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            bufsize=0,
        )
        self.default_timeout = timeout
        self._next_id = 1

    def send(self, method, params=None, timeout=None):
        id_ = self._next_id
        self._next_id += 1
        msg = {"jsonrpc": "2.0", "id": id_, "method": method}
        if params is not None:
            msg["params"] = params
        self.proc.stdin.write((json.dumps(msg) + "\n").encode())
        self.proc.stdin.flush()
        return id_, timeout or self.default_timeout

    def notify(self, method):
        msg = {"jsonrpc": "2.0", "method": method}
        self.proc.stdin.write((json.dumps(msg) + "\n").encode())
        self.proc.stdin.flush()

    def recv(self, timeout=None):
        timeout = timeout or self.default_timeout
        fd = self.proc.stdout.fileno()
        data = b""
        while True:
            ready, _, _ = select.select([fd], [], [], timeout)
            if not ready:
                raise TimeoutError(f"No response after {timeout}s")
            ch = os.read(fd, 1)
            if ch == b"\n" or ch == b"":
                if data:
                    break
                continue
            data += ch
            timeout = 30  # After first byte, use shorter timeout
        if not data:
            raise EOFError("Server closed stdout")
        return json.loads(data)

    def call(self, method, params=None, timeout=None):
        id_, t = self.send(method, params, timeout)
        resp = self.recv(t)
        assert resp.get("id") == id_, f"Response id mismatch: expected {id_}, got {resp.get('id')}"
        return resp

    def call_tool(self, name, arguments, timeout=None):
        return self.call("tools/call", {
            "name": name,
            "arguments": arguments,
        }, timeout=timeout)

    def close(self):
        self.proc.terminate()
        try:
            self.proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()


passed = 0
failed = 0


def run_test(name, func, client):
    global passed, failed
    try:
        func(client)
        print(f"PASS: {name}")
        passed += 1
    except Exception as e:
        print(f"FAIL: {name}: {e}")
        import traceback
        traceback.print_exc()
        failed += 1


def test_initialize(client):
    resp = client.call("initialize", {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {"name": "test", "version": "1.0.0"},
    })
    result = resp["result"]
    assert result["serverInfo"]["name"] == "nix-exec-test", \
        f"Wrong server name: {result['serverInfo']['name']}"
    assert "tools" in result["capabilities"], \
        f"Missing tools in capabilities"


def test_tools_list(client):
    client.notify("notifications/initialized")
    resp = client.call("tools/list")
    tools = resp["result"]["tools"]
    run_code = next((t for t in tools if t["name"] == "run_code"), None)
    assert run_code is not None, f"run_code not found in tools"
    schema = run_code["inputSchema"]
    assert "language" in schema["properties"]
    assert "code" in schema["properties"]
    assert "language" in schema.get("required", [])
    assert "code" in schema.get("required", [])


def test_bash_echo(client):
    resp = client.call_tool("run_code", {
        "language": "bash",
        "code": "echo 'hello from nix-exec'",
    }, timeout=180)
    assert not resp["result"].get("isError", False), \
        f"Tool error: {resp['result']}"
    text = resp["result"]["content"][0]["text"]
    assert "hello from nix-exec" in text, f"Missing output: {text}"
    assert "Exit code: 0" in text, f"Wrong exit code: {text}"


def test_bash_caching(client):
    start = import_time()
    resp = client.call_tool("run_code", {
        "language": "bash",
        "code": "echo 'cached call'",
    }, timeout=30)
    elapsed = import_time() - start
    assert not resp["result"].get("isError", False)
    text = resp["result"]["content"][0]["text"]
    assert "cached call" in text
    assert elapsed < 10, f"Cached call took {elapsed:.1f}s, caching may not work"


def test_python_execution(client):
    resp = client.call_tool("run_code", {
        "language": "python",
        "code": "import sys; print(f'python {sys.version_info.major}.{sys.version_info.minor}')",
    }, timeout=180)
    assert not resp["result"].get("isError", False)
    text = resp["result"]["content"][0]["text"]
    assert "python 3" in text, f"Missing python version: {text}"
    assert "Exit code: 0" in text


def test_filesystem_isolation(client):
    resp = client.call_tool("run_code", {
        "language": "bash",
        "code": "touch /nix/store/test-write-$$ 2>&1; echo \"exit=$?\"",
    }, timeout=30)
    text = resp["result"]["content"][0]["text"]
    is_isolated = (
        "Permission denied" in text
        or "Read-only" in text
        or "exit=1" in text
    )
    assert is_isolated, f"Sandbox filesystem not isolated: {text}"


def test_network_isolation(client):
    resp = client.call_tool("run_code", {
        "language": "bash",
        "code": (
            "bash -c 'echo test > /dev/tcp/8.8.8.8/53' 2>/dev/null && "
            "echo 'NETWORK_ACCESSIBLE' || echo 'NETWORK_BLOCKED'"
        ),
    }, timeout=30)
    text = resp["result"]["content"][0]["text"]
    assert "NETWORK_BLOCKED" in text, f"Network not isolated: {text}"


def test_timeout(client):
    resp = client.call_tool("run_code", {
        "language": "bash",
        "code": "echo 'starting'; sleep 30; echo 'should not reach'",
    }, timeout=30)
    text = resp["result"]["content"][0]["text"]
    assert "TIMED OUT" in text, f"Timeout not enforced: {text}"


def test_env_vars(client):
    resp = client.call_tool("run_code", {
        "language": "bash",
        "code": "echo \"MY_VAR=$MY_VAR\"",
        "env": {"MY_VAR": "test_value_123"},
    }, timeout=30)
    assert not resp["result"].get("isError", False)
    text = resp["result"]["content"][0]["text"]
    assert "MY_VAR=test_value_123" in text, f"Env var not passed: {text}"


def test_unsupported_language(client):
    resp = client.call_tool("run_code", {
        "language": "rust",
        "code": "fn main() {}",
    }, timeout=10)
    assert resp["result"].get("isError", False), "Should fail for unsupported language"
    text = resp["result"]["content"][0]["text"]
    assert "unsupported language" in text.lower(), f"Wrong error: {text}"


def test_exit_code_propagation(client):
    resp = client.call_tool("run_code", {
        "language": "bash",
        "code": "exit 42",
    }, timeout=30)
    text = resp["result"]["content"][0]["text"]
    assert "Exit code: 42" in text, f"Exit code not propagated: {text}"


def import_time():
    import time
    return time.monotonic()


def main():
    print(f"Starting nix-exec integration tests (config: {CONFIG})")

    client = MCPClient(timeout=120)

    tests = [
        ("MCP initialize handshake", test_initialize),
        ("MCP tools/list", test_tools_list),
        ("bash echo execution", test_bash_echo),
        ("environment caching", test_bash_caching),
        ("python execution", test_python_execution),
        ("filesystem isolation", test_filesystem_isolation),
        ("network isolation", test_network_isolation),
        ("timeout enforcement", test_timeout),
        ("environment variables", test_env_vars),
        ("unsupported language error", test_unsupported_language),
        ("exit code propagation", test_exit_code_propagation),
    ]

    for name, func in tests:
        run_test(name, func, client)

    client.close()
    print(f"\nResults: {passed} passed, {failed} failed out of {len(tests)} tests")
    sys.exit(1 if failed > 0 else 0)


if __name__ == "__main__":
    main()
