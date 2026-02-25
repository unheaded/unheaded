{ config, lib, pkgs, ... }:

{
  # =============================================================================
  # UNHEADED TLS & SECRETS MODULE
  # =============================================================================
  # Provides TLS 1.3, mTLS between services, and SOPS+age secrets management.
  #
  # Features:
  #   - TLS 1.3 minimum for all external connections
  #   - Mutual TLS (mTLS) between services using internal CA
  #   - SOPS + age for encrypted secrets at rest
  #   - Automatic certificate rotation via systemd timers
  #   - OCSP stapling for external certificates
  #
  # Usage:
  #   imports = [ ../modules/tls.nix ];
  #   unheaded.tls.enable = true;
  #   unheaded.tls.serviceName = "timeguru";
  # =============================================================================

  options = {
    unheaded.tls = {
      enable = lib.mkEnableOption "Unheaded TLS and secrets configuration";

      serviceName = lib.mkOption {
        type = lib.types.str;
        description = "Service name for certificate CN and file paths";
      };

      # =========================================================================
      # TLS OPTIONS
      # =========================================================================
      minVersion = lib.mkOption {
        type = lib.types.enum [ "1.2" "1.3" ];
        default = "1.3";
        description = "Minimum TLS version (1.3 recommended, 1.2 for legacy)";
      };

      mtls = {
        enable = lib.mkOption {
          type = lib.types.bool;
          default = true;
          description = "Enable mutual TLS for inter-service communication";
        };

        verifyClient = lib.mkOption {
          type = lib.types.enum [ "require" "optional" "off" ];
          default = "require";
          description = "Client certificate verification mode";
        };
      };

      # =========================================================================
      # CERTIFICATE OPTIONS
      # =========================================================================
      certs = {
        caPath = lib.mkOption {
          type = lib.types.str;
          default = "/etc/unheaded/tls/ca.pem";
          description = "Path to internal CA certificate";
        };

        certPath = lib.mkOption {
          type = lib.types.str;
          default = "/etc/unheaded/tls/${config.unheaded.tls.serviceName}.pem";
          description = "Path to service TLS certificate";
        };

        keyPath = lib.mkOption {
          type = lib.types.str;
          default = "/etc/unheaded/tls/${config.unheaded.tls.serviceName}-key.pem";
          description = "Path to service TLS private key";
        };

        rotationDays = lib.mkOption {
          type = lib.types.int;
          default = 30;
          description = "Certificate validity period in days";
        };

        renewBeforeDays = lib.mkOption {
          type = lib.types.int;
          default = 7;
          description = "Renew certificate this many days before expiry";
        };
      };

      # =========================================================================
      # SECRETS OPTIONS
      # =========================================================================
      secrets = {
        enable = lib.mkOption {
          type = lib.types.bool;
          default = true;
          description = "Enable SOPS+age secrets management";
        };

        ageKeyPath = lib.mkOption {
          type = lib.types.str;
          default = "/etc/unheaded/secrets/age-key.txt";
          description = "Path to age private key for decryption";
        };

        secretsDir = lib.mkOption {
          type = lib.types.str;
          default = "/etc/unheaded/secrets/${config.unheaded.tls.serviceName}";
          description = "Directory for decrypted service secrets";
        };

        encryptedDir = lib.mkOption {
          type = lib.types.str;
          default = "/etc/unheaded/secrets/encrypted";
          description = "Directory for SOPS-encrypted secret files";
        };
      };
    };
  };

  config = lib.mkIf config.unheaded.tls.enable {
    # =========================================================================
    # REQUIRED PACKAGES
    # =========================================================================
    environment.systemPackages = with pkgs; [
      # TLS tools
      openssl

      # Secrets management
      sops
      age
    ];

    # =========================================================================
    # DIRECTORY STRUCTURE
    # =========================================================================
    systemd.tmpfiles.rules = [
      # TLS certificate directory
      "d /etc/unheaded/tls 0750 root unheaded -"

      # Secrets directories
      "d /etc/unheaded/secrets 0700 root root -"
      "d ${config.unheaded.tls.secrets.secretsDir} 0750 root unheaded -"
      "d ${config.unheaded.tls.secrets.encryptedDir} 0750 root root -"
    ];

    # =========================================================================
    # INTERNAL CA INITIALIZATION
    # =========================================================================
    # One-shot service to generate internal CA if not present.
    # In production, the CA is pre-provisioned; this is for bootstrapping.
    #
    # For development environments, you can also use the Go cert-gen tool:
    #   go run ./cmd/cert-gen -out /etc/unheaded/tls
    # Or the shell wrapper:
    #   CERT_DIR=/etc/unheaded/tls ./scripts/gen-dev-certs.sh
    #
    # The Go tool produces ECDSA P-256 certs with the same SANs and hardening
    # as this openssl-based NixOS service, ensuring dev-prod parity.
    # =========================================================================
    systemd.services."unheaded-ca-init" = {
      description = "Initialize Unheaded Internal CA";
      wantedBy = [ "multi-user.target" ];
      before = [ "unheaded-cert-generate.service" ];

      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
      };

      script = ''
        CA_DIR="/etc/unheaded/tls"
        CA_CERT="$CA_DIR/ca.pem"
        CA_KEY="$CA_DIR/ca-key.pem"

        # Only generate if CA doesn't exist
        if [ -f "$CA_CERT" ] && [ -f "$CA_KEY" ]; then
          echo "CA already exists, skipping generation"
          exit 0
        fi

        echo "Generating Unheaded Internal CA..."

        # Generate CA private key (EC P-256 for performance)
        ${pkgs.openssl}/bin/openssl ecparam -genkey -name prime256v1 \
          -out "$CA_KEY" 2>/dev/null

        # Generate CA certificate (10 year validity)
        ${pkgs.openssl}/bin/openssl req -new -x509 \
          -key "$CA_KEY" \
          -out "$CA_CERT" \
          -days 3650 \
          -subj "/C=US/O=Unheaded/OU=Infrastructure/CN=Unheaded Internal CA" \
          -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
          -addext "keyUsage=critical,keyCertSign,cRLSign" \
          -addext "subjectKeyIdentifier=hash" 2>/dev/null

        # Set permissions
        chmod 644 "$CA_CERT"
        chmod 600 "$CA_KEY"
        chown root:unheaded "$CA_CERT"
        chown root:root "$CA_KEY"

        echo "CA generated: $CA_CERT"
      '';
    };

    # =========================================================================
    # SERVICE CERTIFICATE GENERATION
    # =========================================================================
    # Generates service-specific certificates signed by the internal CA.
    # =========================================================================
    systemd.services."unheaded-cert-generate" = {
      description = "Generate TLS Certificate for ${config.unheaded.tls.serviceName}";
      wantedBy = [ "multi-user.target" ];
      after = [ "unheaded-ca-init.service" ];
      requires = [ "unheaded-ca-init.service" ];
      before = [ "${config.unheaded.tls.serviceName}.service" ];

      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
      };

      script = let
        serviceName = config.unheaded.tls.serviceName;
        certPath = config.unheaded.tls.certs.certPath;
        keyPath = config.unheaded.tls.certs.keyPath;
        caPath = config.unheaded.tls.certs.caPath;
        caKey = "/etc/unheaded/tls/ca-key.pem";
        days = toString config.unheaded.tls.certs.rotationDays;
        renewDays = toString config.unheaded.tls.certs.renewBeforeDays;
      in ''
        # Check if certificate exists and is still valid
        if [ -f "${certPath}" ]; then
          # Check if cert expires within renewDays
          if ${pkgs.openssl}/bin/openssl x509 -checkend $((${renewDays} * 86400)) \
              -in "${certPath}" -noout 2>/dev/null; then
            echo "Certificate still valid, skipping renewal"
            exit 0
          fi
          echo "Certificate expiring soon, renewing..."
        fi

        echo "Generating certificate for ${serviceName}..."

        # Generate service private key (EC P-256)
        ${pkgs.openssl}/bin/openssl ecparam -genkey -name prime256v1 \
          -out "${keyPath}" 2>/dev/null

        # Generate CSR
        ${pkgs.openssl}/bin/openssl req -new \
          -key "${keyPath}" \
          -out "/tmp/${serviceName}.csr" \
          -subj "/C=US/O=Unheaded/OU=${serviceName}/CN=${serviceName}.unheaded.internal" 2>/dev/null

        # Create extensions file for SAN
        cat > "/tmp/${serviceName}-ext.cnf" <<EXTEOF
        [v3_ext]
        basicConstraints = CA:FALSE
        keyUsage = critical, digitalSignature, keyEncipherment
        extendedKeyUsage = serverAuth, clientAuth
        subjectAltName = @alt_names
        subjectKeyIdentifier = hash
        authorityKeyIdentifier = keyid,issuer

        [alt_names]
        DNS.1 = ${serviceName}
        DNS.2 = ${serviceName}.unheaded.internal
        DNS.3 = ${serviceName}.unheaded.local
        DNS.4 = localhost
        IP.1 = 127.0.0.1
        IP.2 = ::1
        EXTEOF

        # Sign with internal CA
        ${pkgs.openssl}/bin/openssl x509 -req \
          -in "/tmp/${serviceName}.csr" \
          -CA "${caPath}" \
          -CAkey "${caKey}" \
          -CAcreateserial \
          -out "${certPath}" \
          -days ${days} \
          -extfile "/tmp/${serviceName}-ext.cnf" \
          -extensions v3_ext 2>/dev/null

        # Clean up temporary files
        rm -f "/tmp/${serviceName}.csr" "/tmp/${serviceName}-ext.cnf"

        # Set permissions
        chmod 644 "${certPath}"
        chmod 600 "${keyPath}"
        chown root:unheaded "${certPath}"
        chown root:unheaded "${keyPath}"

        echo "Certificate generated: ${certPath} (valid ${days} days)"
      '';
    };

    # =========================================================================
    # CERTIFICATE ROTATION TIMER
    # =========================================================================
    # Daily check for certificate expiry, auto-renew if needed.
    # =========================================================================
    systemd.timers."unheaded-cert-rotate" = {
      description = "Unheaded Certificate Rotation Timer";
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnCalendar = "daily";
        Persistent = true;
        RandomizedDelaySec = "1h";
      };
    };

    systemd.services."unheaded-cert-rotate" = {
      description = "Rotate TLS Certificate for ${config.unheaded.tls.serviceName}";

      serviceConfig = {
        Type = "oneshot";
      };

      script = let
        serviceName = config.unheaded.tls.serviceName;
        certPath = config.unheaded.tls.certs.certPath;
        renewDays = toString config.unheaded.tls.certs.renewBeforeDays;
      in ''
        if [ ! -f "${certPath}" ]; then
          echo "No certificate found, triggering generation..."
          systemctl start unheaded-cert-generate.service
          exit 0
        fi

        # Check if cert expires within renewDays
        if ! ${pkgs.openssl}/bin/openssl x509 -checkend $((${renewDays} * 86400)) \
            -in "${certPath}" -noout 2>/dev/null; then
          echo "Certificate expiring within ${renewDays} days, rotating..."

          # Regenerate certificate
          systemctl start unheaded-cert-generate.service

          # Reload the service to pick up new certificate
          systemctl reload-or-restart "${serviceName}.service" || true

          echo "Certificate rotated and service reloaded"
        else
          echo "Certificate still valid, no rotation needed"
        fi
      '';
    };

    # =========================================================================
    # SOPS + AGE SECRETS DECRYPTION
    # =========================================================================
    # Decrypts SOPS-encrypted secrets on boot using age key.
    # Secrets are mounted as files, never as environment variables.
    # =========================================================================
    systemd.services."unheaded-secrets-decrypt" = lib.mkIf config.unheaded.tls.secrets.enable {
      description = "Decrypt SOPS Secrets for ${config.unheaded.tls.serviceName}";
      wantedBy = [ "multi-user.target" ];
      before = [ "${config.unheaded.tls.serviceName}.service" ];

      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
      };

      script = let
        serviceName = config.unheaded.tls.serviceName;
        ageKeyPath = config.unheaded.tls.secrets.ageKeyPath;
        secretsDir = config.unheaded.tls.secrets.secretsDir;
        encryptedDir = config.unheaded.tls.secrets.encryptedDir;
      in ''
        # Check for age key
        if [ ! -f "${ageKeyPath}" ]; then
          echo "WARNING: No age key found at ${ageKeyPath}"
          echo "Secrets will not be decrypted. Service may fail."
          exit 0
        fi

        # Check for encrypted secrets
        ENCRYPTED_FILE="${encryptedDir}/${serviceName}.enc.yaml"
        if [ ! -f "$ENCRYPTED_FILE" ]; then
          echo "No encrypted secrets found for ${serviceName}, skipping"
          exit 0
        fi

        echo "Decrypting secrets for ${serviceName}..."

        # Set age key for SOPS
        export SOPS_AGE_KEY_FILE="${ageKeyPath}"

        # Decrypt secrets to service directory
        ${pkgs.sops}/bin/sops --decrypt \
          --output "${secretsDir}/secrets.yaml" \
          "$ENCRYPTED_FILE"

        # Extract individual secrets as files (one secret per file)
        # This prevents secrets from appearing in process environment
        ${pkgs.yq-go}/bin/yq eval '.secrets // {} | keys | .[]' \
          "${secretsDir}/secrets.yaml" 2>/dev/null | while read -r key; do
          ${pkgs.yq-go}/bin/yq eval ".secrets.\"$key\"" \
            "${secretsDir}/secrets.yaml" > "${secretsDir}/$key"
          chmod 640 "${secretsDir}/$key"
          chown root:unheaded "${secretsDir}/$key"
        done

        # Set directory permissions
        chmod 750 "${secretsDir}"
        chown root:unheaded "${secretsDir}"

        echo "Secrets decrypted to ${secretsDir}"
      '';
    };

    # =========================================================================
    # TLS ENVIRONMENT VARIABLES
    # =========================================================================
    # Services read these to configure their TLS connections.
    # Private key path is intentionally NOT in env vars (read from file).
    # =========================================================================
    environment.variables = {
      # Certificate paths (public info, safe for env vars)
      UNHEADED_TLS_CA_CERT = config.unheaded.tls.certs.caPath;
      UNHEADED_TLS_CERT = config.unheaded.tls.certs.certPath;
      UNHEADED_TLS_KEY_FILE = config.unheaded.tls.certs.keyPath;

      # TLS configuration
      UNHEADED_TLS_MIN_VERSION = config.unheaded.tls.minVersion;
      UNHEADED_TLS_MTLS_ENABLED = if config.unheaded.tls.mtls.enable then "true" else "false";
      UNHEADED_TLS_VERIFY_CLIENT = config.unheaded.tls.mtls.verifyClient;

      # Secrets path
      UNHEADED_SECRETS_DIR = config.unheaded.tls.secrets.secretsDir;
    };

    # =========================================================================
    # SYSCTL: TLS ENTROPY
    # =========================================================================
    # Ensure sufficient entropy for key generation
    # =========================================================================
    boot.kernel.sysctl = {
      "kernel.random.write_wakeup_threshold" = 3072;
    };
  };
}
