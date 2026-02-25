# RSS Security Feed Access Plan
## MoatGhost Threat Intelligence Ingestion

**Version**: 1.0
**Created**: 2026-02-20 (Round Table S26)
**Owner**: MoatGhost
**Classification**: INTERNAL — Key material procedures

---

## Overview

Self-hosted RSS/API security feed aggregator for continuous threat intelligence ingestion. MoatGhost auto-ingests daily feeds to populate the Kingdom's threat intelligence register. All credentials stored via sops-nix encrypted secrets — never plaintext.

---

## Architecture

```
┌─────────────────────────────────────────────┐
│              Feed Sources                    │
│  NIST NVD │ CISA KEV │ CVEfeed │ ZDI       │
│  Red Hat  │ Juniper  │ OTX     │ Shadowserver│
└──────────────────┬──────────────────────────┘
                   │ HTTPS (RSS/JSON/STIX)
                   ▼
┌─────────────────────────────────────────────┐
│     NixOS Container: miniflux-aggregator     │
│     ├─ Miniflux (RSS reader)                 │
│     ├─ PostgreSQL (feed storage)             │
│     ├─ IP: 192.168.100.10/24 (VXLAN)        │
│     └─ sops-nix managed secrets              │
└──────────────────┬──────────────────────────┘
                   │ Miniflux API (Bearer token)
                   ▼
┌─────────────────────────────────────────────┐
│  NixOS Container: threat-intel-worker        │
│  ├─ Python ingestion scripts (cron)          │
│  │   ├─ NVD API v2 (every 2 hours)          │
│  │   ├─ CISA KEV JSON (daily)               │
│  │   └─ Vendor feeds (every 6 hours)        │
│  ├─ IP: 192.168.100.20/24 (VXLAN)           │
│  └─ sops-nix managed API keys               │
└──────────────────┬──────────────────────────┘
                   │ Structured threat data
                   ▼
┌─────────────────────────────────────────────┐
│        MoatGhost Threat Register             │
│  ├─ CVE tracking with CVSS scoring           │
│  ├─ CISA KEV compliance check                │
│  ├─ Vendor advisory correlation              │
│  ├─ Critical alert triggers (CVSS > 8.0)     │
│  └─ Feeds: BlackMage (targets), Architect    │
│     (hardening), Developer (patches),        │
│     Captain (risk dashboard)                 │
└─────────────────────────────────────────────┘
```

---

## Feed Sources

### Tier 1: Critical (Auto-Ingest, High Frequency)

| Source | Protocol | URL | Frequency | Auth |
|--------|----------|-----|-----------|------|
| NIST NVD | REST API v2 | `https://services.nvd.nist.gov/rest/json/cves/2.0` | Every 2 hours | API Key |
| CISA KEV | JSON API | `https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json` | Daily | None |
| CVEfeed (Critical) | RSS | `https://cvefeed.io/rssfeed?severity=critical` | Every 15 min | None |

### Tier 1.5: Go Ecosystem (Auto-Ingest, Critical for Unheaded)

| Source | Protocol | URL | Frequency | Auth |
|--------|----------|-----|-----------|------|
| Go Vuln DB | Web + API | `https://pkg.go.dev/vuln/` | Daily | None |
| govulncheck | CLI | `govulncheck ./...` (local) | Every build | None |

**Note**: `pkg.go.dev/vuln/` is the official Go vulnerability database. Cross-reference with `govulncheck` output during CI/CD. Any Go vuln hitting our dependency tree is auto-P0.

### Tier 2: Vendor Advisories (Auto-Ingest, Medium Frequency)

| Source | Protocol | URL | Frequency | Auth |
|--------|----------|-----|-----------|------|
| Red Hat Security | OVAL + RSS | `https://access.redhat.com/security/data` | Every 6 hours | None |
| Juniper Security | RSS | `https://kb.juniper.net/InfoCenter/?rss=true` | Daily | None |
| Ubuntu/Canonical | Aggregated | `https://linuxsecurity.com/advisories/ubuntu` | Every 6 hours | None |
| Zero Day Initiative | RSS | `https://www.zerodayinitiative.com/rss/` | Daily | None |
| FortiGuard | RSS | `https://www.fortiguard.com/rss-feeds` | Daily | None |

### Tier 3: Threat Intelligence (Structured, Lower Frequency)

| Source | Protocol | Access | Frequency | Auth |
|--------|----------|--------|-----------|------|
| AlienVault OTX | STIX/TAXII + REST | `https://otx.alienvault.com/taxii/` | Real-time | API Key |
| Shadowserver | Reports | Registration-based | Daily | Registration |

**Note**: CISA retired RSS feeds on May 12, 2025. All CISA data now via JSON API only.

---

## Key Management

### Ed25519 Key Pair Generation

```bash
# Generate key pair for feed reader service identity
ssh-keygen -t ed25519 -f /tmp/feed-reader-ed25519 -C "feed-reader@unheaded.kingdom" -N ""

# Public key → register with feed providers requiring SSH auth
cat /tmp/feed-reader-ed25519.pub

# Private key → encrypt with sops and store in repo
sops --encrypt /tmp/feed-reader-ed25519 > secrets/feed-reader-key.enc.yaml

# DESTROY plaintext immediately
shred -u /tmp/feed-reader-ed25519
```

### sops-nix Secret Configuration

```nix
# In NixOS configuration
{
  sops = {
    defaultSopsFile = ./secrets/secrets.yaml;

    # Use existing SSH Ed25519 host keys for age encryption
    age.sshKeyPaths = [ "/etc/ssh/ssh_host_ed25519_key" ];

    secrets = {
      miniflux_admin_credentials = {
        sopsFile = ./secrets/miniflux.yaml;
        owner = "miniflux";
        mode = "0400";  # Read-only by owner
      };

      nvd_api_key = {
        sopsFile = ./secrets/api-keys.yaml;
        owner = "threat-intel";
        mode = "0400";
      };

      otx_api_key = {
        sopsFile = ./secrets/api-keys.yaml;
        owner = "threat-intel";
        mode = "0400";
      };

      feed_reader_private_key = {
        sopsFile = ./secrets/feed-reader-key.yaml;
        owner = "threat-intel";
        mode = "0400";
      };
    };
  };
}
```

### Secret Storage Rules

1. **NEVER** store plaintext keys in the repository
2. **NEVER** log API keys or tokens
3. **ALWAYS** use sops-nix for NixOS secret provisioning
4. **ALWAYS** set file permissions to 0400 (owner read-only)
5. **ROTATE** API keys quarterly (calendar reminder)
6. **AUDIT** key access via sops audit log

---

## NixOS Container Configuration

### Miniflux Aggregator Container

```nix
containers.miniflux-aggregator = {
  autoStart = true;
  privateNetwork = true;
  hostBridge = "vxlan0";
  localAddress = "192.168.100.10/24";

  config = { config, pkgs, ... }: {
    services.postgresql = {
      enable = true;
      package = pkgs.postgresql_16;
    };

    services.miniflux = {
      enable = true;
      config = {
        LISTEN_ADDR = "0.0.0.0:8080";
        DATABASE_URL = "postgres://miniflux@localhost/miniflux?sslmode=disable";
        RUN_MIGRATIONS = "1";
        CREATE_ADMIN = "1";
        LOG_LEVEL = "info";
        POLLING_FREQUENCY = "60";       # Minutes between feed checks
        BATCH_SIZE = "100";             # Entries per batch
        CLEANUP_FREQUENCY_HOURS = "24"; # Daily cleanup
        CLEANUP_ARCHIVE_READ_DAYS = "90"; # Keep 90 days
      };
      adminCredentialsFile = config.sops.secrets.miniflux_admin_credentials.path;
    };

    networking.firewall.allowedTCPPorts = [ 8080 ];
  };
};
```

### Threat Intelligence Worker Container

```nix
containers.threat-intel-worker = {
  autoStart = true;
  privateNetwork = true;
  hostBridge = "vxlan0";
  localAddress = "192.168.100.20/24";

  config = { config, pkgs, ... }: {
    environment.systemPackages = with pkgs; [
      python3
      python3Packages.requests
      python3Packages.pyyaml
      python3Packages.miniflux  # Miniflux Python client
    ];

    services.cron = {
      enable = true;
      systemCronJobs = [
        # NVD API ingestion (every 2 hours)
        "0 */2 * * * threat-intel /opt/scripts/ingest-nvd.py"
        # CISA KEV (daily at 03:00)
        "0 3 * * * threat-intel /opt/scripts/ingest-cisa-kev.py"
        # Vendor RSS feeds (every 6 hours)
        "0 */6 * * * threat-intel /opt/scripts/ingest-vendor-feeds.py"
        # Critical alert check (hourly)
        "0 * * * * threat-intel /opt/scripts/check-critical-cves.py"
      ];
    };

    networking.firewall.allowedTCPPorts = [ 443 ];
  };
};
```

---

## MoatGhost Integration

The threat intelligence register feeds directly into MoatGhost's compliance and audit functions:

- **BlackMage** receives: New CVE targets for red team assessment
- **Architect** receives: Hardening recommendations based on vendor advisories
- **Developer** receives: Patch priorities for dependencies (govulncheck correlation)
- **Captain** receives: Risk dashboard updates for strategic decisions
- **Micromanager** receives: Sprint priority adjustments when critical CVEs land

### Alert Thresholds

| CVSS Score | Action | Owner |
|------------|--------|-------|
| 9.0 - 10.0 | IMMEDIATE: Wotan broadcast to all services | MoatGhost → All |
| 7.0 - 8.9 | HIGH: Add to next sprint P0 | MoatGhost → Micromanager |
| 4.0 - 6.9 | MEDIUM: Track in register, review weekly | MoatGhost → Architect |
| 0.1 - 3.9 | LOW: Log only | MoatGhost |

---

## Key Provisioning Runbook

### Initial Setup

```bash
# 1. Register for NVD API key
# https://nvd.nist.gov/developers/start-here
# Store key in secrets/api-keys.yaml via sops

# 2. Register for AlienVault OTX
# https://otx.alienvault.com/
# Store API key in secrets/api-keys.yaml via sops

# 3. Generate Ed25519 key pair
ssh-keygen -t ed25519 -f /tmp/feed-reader -C "feed-reader@unheaded" -N ""
sops --encrypt --age $(age-keygen -y /var/lib/sops-nix/key.txt) \
  /tmp/feed-reader > secrets/feed-reader-key.enc.yaml
shred -u /tmp/feed-reader

# 4. Create sops secrets file
cat > /tmp/api-keys.yaml << 'EOF'
nvd_api_key: <your-nvd-api-key>
otx_api_key: <your-otx-api-key>
miniflux_admin: admin:<strong-password>
EOF
sops --encrypt /tmp/api-keys.yaml > secrets/api-keys.yaml
shred -u /tmp/api-keys.yaml

# 5. Deploy containers
nixos-rebuild switch

# 6. Verify
curl -H "X-Auth-Token: <api-key>" http://192.168.100.10:8080/v1/feeds
```

### Key Rotation (Quarterly)

```bash
# 1. Generate new API keys from providers
# 2. Update sops-encrypted secrets
sops secrets/api-keys.yaml  # Edit in-place, re-encrypts on save
# 3. Restart affected containers
systemctl restart container@miniflux-aggregator
systemctl restart container@threat-intel-worker
# 4. Verify feed ingestion resumes
# 5. Log rotation event in docs/security/key-rotation-log.md
```

---

## Deployment Checklist

- [ ] Register for NVD API key
- [ ] Register for AlienVault OTX account
- [ ] Generate Ed25519 key pair via sops-nix
- [ ] Create sops-encrypted secrets files
- [ ] Deploy miniflux-aggregator NixOS container
- [ ] Deploy threat-intel-worker NixOS container
- [ ] Configure Miniflux with all Tier 1 + Tier 2 feeds
- [ ] Verify cron jobs execute on schedule
- [ ] Test critical CVE alert pipeline (CVSS > 9.0)
- [ ] Validate MoatGhost integration (threat register populated)
- [ ] Set quarterly key rotation calendar reminder
- [ ] Document any feed-specific access requirements

---

_The Ghost sees the cracks. The feeds never sleep._
_Documented at Round Table S26 — February 20, 2026_
