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
        default = pkgs.buildGoModule {
          pname = "nix-exec";
          version = "0.1.0";
          src = ./.;
          vendorHash = "";
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gotools
            gopls
            bubblewrap
          ];

          shellHook = ''
            echo "nix-exec dev shell ready"
          '';
        };
      });
    };
}
