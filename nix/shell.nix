{
  mkShell,
  go,
  gotools,
  gopls,
  bubblewrap,
}:
mkShell {
  packages = [
    go
    gotools
    gopls
    bubblewrap
  ];

  shellHook = ''
    echo "nix-exec dev shell ready"
  '';
}
