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
              pnpm
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
              fetcherVersion = 3;
              hash = "sha256-+2nzW3TlJ6LcHUi951n/FVRuj57GySt6m753ZmArOUw=";
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
            ldflags = [
              "-s"
              "-w"
            ];
            vendorHash = "sha256-zCCH0h9n+Z7fWOJ7d8zFv4OmmNna3pzPPZ41DPOJ9dI=";

            preBuild = ''
              mkdir -p internal/frontend/app
              cp -r ${frontend}/dist internal/frontend/app
            '';
            buildInputs = [ frontend ];
          };

          packages.default = self'.packages.server;

          devShells.default = pkgs.mkShell {
            packages = with pkgs; [
              git-cliff
              gnumake
              go
              golangci-lint
              grype
              nodejs_24
              pnpm
            ];
          };
        };
    };
}
