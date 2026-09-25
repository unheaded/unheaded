# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# Suricata IDS/IPS NixOS module — GPL-2.0 isolation boundary: EVE JSON REST API
# Source: ~/tmp/suricata/ (built from source on bare metal — NOT in this module)
{ config, lib, pkgs, ... }:

let
  cfg = config.services.unheaded.suricata;
in {
  options.services.unheaded.suricata = {
    enable = lib.mkEnableOption "Suricata IDS/IPS integration";
    
    interface = lib.mkOption {
      type = lib.types.str;
      default = "eth1";
      description = "Network interface for AF_PACKET capture";
    };
    
    homeNet = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = ["10.20.0.0/16" "fd00:dead:beef::/48"];
      description = "HOME_NET definition for Suricata rules";
    };
    
    logDir = lib.mkOption {
      type = lib.types.path;
      default = "/var/log/suricata";
      description = "Suricata log directory (EVE JSON lives here)";
    };
    
    idsMode = lib.mkOption {
      type = lib.types.enum ["alert" "drop"];
      default = "alert";
      description = "IDS mode (alert) or IPS inline mode (drop)";
    };
    
    monadRulesEnable = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Deploy Monad HbH protocol detection rules (sid 9000001-9000099)";
    };
    
    eveJsonPath = lib.mkOption {
      type = lib.types.path;
      default = "/var/log/suricata/eve.json";
      description = "Path to EVE JSON output (read by anamnesis suricata bridge)";
    };
  };

  config = lib.mkIf cfg.enable {
    # Suricata runs as dedicated user with only required capabilities
    users.users.suricata = {
      isSystemUser = true;
      group = "suricata";
      uid = 994;
      description = "Suricata IDS/IPS service user";
    };
    users.groups.suricata = { gid = 994; };
    
    # Log directory with proper ownership
    systemd.tmpfiles.rules = [
      "d ${cfg.logDir} 0750 suricata suricata -"
      "d /etc/suricata 0755 root root -"
      "d /etc/suricata/rules 0755 root root -"
      "d /run/suricata 0755 suricata suricata -"
    ];
    
    # Deploy Monad rules if enabled
    # Scoped to this one file's attribute path. It used to be
    # `environment.etc = lib.mkIf ... { ... };`, which collides with the
    # environment.etc."suricata/suricata.yaml" below: Nix rejected the module
    # ("attribute 'environment.etc' already defined"), so nothing importing it
    # (nixos/hosts/host-b) could evaluate.
    #
    # The canonical rules file, not an inline copy: the inline copy had
    # drifted from it, and both used `ipv6-exthdr`, which is not a Suricata
    # keyword, so no rule ever loaded. scripts/check-suricata-rules.sh checks
    # that this still points at the canonical file.
    environment.etc."suricata/rules/unheaded-monad.rules" = lib.mkIf cfg.monadRulesEnable {
      source = ../../routing/suricata/rules/unheaded-monad.rules;
    };
    # Suricata 7 will not start its detection engine without these two once
    # rules are loaded (measured), and nothing else in this module installs them.
    environment.etc."suricata/classification.config" = lib.mkIf cfg.monadRulesEnable {
      source = ../../routing/suricata/rules/classification.config;
    };
    environment.etc."suricata/reference.config" = lib.mkIf cfg.monadRulesEnable {
      source = ../../routing/suricata/rules/reference.config;
    };
    
    # Main Suricata config
    environment.etc."suricata/suricata.yaml".text = ''
      %YAML 1.1
      ---
      # Suricata IDS/IPS Configuration — Unheaded Kingdom
      # CRITICAL: decode-events: no for hopopt — NEVER strip Monad HbH headers
      
      vars:
        address-groups:
          HOME_NET: "[${lib.concatStringsSep "," cfg.homeNet}]"
          EXTERNAL_NET: "!$HOME_NET"
        
        port-groups:
          DOOM_RANGE: "[16666:16689]"
          HTTP_PORTS: "80"
          HTTPS_PORTS: "443"
      
      # Decoder settings — CRITICAL for Monad HbH preservation
      decoder:
        # NEVER strip or alert on Monad HbH headers
        ipv6-hopopts: yes
        # Explicitly disable decode events for HOPOPT to prevent stripping
        decode-events: no
      
      # AF_PACKET capture (eBPF accelerated)
      af-packet:
        - interface: ${cfg.interface}
          threads: auto
          cluster-id: 99
          cluster-type: cluster_flow
          defrag: yes
          use-mmap: yes
          mmap-locked: yes
          tpacket-v3: yes
          ring-size: 2048
          block-size: 32768
          buffer-size: 67108864
          copy-mode: ${if cfg.idsMode == "drop" then "ias" else "none"}
          # eBPF bypass — shares BPF maps with Shield
          ebpf-lb-file: /etc/suricata/ebpf/xdp_filter.o
          xdp-mode: driver
          xdp-fail-mode: soft
      
      outputs:
        - eve-log:
            enabled: yes
            type: regular
            filename: ${cfg.eveJsonPath}
            rotate-interval: day
            types:
              - alert:
                  payload: yes
                  packet: yes
                  metadata: yes
              - anomaly:
                  enabled: yes
                  types:
                    decode: yes
                    stream: yes
              - stats:
                  enabled: yes
                  interval: 60
        
        - stats:
            enabled: yes
            filename: /var/log/suricata/stats.log
            interval: 60
            null-values: no
        
        - fast:
            enabled: yes
            filename: /var/log/suricata/fast.log
            append: yes
      
      logging:
        default-log-level: notice
        outputs:
          - console:
              enabled: yes
          - file:
              enabled: yes
              filename: /var/log/suricata/suricata.log
              level: info
      
      # Rule files
      default-rule-path: /etc/suricata/rules
      rule-files:
        - unheaded-monad.rules
      classification-file: /etc/suricata/classification.config
      reference-config-file: /etc/suricata/reference.config
      
      # Stats
      stats:
        enabled: yes
        interval: 60
      
      # Unix command socket
      unix-command:
        enabled: yes
        filename: /run/suricata/suricata.socket
    '';
    
    # Systemd service
    systemd.services.suricata = {
      description = "Suricata IDS/IPS — Unheaded Kingdom";
      wantedBy = ["multi-user.target"];
      after = ["network-online.target" "wg0.service" "unheaded-frr.service"];
      wants = ["network-online.target"];
      
      serviceConfig = {
        Type = "simple";
        User = "suricata";
        Group = "suricata";
        
        # Suricata binary must be built from ~/tmp/suricata/ on bare metal
        # Path set during nixos-rebuild switch by local overlay
        ExecStart = "${pkgs.suricata or "/usr/local/bin/suricata"}/bin/suricata " +
          "-c /etc/suricata/suricata.yaml " +
          "--af-packet=${cfg.interface} " +
          "--runmode=autofp " +
          (if cfg.idsMode == "drop" then "--inline" else "") + " " +
          "--unix-socket=/run/suricata/suricata.socket";
        
        ExecReload = "${pkgs.util-linux}/bin/kill -USR2 $MAINPID";
        
        Restart = "on-failure";
        RestartSec = "10s";
        
        # Capabilities — only what's needed for raw packet capture
        AmbientCapabilities = ["CAP_NET_RAW" "CAP_NET_ADMIN" "CAP_SYS_NICE"];
        CapabilityBoundingSet = ["CAP_NET_RAW" "CAP_NET_ADMIN" "CAP_SYS_NICE"];
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        PrivateDevices = false;  # Needs /dev/net/tun for AF_PACKET
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [cfg.logDir "/run/suricata"];
        ReadOnlyPaths = ["/etc/suricata"];
        
        LimitNOFILE = 65535;
        LimitNPROC = 4096;
      };
    };
    
    # Firewall — Suricata unix socket is local only, no port exposure needed
    # The EVE JSON path is read by anamnesis bridge — no extra firewall rules
    
    # CRITICAL: ensure HbH passthrough is maintained in nftables
    # (This is already done in firewall-bridge.nix — belt AND suspenders)
    networking.firewall.extraInputRules = lib.mkAfter ''
      # Monad HbH passthrough — belt-and-suspenders (primary rule in firewall-bridge.nix)
      ip6 nexthdr 0 accept comment "Monad HbH passthrough — Suricata module"
    '';
  };
}
