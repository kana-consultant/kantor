{ lib, buildNpmPackage }:

buildNpmPackage {
  pname = "kantor-frontend";
  version = "0.1.0";

  src = ../frontend;

  npmDepsHash = "sha256-QfiHC3SWfW9cxJmkDYjxeyXjaOyJc5kgb1zBVzrqP/M=";

  VITE_API_BASE_URL = "/api/v1";

  buildPhase = ''
    runHook preBuild
    npx vite build
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    mkdir -p $out
    cp -r dist/* $out/
    runHook postInstall
  '';

  meta = {
    description = "Kantor frontend web application";
    license = lib.licenses.mit;
  };
}
