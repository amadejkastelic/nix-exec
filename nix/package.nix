{
  buildGoModule,
}:
buildGoModule {
  pname = "nix-exec";
  version = "0.1.0";
  src = ./..;
  vendorHash = "sha256-TNGu96NH5DSdsHfjiPXT0twuOCsVlc4kpFULb+ebbLE=";

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${placeholder "version"}"
  ];
}
