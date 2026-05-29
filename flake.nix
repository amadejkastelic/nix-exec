{
  description = "MCP Server for secure, sandboxed code execution with Nix";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      forAllSystems =
        fn:
        nixpkgs.lib.genAttrs nixpkgs.lib.systems.flakeExposed (
          system: fn nixpkgs.legacyPackages.${system}
        );
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.callPackage ./nix/package.nix { };
        test = pkgs.callPackage ./nix/test.nix { };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.callPackage ./nix/shell.nix { };
      });

      checks = forAllSystems (pkgs: {
        integration =
          (pkgs.testers.nixosTest or pkgs.nixosTest) (
            import ./nix/tests/integration.nix {
              inherit self nixpkgs;
              system = pkgs.stdenv.hostPlatform.system;
            }
          );
      });
    };
}
