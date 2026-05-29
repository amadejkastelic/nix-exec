{
  description = "MCP Server for secure, sandboxed code execution with Nix";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

    pre-commit-hooks = {
      url = "github:cachix/git-hooks.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      pre-commit-hooks,
      ...
    }:
    let
      forAllSystems =
        fn:
        nixpkgs.lib.genAttrs nixpkgs.lib.systems.flakeExposed (
          system: fn nixpkgs.legacyPackages.${system} system
        );
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.callPackage ./nix/package.nix { };
        test = pkgs.callPackage ./nix/tests/package.nix { };
      });

      devShells = forAllSystems (
        pkgs: system:
        pkgs.callPackage ./nix/shell.nix {
          preCommitCheck = self.checks.${system}.pre-commit-check;
        }
      );

      nixosModules.default = import ./nix/module.nix;

      checks = forAllSystems (
        pkgs: system:
        {
          integration = (pkgs.testers.nixosTest or pkgs.nixosTest) (
            import ./nix/tests/test.nix {
              inherit self nixpkgs;
              system = pkgs.stdenv.hostPlatform.system;
            }
          );

          pre-commit-check = import ./nix/pre-commit.nix {
            inherit pkgs;
            preCommitHooks = pre-commit-hooks.lib.${system};
          };
        }
      );

      devShells = forAllSystems (pkgs: {
        default = pkgs.callPackage ./nix/shell.nix {
          inherit preCommitCheck;
        };
      });

      nixosModules.default = import ./nix/module.nix;

      checks = forAllSystems (pkgs: {
        integration = (pkgs.testers.nixosTest or pkgs.nixosTest) (
          import ./nix/tests/test.nix {
            inherit self nixpkgs;
            system = pkgs.stdenv.hostPlatform.system;
          }
        );

        pre-commit-check = import ./nix/pre-commit.nix {
          inherit pkgs;
          preCommitHooks = pre-commit-hooks.lib.${pkgs.stdenv.hostPlatform.system};
        };
      });
    };
}
