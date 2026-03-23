{
  description = "Kantor - Internal company platform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      packages = forAllSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          backend = pkgs.callPackage ./nix/backend.nix { };
          frontend = pkgs.callPackage ./nix/frontend.nix { };
          default = self.packages.${system}.backend;
        }
      );

      nixosModules.default = import ./module.nix;
    };
}
