{ self, lib }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.yafe;
in
{
  options.services.yafe = {
    enable = lib.mkEnableOption "[yafe](https://git.myservermanager.com/varakh/yafe) - enable yafe daemon";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "self.packages.\${system}.default";
      description = "The yafe package to use.";
    };

    extraPackages = lib.mkOption {
      type = lib.types.listOf lib.types.package;
      default = [ pkgs.bash ];
      defaultText = lib.literalExpression "[ pkgs.bash ]";
      example = lib.literalExpression "[ pkgs.bash ]";
      description = "Extra packages to add to PATH for the yaefe daemon process.";
    };

    environment = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      example = {
        SERVER_LISTEN = "127.0.0.1";
        SERVER_PORT = "8080";
      };
      description = ''
        Environment variables for yafe. Non-sensitive values go here.
        Secrets (YAFE_AUTH_KEY, etc.) must be
        set via {option}`environmentFiles` so they are not stored in the nix store.
        See [configuration reference](https://git.myservermanager.com/varakh/yafe) for all options.
      '';
    };

    environmentFiles = lib.mkOption {
      type = lib.types.listOf lib.types.path;
      default = [ ];
      example = [ "/run/secrets/yafe.env" ];
      description = ''
        Files containing additional environment variables for yafe.
        Secrets such as YAFE_AUTH_KEY must be provided here
        rather than in {option}`environment` to avoid storing them in the nix store.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.services.yafe = {
      description = "yafe - a lightweight workflow engine";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      path = cfg.extraPackages;

      environment = cfg.environment;

      serviceConfig = {
        ExecStart = "${cfg.package}/bin/yafe serve";
        EnvironmentFile = cfg.environmentFiles;
        DynamicUser = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        RestrictNamespaces = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        RestrictRealtime = true;
      };
    };
  };
}
