{ config, lib, pkgs, ... }:

let
  cfg = config.services.kantor;
  backend = pkgs.callPackage ./nix/backend.nix { };
  frontend = pkgs.callPackage ./nix/frontend.nix { };
in
{
  options.services.kantor = {
    enable = lib.mkEnableOption "Kantor internal platform";

    domain = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Domain name for the Kantor platform. If null, serves on IP with HTTP only.";
    };

    listenPort = lib.mkOption {
      type = lib.types.port;
      default = 3000;
      description = "Nginx listen port for the frontend";
    };

    acmeEmail = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Email for ACME/Let's Encrypt registration";
    };

    cloudflareTokenFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to file containing Cloudflare DNS API token";
    };

    port = lib.mkOption {
      type = lib.types.port;
      default = 8080;
      description = "Port for the backend API server";
    };

    envFile = lib.mkOption {
      type = lib.types.path;
      description = "Path to environment file with secrets (JWT_SECRET, DATA_ENCRYPTION_KEY, etc.)";
    };

    uploadsDir = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/kantor/uploads";
      description = "Directory for file uploads";
    };

    database = {
      name = lib.mkOption {
        type = lib.types.str;
        default = "kantor";
        description = "PostgreSQL database name";
      };

      user = lib.mkOption {
        type = lib.types.str;
        default = "kantor";
        description = "PostgreSQL user";
      };
    };
  };

  config = lib.mkIf cfg.enable {
    # PostgreSQL
    services.postgresql = {
      enable = true;
      ensureDatabases = [ cfg.database.name ];
      ensureUsers = [
        {
          name = cfg.database.user;
          ensureDBOwnership = true;
        }
      ];
    };

    # Backend systemd service
    systemd.services.kantor-backend = {
      description = "Kantor Backend API";
      after = [ "network.target" "postgresql.service" ];
      requires = [ "postgresql.service" ];
      wantedBy = [ "multi-user.target" ];

      environment = {
        APP_ENV = "production";
        PORT = toString cfg.port;
        DATABASE_URL = "postgres://${cfg.database.user}@localhost/${cfg.database.name}?sslmode=disable&host=/run/postgresql";
        UPLOADS_DIR = cfg.uploadsDir;
        CORS_ORIGINS =
          if cfg.domain != null
          then "https://${cfg.domain}"
          else "http://72.60.79.109:${toString cfg.listenPort}";
        JWT_ACCESS_EXPIRY = "15m";
        JWT_REFRESH_EXPIRY = "168h";
        TRACKER_RETENTION_DAYS = "90";
        SEED_SUPERADMIN_ENABLED = "true";
        SEED_DEMO_USERS_ENABLED = "true";
        WAHA_ENABLED = "true";
        WAHA_SESSION = "default";
        WAHA_MAX_DAILY_MESSAGES = "100";
        WAHA_MIN_DELAY_MS = "1000";
        WAHA_MAX_DELAY_MS = "3000";
        WAHA_REMINDER_CRON = "0 8 * * 1-5";
        WAHA_WEEKLY_DIGEST_CRON = "0 9 * * 1";
        APP_URL =
          if cfg.domain != null
          then "https://${cfg.domain}"
          else "http://72.60.79.109:${toString cfg.listenPort}";
      };

      preStart = ''
        mkdir -p ${cfg.uploadsDir}
        ln -sfn ${backend}/share/kantor/migrations migrations
        rm -rf extension
        cp -rL ${backend}/share/kantor/extension extension
      '';

      serviceConfig = {
        Type = "simple";
        ExecStart = "${backend}/bin/server";
        EnvironmentFile = cfg.envFile;
        User = "kantor";
        Group = "kantor";
        StateDirectory = "kantor";
        WorkingDirectory = "/var/lib/kantor";
        Restart = "on-failure";
        RestartSec = 5;

        # Hardening
        NoNewPrivileges = true;
        ProtectHome = true;
        PrivateTmp = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictSUIDSGID = true;
      };
    };

    # Kantor system user
    users.users.kantor = {
      isSystemUser = true;
      group = "kantor";
      home = "/var/lib/kantor";
    };
    users.groups.kantor = { };

    # Create uploads directory
    systemd.tmpfiles.rules = [
      "d ${cfg.uploadsDir} 0750 kantor kantor -"
    ];

    # ACME (only when domain is set)
    security.acme = lib.mkIf (cfg.domain != null && cfg.cloudflareTokenFile != null) {
      acceptTerms = true;
      defaults.email = cfg.acmeEmail;
      certs.${cfg.domain} = {
        domain = cfg.domain;
        extraDomainNames = [ "*.${cfg.domain}" ];
        group = "nginx";
        dnsProvider = "cloudflare";
        dnsResolver = "1.1.1.1:53";
        credentialFiles = {
          "CF_DNS_API_TOKEN_FILE" = cfg.cloudflareTokenFile;
        };
      };
    };

    # Nginx
    services.nginx = {
      enable = true;
      recommendedOptimisation = true;
      recommendedGzipSettings = true;
      recommendedProxySettings = true;

      virtualHosts."kantor" = {
        listen = [{ addr = "0.0.0.0"; port = cfg.listenPort; }];
        serverName = if cfg.domain != null then cfg.domain else "_";
        forceSSL = cfg.domain != null;
        useACMEHost = lib.mkIf (cfg.domain != null) cfg.domain;

        root = "${frontend}";

        locations."/" = {
          tryFiles = "$uri $uri/ /index.html";
        };

        locations."/api/" = {
          proxyPass = "http://127.0.0.1:${toString cfg.port}";
          proxyWebsockets = true;
          extraConfig = ''
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
          '';
        };

        locations."/assets/" = {
          root = "${frontend}";
          extraConfig = ''
            add_header Cache-Control "public, max-age=31536000, immutable";
            add_header X-Frame-Options "DENY" always;
            add_header X-Content-Type-Options "nosniff" always;
            add_header Referrer-Policy "strict-origin-when-cross-origin" always;
            add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;
            try_files $uri =404;
          '';
        };

        extraConfig = ''
          add_header X-Frame-Options "DENY" always;
          add_header X-Content-Type-Options "nosniff" always;
          add_header Referrer-Policy "strict-origin-when-cross-origin" always;
          add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;
        '';
      };
    };

    networking.firewall.allowedTCPPorts = [ cfg.listenPort ];
  };
}
