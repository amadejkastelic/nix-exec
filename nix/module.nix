{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.programs.nix-exec;
  format = pkgs.formats.yaml { };
in
{
  options.programs.nix-exec = {
    enable = lib.mkEnableOption "nix-exec MCP server";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./package.nix { };
      defaultText = lib.literalExpression "pkgs.nix-exec";
      description = "The nix-exec package to use.";
    };

    settings = lib.mkOption {
      type = lib.types.submodule {
        freeformType = format.type;

        options.server = lib.mkOption {
          type = lib.types.submodule {
            freeformType = format.type;
            options = {
              name = lib.mkOption {
                type = lib.types.str;
                default = "nix-exec";
                description = "Server name reported in MCP handshake.";
              };
            };
          };
          default = { };
        };

        options.sandbox = lib.mkOption {
          type = lib.types.submodule {
            freeformType = format.type;
            options = {
              timeout = lib.mkOption {
                type = lib.types.str;
                default = "30s";
                description = "Maximum execution time per run.";
              };
              max_output_bytes = lib.mkOption {
                type = lib.types.ints.positive;
                default = 1048576;
                description = "Maximum bytes captured from stdout/stderr.";
              };
              workspace_path = lib.mkOption {
                type = lib.types.str;
                default = "";
                description = "Path to expose read-only as /workspace inside the sandbox.";
              };
              package_denylist = lib.mkOption {
                type = lib.types.listOf lib.types.str;
                default = [ ];
                description = "Nix packages that are never allowed.";
              };
            };
          };
          default = { };
        };

        options.executor = lib.mkOption {
          type = lib.types.submodule {
            freeformType = format.type;
            options = {
              cache_dir = lib.mkOption {
                type = lib.types.str;
                default = "~/.cache/nix-exec";
                description = "Directory for caching built Nix environments.";
              };
              cache_max_size = lib.mkOption {
                type = lib.types.ints.positive;
                default = 64;
                description = "Maximum number of cached environments.";
              };
              temp_dir = lib.mkOption {
                type = lib.types.str;
                default = "/tmp";
                description = "Base directory for temporary files.";
              };
              nixpkgs_url = lib.mkOption {
                type = lib.types.str;
                default = "github:NixOS/nixpkgs/nixpkgs-unstable";
                description = "Nixpkgs flake URL used to resolve packages.";
              };
              substituters = lib.mkOption {
                type = lib.types.nullOr (lib.types.listOf lib.types.str);
                default = null;
                description = "Nix substituters. Set to [] to disable, null uses system defaults.";
              };
            };
          };
          default = { };
        };

        options.logging = lib.mkOption {
          type = lib.types.submodule {
            freeformType = format.type;
            options = {
              level = lib.mkOption {
                type = lib.types.enum [
                  "debug"
                  "info"
                  "warn"
                  "error"
                ];
                default = "info";
                description = "Log level.";
              };
              format = lib.mkOption {
                type = lib.types.enum [
                  "text"
                  "json"
                ];
                default = "json";
                description = "Log format.";
              };
            };
          };
          default = { };
        };
      };
      default = { };
      description = "nix-exec configuration (config.yaml).";
    };
  };

  config = lib.mkIf cfg.enable {
    environment.systemPackages = [
      cfg.package
      pkgs.bubblewrap
    ];

    nix.enable = lib.mkDefault true;
    nix.settings.experimental-features = lib.mkDefault [
      "nix-command"
      "flakes"
    ];

    environment.etc."nix-exec/config.yaml".source = format.generate "nix-exec-config.yaml" cfg.settings;
  };
}
