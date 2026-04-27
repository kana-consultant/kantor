{ lib, buildGoModule }:

buildGoModule {
  pname = "kantor-backend";
  version = "0.1.0";

  src = ../backend;

  vendorHash = "sha256-l7nHZg1E48sPRCE4javft1FyREMmHLm2fO6jnk/2nts=";

  subPackages = [ "cmd/server" ];

  postInstall = ''
    mkdir -p $out/share/kantor
    cp -r $src/migrations $out/share/kantor/migrations
    cp -r ${../extension} $out/share/kantor/extension
  '';

  meta = {
    description = "Kantor backend API server";
    license = lib.licenses.mit;
    mainProgram = "server";
  };
}
