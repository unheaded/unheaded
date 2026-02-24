{ config, pkgs, ... }:

{
  # =============================================================================
  # GATEWAY CONTAINER
  # =============================================================================
  # The public access point for the Unheaded infrastructure platform.
  # Reverse proxy with TLS 1.3 termination, HTTP/3 support, and rate limiting.
  #
  # Service: API Gateway (HTTPS/HTTP3 reverse proxy)
  # IP: 10.10.10.100
  # Ports: 80 (HTTP redirect), 443 (HTTPS/HTTP3)
  #
  # Proxies to:
  #   - Wotan (10.10.10.10:18001) - Message bus gRPC
  #   - Timeguru (10.10.10.20:19000) - Timeline API
  #   - Captain (10.10.10.21:19002) - Captain service
  #   - Micromanager (10.10.10.22:19003) - Micromanager service
  #   - Architect (10.10.10.23:19001) - Architect service
  #   - Monad (10.10.10.27:19004) - Monad service
  #   - Sophia (10.10.10.26:19005) - Sophia service
  #   - Kanban (10.10.10.200:20001) - Kanban application
  #   - Dashboard (10.10.10.201:20000) - Dashboard application
  #   - Cuirass (10.10.10.5:19006) - Control plane API
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
    ../modules/tls.nix
    ../modules/health-monitor.nix
  ];

  # ===========================================================================
  # HARDENING CONFIGURATION
  # ===========================================================================
  unheaded.hardening = {
    enable = true;
    serviceName = "gateway";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/gateway"
      "/var/log/unheaded/gateway"
      "/run/nginx"
    ];
    allowedPorts = [ 80 443 ];
  };

  # ===========================================================================
  # NETWORKING CONFIGURATION
  # ===========================================================================
  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.100";
    servicePort = 443;
    allowDirectAccess = true;          # Gateway is the public entry point
    allowedSources = [ "0.0.0.0/0" ];  # Accept from anywhere
  };

  # ===========================================================================
  # TLS CONFIGURATION
  # ===========================================================================
  unheaded.tls = {
    enable = true;
    serviceName = "gateway";
    minVersion = "1.3";
    mtls = {
      enable = false;                   # No mTLS for external connections
      verifyClient = "off";
    };
    certs = {
      caPath = "/etc/unheaded/tls/ca.pem";
      certPath = "/etc/unheaded/tls/gateway.pem";
      keyPath = "/etc/unheaded/tls/gateway-key.pem";
      rotationDays = 30;
      renewBeforeDays = 7;
    };
    secrets = {
      enable = false;                   # Gateway doesn't use service secrets
    };
  };

  # ===========================================================================
  # HEALTH MONITORING CONFIGURATION
  # ===========================================================================
  unheaded.healthMonitor = {
    enable = true;
    serviceName = "gateway";
    dependencies = [
      { name = "wotan"; ip = "10.10.10.10"; port = 18000; path = "/health"; }
      { name = "timeguru"; ip = "10.10.10.20"; port = 19000; path = "/health"; }
      { name = "captain"; ip = "10.10.10.21"; port = 19002; path = "/health"; }
      { name = "micromanager"; ip = "10.10.10.22"; port = 19003; path = "/health"; }
      { name = "architect"; ip = "10.10.10.23"; port = 19001; path = "/health"; }
      { name = "monad"; ip = "10.10.10.27"; port = 19004; path = "/health"; }
      { name = "sophia"; ip = "10.10.10.26"; port = 19005; path = "/health"; }
      { name = "kanban"; ip = "10.10.10.200"; port = 20001; path = "/health"; }
      { name = "dashboard"; ip = "10.10.10.201"; port = 20000; path = "/health"; }
      { name = "cuirass"; ip = "10.10.10.5"; port = 19006; path = "/health"; }
      { name = "doom-bridge"; ip = "10.10.10.28"; port = 16666; path = "/health"; }
    ];
    checkInterval = "30s";
    checkTimeout = 5;
    totalServices = 11;
  };

  # ===========================================================================
  # COMMON CONFIGURATION
  # ===========================================================================
  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
    metricsPort = 9100;
  };

  # ===========================================================================
  # NGINX REVERSE PROXY CONFIGURATION
  # ===========================================================================
  services.nginx = {
    enable = true;
    user = "unheaded";
    group = "unheaded";

    # Minimal configuration via services.nginx.httpConfig
    logError = "/var/log/unheaded/gateway/error.log";

    commonHttpConfig = ''
      # =======================
      # LOGGING
      # =======================
      log_format json_combined escape=json
        '{'
          '"timestamp":"$time_iso8601",'
          '"remote_addr":"$remote_addr",'
          '"request_method":"$request_method",'
          '"request_uri":"$request_uri",'
          '"status":$status,'
          '"bytes_sent":$bytes_sent,'
          '"request_time":$request_time,'
          '"upstream_response_time":"$upstream_response_time",'
          '"user_agent":"$http_user_agent",'
          '"upstream_addr":"$upstream_addr"'
        '}';

      access_log /var/log/unheaded/gateway/access.log json_combined;

      # =======================
      # PERFORMANCE
      # =======================
      sendfile on;
      tcp_nopush on;
      tcp_nodelay on;
      keepalive_timeout 65;
      types_hash_max_size 2048;
      client_max_body_size 20M;

      # =======================
      # GZIP COMPRESSION
      # =======================
      gzip on;
      gzip_vary on;
      gzip_min_length 1000;
      gzip_types text/plain text/css text/xml text/javascript
                 application/json application/javascript application/xml+rss
                 application/rss+xml font/truetype font/opentype
                 application/vnd.ms-fontobject image/svg+xml;

      # =======================
      # RATE LIMITING
      # =======================
      # Create zones for rate limiting per upstream
      limit_req_zone $binary_remote_addr zone=general:10m rate=100r/s;
      limit_req_zone $binary_remote_addr zone=api:10m rate=50r/s;
      limit_req_zone $binary_remote_addr zone=strict:10m rate=10r/s;

      # =======================
      # UPSTREAM DEFINITIONS
      # =======================
      # All active services in the Unheaded platform
      upstream wotan_backend {
        server 10.10.10.10:18001;
        keepalive 32;
      }

      upstream timeguru_backend {
        server 10.10.10.20:19000;
        keepalive 32;
      }

      upstream captain_backend {
        server 10.10.10.21:19002;
        keepalive 32;
      }

      upstream micromanager_backend {
        server 10.10.10.22:19003;
        keepalive 32;
      }

      upstream architect_backend {
        server 10.10.10.23:19001;
        keepalive 32;
      }

      upstream monad_backend {
        server 10.10.10.27:19004;
        keepalive 32;
      }

      upstream sophia_backend {
        server 10.10.10.26:19005;
        keepalive 32;
      }

      upstream kanban_backend {
        server 10.10.10.200:20001;
        keepalive 32;
      }

      upstream dashboard_backend {
        server 10.10.10.201:20000;
        keepalive 32;
      }

      upstream cuirass_backend {
        server 10.10.10.5:19006;
        keepalive 32;
      }

      upstream doom_bridge_backend {
        server 10.10.10.28:16666;
        keepalive 32;
      }

      # =======================
      # SECURITY HEADERS
      # =======================
      map $scheme $hsts_header {
        https "max-age=31536000; includeSubDomains; preload";
        default "";
      }

      # =======================
      # HTTP/3 QUIC
      # =======================
      http2_max_field_size 16k;
      http2_max_header_size 32k;

      # Support both IPv4 and IPv6
      # HTTP/3 support via QUIC (requires nginx-quic or recent OpenSSL 1.1.1)
    '';

    virtualHosts."_" = {
      listen = [
        { addr = "0.0.0.0"; port = 80; }
        { addr = "0.0.0.0"; port = 443; ssl = true; http2 = true; http3 = true; }
      ];

      default = true;

      # SSL/TLS Configuration
      sslCertificate = "/etc/unheaded/tls/gateway.pem";
      sslCertificateKey = "/etc/unheaded/tls/gateway-key.pem";
      sslProtocols = "TLSv1.3";
      sslCiphers = "TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256:TLS_AES_128_GCM_SHA256";
      sslEcdhCurve = "X25519:P-256:P-384";
      sslPreferServerCiphers = false;   # TLS 1.3 ignores this anyway
      sslSessionCache = "shared:SSL:10m";
      sslSessionTimeout = "10m";

      # OCSP stapling for performance
      sslStapling = true;
      sslStaplingCache = "shared:STAPLING:10m";
      sslStaplingResponderTimeout = "5s";
      sslStaplingVerifyDepth = 5;

      # HTTP to HTTPS redirect
      locations."/" = {
        extraConfig = ''
          if ($scheme != "https") {
            return 301 https://$server_name$request_uri;
          }

          # Add security headers
          add_header Strict-Transport-Security $hsts_header always;
          add_header X-Content-Type-Options "nosniff" always;
          add_header X-Frame-Options "SAMEORIGIN" always;
          add_header X-XSS-Protection "1; mode=block" always;
          add_header Referrer-Policy "strict-origin-when-cross-origin" always;
          add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;

          # Default: proxy to wotan (can be customized)
          proxy_pass http://wotan_backend;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          proxy_connect_timeout 10s;
          proxy_send_timeout 30s;
          proxy_read_timeout 30s;

          limit_req zone=general burst=200 nodelay;
        '';
      };

      # Health check endpoint (metrics)
      locations."/health" = {
        extraConfig = ''
          access_log off;
          default_type application/json;
          return 200 '{"status":"healthy","service":"gateway","ip":"10.10.10.100"}';
        '';
      };

      # Metrics endpoint (restricted to internal network by firewall)
      locations."/metrics" = {
        extraConfig = ''
          proxy_pass http://127.0.0.1:9100;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          access_log off;
        '';
      };

      # API routes with path-based routing
      locations."/api/wotan/" = {
        extraConfig = ''
          proxy_pass http://wotan_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=api burst=100 nodelay;
        '';
      };

      locations."/api/timeguru/" = {
        extraConfig = ''
          proxy_pass http://timeguru_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=api burst=100 nodelay;
        '';
      };

      locations."/api/captain/" = {
        extraConfig = ''
          proxy_pass http://captain_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=api burst=100 nodelay;
        '';
      };

      locations."/api/micromanager/" = {
        extraConfig = ''
          proxy_pass http://micromanager_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=api burst=100 nodelay;
        '';
      };

      locations."/api/architect/" = {
        extraConfig = ''
          proxy_pass http://architect_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=api burst=100 nodelay;
        '';
      };

      locations."/api/monad/" = {
        extraConfig = ''
          proxy_pass http://monad_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=api burst=100 nodelay;
        '';
      };

      locations."/api/sophia/" = {
        extraConfig = ''
          proxy_pass http://sophia_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=api burst=100 nodelay;
        '';
      };

      locations."/kanban/" = {
        extraConfig = ''
          proxy_pass http://kanban_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=general burst=200 nodelay;
        '';
      };

      locations."/dashboard/" = {
        extraConfig = ''
          proxy_pass http://dashboard_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=general burst=200 nodelay;
        '';
      };

      locations."/control/" = {
        extraConfig = ''
          proxy_pass http://cuirass_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=strict burst=50 nodelay;
        '';
      };

      locations."/doom/" = {
        extraConfig = ''
          proxy_pass http://doom_bridge_backend/;
          proxy_http_version 1.1;
          proxy_set_header Connection "";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          limit_req zone=general burst=200 nodelay;
        '';
      };

      locations."/ws/doom/" = {
        extraConfig = ''
          proxy_pass http://doom_bridge_backend/ws;
          proxy_http_version 1.1;
          proxy_set_header Upgrade $http_upgrade;
          proxy_set_header Connection "upgrade";
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_buffering off;
          proxy_read_timeout 86400;
        '';
      };

      # Catch-all for undefined routes
      locations."~" = {
        extraConfig = ''
          default_type application/json;
          return 404 '{"error":"Not found","path":"$request_uri"}';
        '';
      };
    };
  };

  # ===========================================================================
  # GATEWAY SYSTEMD SERVICE
  # ===========================================================================
  systemd.services.gateway = {
    description = "Unheaded Gateway - API Gateway and Reverse Proxy";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "unheaded-cert-generate.service" ];
    wants = [ "network-online.target" ];
    requires = [ "unheaded-cert-generate.service" ];

    serviceConfig = {
      Type = "notify";
      User = "unheaded";
      Group = "unheaded";
      ExecStart = "${pkgs.nginx}/bin/nginx -c /etc/nginx/nginx.conf -g 'daemon off;'";
      ExecReload = "${pkgs.nginx}/bin/nginx -s reload";

      # Health check
      ExecStartPost = "${pkgs.bash}/bin/bash -c 'sleep 2 && /etc/unheaded/health-check.sh'";

      # Graceful shutdown
      KillSignal = "SIGTERM";
      TimeoutStopSec = "30s";

      # Resource limits
      MemoryMax = "512M";
      CPUQuota = "150%";
      TasksMax = 512;

      # Restart policy
      Restart = "always";
      RestartSec = "5s";
      StartLimitBurst = 10;
      StartLimitIntervalSec = 60;
    };
  };

  # ===========================================================================
  # FIREWALL CONFIGURATION
  # ===========================================================================
  # HTTP and HTTPS ports exposed to the world
  # ===========================================================================
  networking.firewall.allowedTCPPorts = [ 80 443 9100 ];

  # ===========================================================================
  # DIRECTORY STRUCTURE
  # ===========================================================================
  systemd.tmpfiles.rules = [
    # Gateway-specific directories
    "d /var/lib/unheaded/gateway 0750 unheaded unheaded -"
    "d /var/log/unheaded/gateway 0755 unheaded unheaded -"
  ];
}
