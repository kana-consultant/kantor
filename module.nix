{ config, lib, pkgs, ... }:

let
  cfg = config.services.kantor;
  backend = pkgs.callPackage ./nix/backend.nix { };
  frontend = pkgs.callPackage ./nix/frontend.nix { };

  # Derive values from tenants list
  allDomains = lib.concatLists (map (t: t.domains) cfg.tenants);
  hasDomains = allDomains != [];
  primaryDomain = if hasDomains then builtins.head allDomains else null;

  # TENANTS env: "name|slug|d1,d2;name2|slug2|d3"
  tenantsEnv = lib.concatStringsSep ";" (map
    (t: "${t.name}|${t.slug}|${lib.concatStringsSep "," t.domains}")
    cfg.tenants
  );

  corsOrigins =
    if hasDomains
    then lib.concatStringsSep "," (map (d: "https://${d}") allDomains)
    else "http://localhost:${toString cfg.listenPort}";
in
{
  options.services.kantor = {
    enable = lib.mkEnableOption "Kantor internal platform";

    tenants = lib.mkOption {
      type = lib.types.listOf (lib.types.submodule {
        options = {
          name = lib.mkOption {
            type = lib.types.str;
            description = "Display name for the tenant";
          };
          slug = lib.mkOption {
            type = lib.types.str;
            description = "URL-safe slug for the tenant";
          };
          domains = lib.mkOption {
            type = lib.types.listOf lib.types.str;
            description = "Domain names for this tenant (first = primary)";
          };
        };
      });
      default = [];
      description = "List of tenants to seed on startup. First tenant owns existing data.";
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
        CORS_ORIGINS = corsOrigins;
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
          if primaryDomain != null
          then "https://${primaryDomain}"
          else "http://localhost:${toString cfg.listenPort}";
        TENANTS = tenantsEnv;
      };

      preStart = ''
        mkdir -p ${cfg.uploadsDir}
        ln -sfn ${backend}/share/kantor/migrations migrations
        rm -rf extension || true
        chmod -R u+w extension 2>/dev/null || true
        rm -rf extension
        cp -rL ${backend}/share/kantor/extension extension
        chmod -R u+w extension
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

    # ACME (only when domains are set)
    security.acme = lib.mkIf (hasDomains && cfg.cloudflareTokenFile != null) {
      acceptTerms = true;
      defaults.email = cfg.acmeEmail;
      certs.${primaryDomain} = {
        domain = primaryDomain;
        extraDomainNames = builtins.tail allDomains;
        group = "nginx";
        dnsProvider = "cloudflare";
        dnsResolver = "1.1.1.1:53";
        credentialFiles = {
          "CF_DNS_API_TOKEN_FILE" = cfg.cloudflareTokenFile;
        };
      };
    };

    # Nginx — serves all tenant domains from a single vhost
    services.nginx = {
      enable = true;
      recommendedOptimisation = true;
      recommendedGzipSettings = true;
      recommendedProxySettings = true;

      virtualHosts."kantor" = {
        listen = lib.mkIf (!hasDomains) [{ addr = "0.0.0.0"; port = cfg.listenPort; }];
        serverName =
          if hasDomains
          then lib.concatStringsSep " " allDomains
          else "_";
        forceSSL = hasDomains;
        useACMEHost = lib.mkIf hasDomains primaryDomain;

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

    networking.firewall.allowedTCPPorts =
      if hasDomains then [ 80 443 ] else [ cfg.listenPort ];
  };
}
