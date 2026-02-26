# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# vllm-rocm.nix — vLLM inference server + AMD ROCm (Host-A / Forge only)
# Hardware: AMD RX 7700 XT (gfx1101 / RDNA3), 12GB VRAM
# Model: DeepSeek-R1-7B-Q4 (~4.2GB VRAM)
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.vllmRocm;
in {
  options.services.unheaded.vllmRocm = {
    enable = mkEnableOption "Unheaded vLLM ROCm inference server";
    
    model = mkOption {
      type = types.str;
      default = "deepseek-ai/DeepSeek-R1-Distill-Qwen-7B";
      description = "HuggingFace model ID or local path";
    };
    
    quantization = mkOption {
      type = types.enum [ "awq" "gptq" "fp8" "none" ];
      default = "awq";
      description = "Quantization method (awq recommended for Q4 on RDNA3)";
    };
    
    port = mkOption {
      type = types.port;
      default = 8000;
      description = "vLLM HTTP server port (OpenAI-compatible API)";
    };
    
    maxModelLen = mkOption {
      type = types.int;
      default = 8192;
      description = "Maximum model context length";
    };
    
    gpuMemoryUtilization = mkOption {
      type = types.float;
      default = 0.85;
      description = "Fraction of GPU VRAM to use (0.0-1.0)";
    };
    
    maxNumSeqs = mkOption {
      type = types.int;
      default = 4;
      description = "Maximum concurrent sequences";
    };
    
    modelDir = mkOption {
      type = types.path;
      default = "/var/lib/unheaded/models";
      description = "Local model cache directory";
    };
    
    cpuQuota = mkOption {
      type = types.str;
      default = "800%";
      description = "CPU quota (cgroup v2) — 800% = 8 cores max";
    };
    
    memoryMax = mkOption {
      type = types.str;
      default = "14G";
      description = "Maximum RAM allocation (cgroup v2)";
    };
  };

  config = mkIf cfg.enable {
    # ── ROCm hardware support ───────────────────────────────────────────────
    hardware.opengl = {
      enable = true;
      driSupport = true;
      extraPackages = with pkgs; [
        rocm-opencl-icd
        rocm-opencl-runtime
        rocmPackages.rocm-runtime
        rocmPackages.rocm-smi
        rocmPackages.hip
        rocmPackages.clr
      ];
    };
    
    # ROCm symlink (required by HIP applications)
    systemd.tmpfiles.rules = [
      "L+ /opt/rocm/hip - - - - ${pkgs.rocmPackages.hip}"
      "d /var/lib/unheaded/models 0750 vllm vllm -"
      "d /var/log/unheaded/vllm   0750 vllm vllm -"
    ];
    
    # ── vLLM systemd service ────────────────────────────────────────────────
    systemd.services.unheaded-vllm = {
      description = "Unheaded vLLM ROCm Inference Server (${cfg.model})";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" "local-fs.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "vllm";
        Group = "render";  # AMD GPU group
        SupplementaryGroups = [ "video" ];
        
        ExecStart = let
          vllmArgs = concatStringsSep " " [
            "--model ${cfg.model}"
            "--port ${toString cfg.port}"
            "--host 0.0.0.0"
            "--max-model-len ${toString cfg.maxModelLen}"
            "--gpu-memory-utilization ${toString cfg.gpuMemoryUtilization}"
            "--max-num-seqs ${toString cfg.maxNumSeqs}"
            "--download-dir ${cfg.modelDir}"
            "--quantization ${cfg.quantization}"
            "--dtype float16"
            "--disable-log-requests"
            "--trust-remote-code"
          ];
        in "${pkgs.python3}/bin/python3 -m vllm.entrypoints.openai.api_server ${vllmArgs}";
        
        Restart = "on-failure";
        RestartSec = "30s";
        TimeoutStartSec = "300s";  # Model loading can take time
        
        # cgroup v2 resource limits (isolation from Unheaded services)
        CPUAccounting    = true;
        MemoryAccounting = true;
        IOAccounting     = true;
        CPUQuota         = cfg.cpuQuota;
        MemoryMax        = cfg.memoryMax;
        MemorySwapMax    = "0";  # No swap for ML workloads
        
        # Security hardening
        NoNewPrivileges  = false;  # vLLM needs some privileges for GPU
        PrivateTmp       = true;
        ProtectSystem    = "full";
        ReadWritePaths   = [ cfg.modelDir "/var/log/unheaded/vllm" ];
        
        # GPU device access
        DeviceAllow = [
          "/dev/kfd rw"     # AMD KFD (Kernel Fusion Driver)
          "/dev/dri rw"     # DRM devices
          "/dev/shm rw"     # Shared memory
        ];
        DevicePolicy = "closed";
      };
      
      environment = {
        # ROCm configuration
        ROCR_VISIBLE_DEVICES    = "0";
        HIP_VISIBLE_DEVICES     = "0";
        HSA_OVERRIDE_GFX_VERSION = "11.0.1";  # RX 7700 XT = gfx1101
        
        # vLLM / HuggingFace
        HF_HOME                 = cfg.modelDir;
        TRANSFORMERS_CACHE       = cfg.modelDir;
        PYTORCH_HIP_ALLOC_CONF  = "garbage_collection_threshold:0.8,max_split_size_mb:512";
        
        # Unheaded integration
        VLLM_PORT               = toString cfg.port;
        UNHEADED_VLLM_ENDPOINT  = "http://localhost:${toString cfg.port}/v1";
      };
      
      # Metrics endpoint (Prometheus-compatible via vLLM built-in /metrics)
      # vLLM exposes: vllm:gpu_cache_usage_perc, vllm:num_requests_running, etc.
    };
    
    # ── User + group ────────────────────────────────────────────────────────
    users.users.vllm = {
      isSystemUser = true;
      group = "vllm";
      extraGroups = [ "render" "video" ];
      home = "/var/lib/unheaded/models";
      createHome = false;
    };
    users.groups.vllm = {};
    
    # ── Firewall ────────────────────────────────────────────────────────────
    networking.firewall.allowedTCPPorts = [ cfg.port ];
    
    # ── Packages ────────────────────────────────────────────────────────────
    environment.systemPackages = with pkgs; [
      rocmPackages.rocm-smi
      rocmPackages.rocminfo
      python3
      python3Packages.pip
    ];
  };
}
