{
  pkgs,
  preCommitHooks,
}:

let
  deps =
    (pkgs.buildGoModule {
      pname = "nix-exec-modules";
      version = "dev";
      src = ../.;
      proxyVendor = true;
      vendorHash = "sha256-THTp9T9s0lb5++sasngQhbFCN0cEIofHAM4Md73yO/E=";
    }).goModules;

  goWithProxy = pkgs.writeShellScriptBin "go" ''
    export GOPROXY="file://${deps}"
    export GOSUMDB="off"
    exec ${pkgs.go}/bin/go "$@"
  '';
in
preCommitHooks.run {
  src = ../.;
  hooks = {
    nixfmt-rfc-style.enable = true;
    golangci-lint = {
      enable = true;
      extraPackages = [
        goWithProxy
        pkgs.gofumpt
        pkgs.golines
        pkgs.gotools
      ];
    };
    gotest = {
      enable = true;
      extraPackages = [ goWithProxy ];
    };
  };
}
