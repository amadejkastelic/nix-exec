{
  mkShell,
  delve,
  go,
  golangci-lint,
  golines,
  gotools,
  gopls,
  bubblewrap,
  preCommitCheck,
}:
mkShell {
  packages = [
    delve
    go
    golangci-lint
    golines
    gotools
    gopls
    bubblewrap
  ];

  shellHook = ''
    ${preCommitCheck.shellHook}
    go version
  '';
}
