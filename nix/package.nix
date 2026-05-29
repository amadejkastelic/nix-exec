{
  buildGoModule,
}:
let
  version = "0.1.0";
in
buildGoModule {
  pname = "nix-exec";
  inherit version;
  src = ./..;
  vendorHash = "sha256-TNGu96NH5DSdsHfjiPXT0twuOCsVlc4kpFULb+ebbLE=";

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];
}
