# NixOS module for zhend — Zhen Layer 0 daemon.
# 真 — Declarative deployment of the Library that cannot burn.
#
# Usage in configuration.nix:
#
#   imports = [ ./path/to/zhend/nix/module.nix ];
#
#   services.zhend = {
#     enable = true;
#     dataDir = "/var/lib/zhend";
#     grpcAddr = "[::1]:7300";
#     quicAddr = "[::1]:7301";
#     gossipAddr = "[::]:7302";
#     seedPeers = [ "[fd00::2]:7302" "[fd00::3]:7302" ];
#     embeddingModel = null;  # path to ONNX model, or null
#     logFormat = "json";     # "json" or "pretty"
#   };

{ config, lib, pkgs, ... }:

let
  cfg = config.services.zhend;
in
{
  options.services.zhend = {
    enable = lib.mkEnableOption "zhend — Zhen Layer 0 knowledge substrate";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.zhend or (throw "zhend package not in nixpkgs — build from source");
      description = "The zhend package to use.";
    };

    dataDir = lib.mkOption {
      type = lib.types.path;
      default = "/var/lib/zhend";
      description = "Data directory for all storage tiers.";
    };

    grpcAddr = lib.mkOption {
      type = lib.types.str;
      default = "[::1]:7300";
      description = "gRPC listen address (internal Kingdom services).";
    };

    quicAddr = lib.mkOption {
      type = lib.types.str;
      default = "[::1]:7301";
      description = "QUIC/HTTP3 listen address (external/edge access).";
    };

    gossipAddr = lib.mkOption {
      type = lib.types.str;
      default = "[::]:7302";
      description = "Gossip listen address (UDP, Qi propagation).";
    };

    seedPeers = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [];
      description = "Seed peers for initial gossip mesh join.";
    };

    embeddingModel = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to ONNX embedding model. Null disables De (relevance).";
    };

    embeddingDims = lib.mkOption {
      type = lib.types.int;
      default = 384;
      description = "Embedding vector dimensions. Must match model.";
    };

    gossipFanout = lib.mkOption {
      type = lib.types.int;
      default = 3;
      description = "Number of peers per gossip cycle.";
    };

    gossipIntervalMs = lib.mkOption {
      type = lib.types.int;
      default = 1000;
      description = "Gossip cycle interval in milliseconds.";
    };

    l1ToL2Secs = lib.mkOption {
      type = lib.types.int;
      default = 3600;
      description = "L1→L2 sedimentation threshold (seconds).";
    };

    l2ToL3Secs = lib.mkOption {
      type = lib.types.int;
      default = 604800;
      description = "L2→L3 sedimentation threshold (seconds).";
    };

    logFormat = lib.mkOption {
      type = lib.types.enum [ "json" "pretty" ];
      default = "json";
      description = "Log output format.";
    };

    user = lib.mkOption {
      type = lib.types.str;
      default = "zhend";
      description = "System user to run zhend as.";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "zhend";
      description = "System group to run zhend as.";
    };
  };

  config = lib.mkIf cfg.enable {
    # System user/group.
    users.users.${cfg.user} = {
      isSystemUser = true;
      group = cfg.group;
      home = cfg.dataDir;
      description = "Zhen Layer 0 daemon";
    };

    users.groups.${cfg.group} = {};

    # Systemd service — hardened.
    systemd.services.zhend = {
      description = "zhend — Zhen Layer 0 Anti-Fragile Knowledge Substrate";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];

      serviceConfig = {
        Type = "simple";
        User = cfg.user;
        Group = cfg.group;

        ExecStart = lib.concatStringsSep " " ([
          "${cfg.package}/bin/zhend"
          "--data-dir ${cfg.dataDir}"
          "--grpc-addr '${cfg.grpcAddr}'"
          "--quic-addr '${cfg.quicAddr}'"
          "--gossip-addr '${cfg.gossipAddr}'"
          "--l1-to-l2-secs ${toString cfg.l1ToL2Secs}"
          "--l2-to-l3-secs ${toString cfg.l2ToL3Secs}"
          "--embedding-dims ${toString cfg.embeddingDims}"
          "--gossip-fanout ${toString cfg.gossipFanout}"
          "--gossip-interval-ms ${toString cfg.gossipIntervalMs}"
          "--log-format ${cfg.logFormat}"
        ]
        ++ (map (p: "--seed-peer '${p}'") cfg.seedPeers)
        ++ (lib.optional (cfg.embeddingModel != null)
              "--embedding-model '${cfg.embeddingModel}'"));

        Restart = "on-failure";
        RestartSec = "5s";

        # Hardening — defense in depth.
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictAddressFamilies = [ "AF_INET6" "AF_UNIX" ];  # IPv6 only + unix sockets
        RestrictNamespaces = true;
        RestrictSUIDSGID = true;
        MemoryDenyWriteExecute = true;
        LockPersonality = true;
        SystemCallFilter = [ "@system-service" "~@privileged" ];
        ReadWritePaths = [ cfg.dataDir ];

        # Watchdog.
        WatchdogSec = "60s";

        # Resource limits.
        LimitNOFILE = 65536;
        LimitNPROC = 64;
      };

      # Ensure data directory exists with correct ownership.
      preStart = ''
        mkdir -p ${cfg.dataDir}/keys
        chmod 700 ${cfg.dataDir}/keys
      '';
    };

    # Firewall rules — open gossip port if firewall is enabled.
    networking.firewall.allowedUDPPorts =
      lib.mkIf config.networking.firewall.enable
        [ 7302 ];  # gossip UDP
  };
}
