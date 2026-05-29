{
  pkgs,
  preCommitHooks,
}:
let
  deps = (pkgs.callPackage ./package.nix { }).goModules;

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
      extraPackages = with pkgs; [
        goWithProxy
        gofumpt
        golines
        gotools
      ];
    };
    gotest = {
      enable = true;
      extraPackages = [ goWithProxy ];
    };
  };
}
