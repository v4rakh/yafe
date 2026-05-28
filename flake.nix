{
  description = "yafe flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];

      perSystem =
        { pkgs, self', ... }:
        let
          version = "0.1.0";
          frontend = pkgs.stdenv.mkDerivation (finalAttrs: {
            pname = "yafe-ui";
            inherit version;
            src = ./internal/frontend/app;

            nativeBuildInputs = with pkgs; [
              nodejs_24
              pnpm_10
              pnpmConfigHook
            ];

            pnpmInstallFlags = [ "--frozen-lockfile" ];

            pnpmDeps = pkgs.fetchPnpmDeps {
              inherit (finalAttrs)
                pname
                version
                src
                pnpmInstallFlags
                ;
              fetcherVersion = 2;
              hash = "sha256-IkG5eHMilKFArpNCj8LfahN3RoK6u7cFeRovPgkr4Rk=";
            };

            buildPhase = ''
              pnpm build
            '';

            installPhase = ''
              runHook preInstall
              mkdir -p $out
              cp -r dist $out/
              runHook postInstall
            '';
          });
        in
        {
          packages.frontend = frontend;

          packages.server = pkgs.buildGoModule {
            pname = "yafe";
            inherit version;
            src = ./.;
            tags = [ "embed" ];
            doCheck = false;
            vendorHash = "sha256-slzOPQPBlHRRE/3rNxQ40/WycMD98O57BL1k6WLaQTQ=";

            preBuild = ''
              mkdir -p internal/frontend/app
              cp -r ${frontend}/dist internal/frontend/app
            '';
            buildInputs = [ frontend ];
          };

          packages.default = self'.packages.server;
        };
    };
}
