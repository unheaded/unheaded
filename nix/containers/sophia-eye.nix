{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.containers.sophia-eye;
  sophia-gateway-src = ./../../deploy/sophia-eye/sophia-gateway;
in

{
  options.containers.sophia-eye = {
    enable = mkEnableOption "Sophia's Eye - AI Model Stack container";

    autoStart = mkOption {
      type = types.bool;
      default = true;
      description = "Automatically start the container on boot";
    };

    dataPath = mkOption {
      type = types.str;
      default = "/data/models";
      description = "Path to model weights directory (2TB HDD)";
    };

    fastPath = mkOption {
      type = types.str;
      default = "/fast/qdrant";
      description = "Path to vector index storage (1TB NVMe)";
    };
  };

  config = mkIf (config.containers.sophia-eye.enable or true) {
    # Container definition for Sophia's Eye
    containers.sophia-eye = {
      autoStart = cfg.autoStart;
      privateNetwork = false;

      bindMounts = {
        "/data/models" = {
          hostPath = cfg.dataPath;
          isReadOnly = true;
        };
        "/fast/qdrant" = {
          hostPath = cfg.fastPath;
          isReadOnly = false;
        };
        "/var/lib/sophia-eye" = {
          hostPath = "/var/lib/sophia-eye";
          isReadOnly = false;
        };
      };

      config = { config, pkgs, ... }: {

        # System packages for AI stack
        environment.systemPackages = with pkgs; [
          curl
          wget
          htop
          tmux
          vim
          jq
          git
          rocm-opencl-icd
          rocm-runtime
          rocm-core
          rocm-device-libs
          hip
          hipblas
          miopen
          miopengemm
          composable_kernel
          python3
          python3Packages.pip
          python3Packages.numpy
          python3Packages.torch
          docker
          docker-compose
        ];

        # Enable Docker daemon in container
        virtualisation.docker.enable = true;
        virtualisation.docker.daemon.settings = {
          storage-driver = "overlay2";
          insecure-registries = [ ];
          registry-mirrors = [ ];
          log-driver = "json-file";
          log-opts = {
            max-size = "100m";
            max-file = "3";
          };
        };

        # Environment variables for ROCm and AI stack
        environment.variables = {
          ROCM_HOME = "${pkgs.rocmPackages.rocm-core}";
          HIP_PLATFORM = "amd";
          VLLM_WORKER_MULTIPROC_METHOD = "spawn";
          TORCH_HIP_USER_DEVICE_MEMORY = "100G";
        };

        # ROCm and GPU configuration
        services.rocm = {
          enable = true;
          gpuTargets = [ "gfx1032" ];  # RDNA 2 (RX 7700 XT)
          rocmPackages = with pkgs.rocmPackages; [
            rocm-runtime
            rocm-opencl-icd
            rocm-device-libs
            rocminfo
            rocm-smi
          ];
        };

        # Qdrant service configuration
        services.qdrant = {
          enable = true;

          dataDir = "/fast/qdrant";

          settings = {
            server = {
              http_port = 6333;
              grpc_port = 6334;
              max_request_size_mb = 200;
            };

            storage = {
              snapshots_path = "/fast/qdrant/snapshots";
              wal = {
                enabled = true;
              };
            };

            cluster = {
              enabled = false;
            };

            log_level = "info";
          };
        };

        # Sophia Gateway service
        systemd.services.sophia-gateway = {
          description = "Sophia's Eye Gateway - AI Service Integration";
          after = [ "network.target" "qdrant.service" ];
          wants = [ "qdrant.service" ];
          wantedBy = [ "multi-user.target" ];

          environment = {
            UNHEADED_PROTOCOL_API = "http://bare-metal:17100";
            WOTAN_ENDPOINT = "bare-metal:18001";
            # Both names reach the one llama-server on the host (privateNetwork
            # is false); sophia-gateway reads them, renaming is ADR-117's.
            VLLM_ENDPOINT = "http://127.0.0.1:8081";
            QWEN_ENDPOINT = "http://127.0.0.1:8081";
            QDRANT_ENDPOINT = "http://localhost:6333";
            EMBEDDINGS_ENDPOINT = "http://localhost:8002";
            SOPHIA_SERVICE_NAME = "sophia-eye";
            SOPHIA_GATEWAY_PORT = "20105";
            METRICS_PORT = "9090";
            LOG_LEVEL = "info";
          };

          serviceConfig = {
            Type = "simple";
            User = "root";
            WorkingDirectory = "/var/lib/sophia-eye";
            ExecStart = "${pkgs.go}/bin/go run ${sophia-gateway-src}/main.go";
            Restart = "on-failure";
            RestartSec = "15s";
            StandardOutput = "journal";
            StandardError = "journal";
            TimeoutStartSec = "30s";
            TimeoutStopSec = "10s";
            SyslogIdentifier = "sophia-gateway";
          };
        };

        # BGE-M3 embeddings service
        systemd.services.embeddings-bge = {
          description = "BGE-M3 Embeddings Service";
          after = [ "network.target" ];
          wantedBy = [ "multi-user.target" ];

          serviceConfig = {
            Type = "simple";
            User = "root";
            WorkingDirectory = "/var/lib/sophia-eye";
            ExecStart = ''
              ${pkgs.python3}/bin/python -m text_embeddings_inference.server \
                --model-id /data/models/bge-m3 \
                --port 8002 \
                --workers 4
            '';
            Restart = "on-failure";
            RestartSec = "10s";
            StandardOutput = "journal";
            StandardError = "journal";
            TimeoutStartSec = "60s";
          };
        };

        # VXLAN interface configuration (Wotan fabric)
        networking.interfaces.vxlan0 = {
          useDHCP = false;
          ipv4.addresses = [
            { address = "172.16.100.2"; prefixLength = 24; }
          ];
          ipv4.routes = [
            { address = "172.16.0.0"; prefixLength = 16; via = "172.16.100.1"; }
          ];
        };

        # VLAN for bare metal connectivity
        networking.vlans.vlan200 = {
          interface = "eth0";
          id = 200;
        };

        networking.interfaces.vlan200 = {
          useDHCP = false;
          ipv4.addresses = [
            { address = "192.168.200.2"; prefixLength = 24; }
          ];
        };

        # Firewall rules for AI services
        networking.firewall.enable = true;
        networking.firewall.allowedTCPPorts = [
          20102  # Qdrant HTTP
          20103  # Qdrant gRPC
          20104  # BGE-M3 embeddings
          20105  # Sophia's Eye gateway
          6333   # Qdrant (internal)
          6334   # Qdrant gRPC (internal)
          8002   # Embeddings internal
          9090   # Prometheus metrics
        ];

        networking.firewall.allowedUDPPorts = [
          4789   # VXLAN
          4790   # VXLAN backup
        ];

        # Logging configuration
        services.journald.extraConfig = ''
          SystemMaxUse=5G
          SystemMaxFileSize=500M
          MaxRetentionSec=30day
        '';

        # NTP for time synchronization
        services.chrony.enable = true;

        # Basic security hardening
        security.acme.acceptTerms = false;

        system.stateVersion = "24.05";
      };
    };

    # Host-level VXLAN configuration for gaming desktop
    networking.interfaces = {
      vxlan0 = {
        useDHCP = false;
        ipv4.addresses = [
          { address = "172.16.100.1"; prefixLength = 24; }
        ];
      };
    };

    # Enable VXLAN tunnel to bare metal
    boot.kernel.sysctl = {
      "net.bridge.bridge-nf-call-iptables" = 1;
      "net.bridge.bridge-nf-call-ip6tables" = 1;
      "net.ipv4.ip_forward" = 1;
      "net.ipv6.conf.all.forwarding" = 1;
    };

    # Monitoring and observability hooks
    environment.etc."sophia-eye/environment" = {
      text = ''
        SOPHIA_SERVICE_NAME=sophia-eye
        SOPHIA_CONTAINER_VERSION=1.0.0
        UNHEADED_KINGDOM=head
      '';
    };
  };
}
