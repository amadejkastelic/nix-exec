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
      imports = [ self.nixosModules.default ];

      services.nix-exec = {
        enable = true;
        package = nix-exec-pkg;
        settings = {
          server.name = "nix-exec-test";
          server.version = "0.1.0-test";
          sandbox.timeout = "5s";
          executor = {
            cache_dir = "/tmp/nix-exec-cache";
            nixpkgs_url = "path:${nixpkgs-path}";
          };
          logging = {
            level = "debug";
            format = "text";
          };
        };
      };

      virtualisation.memorySize = 4096;
      virtualisation.diskSize = 8192;
    };

  testScript = ''
    machine.wait_for_unit("multi-user.target")
    machine.succeed("which nix-exec")
    machine.succeed("which bwrap")
    machine.succeed("which nix")

    machine.succeed(
      "NIX_EXEC_TEST_CONFIG=/etc/nix-exec/config.yaml "
      + "${test-pkg}/bin/nix-exec-integration-test -test.v -test.timeout 600s 2>&1"
    )
  '';
}
