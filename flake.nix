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
        test = pkgs.callPackage ./nix/tests/package.nix { };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.callPackage ./nix/shell.nix { };
      });

      nixosModules.default = import ./nix/module.nix;

      checks = forAllSystems (pkgs: {
        integration =
          (pkgs.testers.nixosTest or pkgs.nixosTest) (
            import ./nix/tests/test.nix {
              inherit self nixpkgs;
              system = pkgs.stdenv.hostPlatform.system;
            }
          );
      });
    };
}
