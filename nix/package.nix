{
  buildGoModule,
}:
buildGoModule {
  pname = "nix-exec";
  version = "0.1.0";
  src = ./..;
  vendorHash = "";
}
