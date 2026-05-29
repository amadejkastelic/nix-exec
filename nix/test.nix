{ buildGoModule }:

buildGoModule {
  pname = "nix-exec-integration";
  version = "0.1.0";
  src = ./..;
  vendorHash = "sha256-TNGu96NH5DSdsHfjiPXT0twuOCsVlc4kpFULb+ebbLE=";

  buildPhase = ''
    runHook preBuild
    go test -c -tags=integration -o integration-test ./tests/integration/
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    mkdir -p $out/bin
    install -Dm755 integration-test $out/bin/nix-exec-integration-test
    runHook postInstall
  '';

  doCheck = false;
}
