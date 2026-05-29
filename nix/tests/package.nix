{ buildGoModule }:

buildGoModule {
  pname = "nix-exec-integration";
  version = "0.1.0";
  src = ./../..;
  vendorHash = "sha256-TNGu96NH5DSdsHfjiPXT0twuOCsVlc4kpFULb+ebbLE=";
  subPackages = [ "./nix/tests" ];
  doCheck = false;
}
