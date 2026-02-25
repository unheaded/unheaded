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

        # vLLM systemd service (DeepSeek-R1 7B)
        systemd.services.vllm-deepseek = {
          description = "vLLM Inference Server - DeepSeek-R1 7B";
          after = [ "network.target" ];
          wantedBy = [ "multi-user.target" ];

          environment = {
            VLLM_WORKER_MULTIPROC_METHOD = "spawn";
            HIP_VISIBLE_DEVICES = "0";
            ROCR_VISIBLE_DEVICES = "0";
            TORCH_ALLOW_TF32 = "1";
            VLLM_ATTENTION_BACKEND = "flashinfer";
          };

          serviceConfig = {
            Type = "simple";
            User = "root";
            WorkingDirectory = "/var/lib/sophia-eye";
            ExecStart = ''
              ${pkgs.python3}/bin/python -m vllm.entrypoints.openai.api_server \
                --model /data/models/deepseek-r1-7b \
                --host 0.0.0.0 \
                --port 8000 \
                --max-model-len 32768 \
                --gpu-memory-utilization 0.85 \
                --max-num-seqs 256 \
                --enable-prefix-caching \
                --trust-remote-code \
                --dtype float16
            '';
            Restart = "on-failure";
            RestartSec = "10s";
            StandardOutput = "journal";
            StandardError = "journal";
            TimeoutStartSec = "120s";
          };
        };

        # vLLM systemd service (Qwen 2.5 Coder 7B)
        systemd.services.vllm-qwen = {
          description = "vLLM Inference Server - Qwen 2.5 Coder 7B";
          after = [ "network.target" ];
          wantedBy = [ "multi-user.target" ];

          environment = {
            VLLM_WORKER_MULTIPROC_METHOD = "spawn";
            HIP_VISIBLE_DEVICES = "0";
            ROCR_VISIBLE_DEVICES = "0";
            TORCH_ALLOW_TF32 = "1";
            VLLM_ATTENTION_BACKEND = "flashinfer";
          };

          serviceConfig = {
            Type = "simple";
            User = "root";
            WorkingDirectory = "/var/lib/sophia-eye";
            ExecStart = ''
              ${pkgs.python3}/bin/python -m vllm.entrypoints.openai.api_server \
                --model /data/models/qwen2.5-coder-7b \
                --host 0.0.0.0 \
                --port 8001 \
                --max-model-len 16384 \
                --gpu-memory-utilization 0.80 \
                --max-num-seqs 128 \
                --enable-prefix-caching \
                --trust-remote-code \
                --dtype float16
            '';
            Restart = "on-failure";
            RestartSec = "10s";
            StandardOutput = "journal";
            StandardError = "journal";
            TimeoutStartSec = "120s";
          };
        };

        # Sophia Gateway service
        systemd.services.sophia-gateway = {
          description = "Sophia's Eye Gateway - AI Service Integration";
          after = [ "network.target" "vllm-deepseek.service" "vllm-qwen.service" "qdrant.service" ];
          wants = [ "vllm-deepseek.service" "vllm-qwen.service" "qdrant.service" ];
          wantedBy = [ "multi-user.target" ];

          environment = {
            UNHEADED_PROTOCOL_API = "http://bare-metal:17100";
            WOTAN_ENDPOINT = "bare-metal:18001";
            VLLM_ENDPOINT = "http://localhost:8000";
            QWEN_ENDPOINT = "http://localhost:8001";
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
          20100  # vLLM (DeepSeek)
          20101  # vLLM (Qwen)
          20102  # Qdrant HTTP
          20103  # Qdrant gRPC
          20104  # BGE-M3 embeddings
          20105  # Sophia's Eye gateway
          6333   # Qdrant (internal)
          6334   # Qdrant gRPC (internal)
          8000   # vLLM internal
          8001   # vLLM internal (Qwen)
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
