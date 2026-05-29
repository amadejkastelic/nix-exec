{
  self,
  nixpkgs,
  system,
}:
let
  nix-exec-pkg = self.packages.${system}.default;
  test-pkg = self.packages.${system}.test;
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

  testScript = ''
    machine.wait_for_unit("multi-user.target")
    machine.succeed("which nix-exec")
    machine.succeed("which bwrap")
    machine.succeed("which nix")

    machine.succeed(
      "NIX_EXEC_TEST_CONFIG=/etc/nix-exec/test-config.yaml "
      + "${test-pkg}/bin/nix-exec-integration-test -test.v -test.timeout 600s 2>&1"
    )
  '';
}
