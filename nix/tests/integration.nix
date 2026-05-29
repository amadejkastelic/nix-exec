{
  self,
  nixpkgs,
  system,
}:
let
  nix-exec-pkg = self.packages.${system}.default;
  nixpkgs-path = toString nixpkgs;
in
{
  name = "nix-exec-integration";
  meta.timeout = 600;

  nodes.machine =
    { pkgs, ... }:
    {
      environment.systemPackages = [
        nix-exec-pkg
        pkgs.bubblewrap
        pkgs.python3
      ];

      nix.enable = true;
      nix.settings.experimental-features = [
        "nix-command"
        "flakes"
      ];

      virtualisation.memorySize = 4096;
      virtualisation.diskSize = 8192;

      environment.etc."nix-exec/test-config.yaml".text = ''
        server:
          name: "nix-exec-test"
          version: "0.1.0-test"
        sandbox:
          timeout: 5s
          max_output_bytes: 1048576
        executor:
          cache_dir: /tmp/nix-exec-cache
          temp_dir: /tmp
          nixpkgs_url: "path:${nixpkgs-path}"
        logging:
          level: "debug"
          format: "text"
      '';
    };

  testScript =
    let
      test-script = ./run_tests.py;
    in
    ''
      import json

      machine.wait_for_unit("multi-user.target")
      machine.wait_for_unit("nix-daemon.service")

      machine.succeed("which nix-exec")
      machine.succeed("which bwrap")

      with subtest("MCP initialize handshake"):
          result = machine.succeed(
              "printf '%s' '"
              + '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}'
              + "' | timeout 10 nix-exec --config /etc/nix-exec/test-config.yaml 2>/dev/null"
          )
          resp = json.loads(result.strip().split("\\n")[0])
          assert resp["id"] == 1
          assert resp["result"]["serverInfo"]["name"] == "nix-exec-test"
          assert "tools" in resp["result"]["capabilities"]

      with subtest("MCP tools/list"):
          result = machine.succeed(
              "printf '%s\\n%s\\n%s' '"
              + '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}'
              + "' '"
              + '{"jsonrpc":"2.0","method":"notifications/initialized"}'
              + "' '"
              + '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
              + "' | timeout 10 nix-exec --config /etc/nix-exec/test-config.yaml 2>/dev/null"
          )
          lines = result.strip().split("\\n")
          resp = json.loads(lines[-1])
          assert resp["id"] == 2
          tools = resp["result"]["tools"]
          run_code = next((t for t in tools if t["name"] == "run_code"), None)
          assert run_code is not None
          assert "language" in run_code["inputSchema"]["properties"]
          assert "code" in run_code["inputSchema"]["properties"]

      with subtest("Full integration test suite"):
          machine.succeed("python3 ${test-script} 2>&1 | tee /tmp/test-output.txt")
          machine.succeed("grep 'Results:' /tmp/test-output.txt")
          machine.fail("grep 'FAIL:' /tmp/test-output.txt")
    '';
}
