{
  buildGoModule,
  lib,
}:
buildGoModule (finalAttrs: {
  pname = "nix-exec";
  version = "0.1.0";
  src = ./..;
  vendorHash = "sha256-TNGu96NH5DSdsHfjiPXT0twuOCsVlc4kpFULb+ebbLE=";

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${finalAttrs.version}"
  ];

  meta = {
    description = "MCP server for secure, sandboxed code execution with Nix";
    homepage = "https://github.com/amadejkastelic/nix-exec";
    license = lib.licenses.mit;
    maintainers = [ lib.maintainers.amadejkastelic ];
    mainProgram = "nix-exec";
    platforms = lib.platforms.linux ++ lib.platforms.darwin;
  };
})
