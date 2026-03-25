{ lib, buildGoModule }:

buildGoModule {
  pname = "kantor-backend";
  version = "0.1.0";

  src = ../backend;

  vendorHash = "sha256-9j/8xOA1ZvWNn/HMMWnkIwCPpA7j6/63IF+T1ApcpU8=";

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
