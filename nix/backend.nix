{ lib, buildGoModule }:

buildGoModule {
  pname = "kantor-backend";
  version = "0.1.0";

  src = ../backend;

  vendorHash = "sha256-Wt8vHpTP8BpN7vdkI6R+isD6co9D8GJhC1gQ39nuGFA=";

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
