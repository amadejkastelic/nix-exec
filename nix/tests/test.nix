{
  self,
  nixpkgs,
  system,
}:
let
  test-pkg = self.packages.${system}.test;
  nixpkgs-path = toString nixpkgs;
in
{
  name = "nix-exec-integration";
  meta.timeout = 600;

  nodes.machine =
    { pkgs, ... }:
    let
      mkEnv =
        paths:
        pkgs.buildEnv {
          name = "nix-exec-env";
          paths = paths;
        };

      mkPythonEnv =
        pythonPkgs:
        pkgs.buildEnv {
          name = "nix-exec-env";
          paths = [ (pkgs.python3.withPackages pythonPkgs) ];
        };

      testEnvs = [
        (mkEnv [ pkgs.bash ])
        (mkEnv [
          pkgs.bash
          pkgs.jq
        ])
        (mkEnv [ pkgs.python3 ])
        (mkEnv [ pkgs.nodejs ])
        (mkPythonEnv (ps: [ ps.pandas ]))
      ];
    in
    {
      imports = [ self.nixosModules.default ];

      programs.nix-exec = {
        enable = true;
        settings = {
          server.name = "nix-exec-test";
          sandbox.timeout = "5m";
          executor = {
            cache_dir = "/tmp/nix-exec-cache";
            nixpkgs_url = "path:${nixpkgs-path}";
            substituters = [ ];
          };
          logging = {
            level = "debug";
            format = "text";
          };
        };
      };

      virtualisation.memorySize = 4096;
      virtualisation.diskSize = 8192;
      virtualisation.writableStore = true;
      virtualisation.additionalPaths = testEnvs;
    };

  testScript = ''
    machine.wait_for_unit("multi-user.target")
    machine.succeed("which nix-exec")
    machine.succeed("which bwrap")
    machine.succeed("which nix")

    machine.succeed("echo 'hello from file' > /tmp/test-input.txt")
    machine.succeed("mkdir -p /tmp/test-output-dir")

    machine.succeed(
      "NIX_EXEC_TEST_CONFIG=/etc/nix-exec/config.yaml "
      + "${test-pkg}/bin/tests 2>&1 | tee /dev/console"
    )
  '';
}
