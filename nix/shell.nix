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
    go version
  '';
}
