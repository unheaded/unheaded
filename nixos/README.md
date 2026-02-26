# Unheaded NixOS Infrastructure

Declarative NixOS configuration for the Unheaded Kingdom bare metal deployment.

## Host Layout

| Host | Role | Specs | IPv6 |
|------|------|-------|------|
| `host-a` (forge) | Full stack + vLLM/ROCm | 16+ cores, 64GB, RX 7700 XT | `fd00:dead:beef:0001::1/64` |
| `host-b` (outpost) | Minimal suite + WireGuard | 8+ cores, 8GB | `fd00:dead:beef:0002::1/64` |

## Directory Structure

```
nixos/
├── hosts/
│   ├── host-a/
│   │   ├── configuration.nix          # Forge: full stack
│   │   └── hardware-configuration.nix # Generated on hardware (not committed)
│   └── host-b/
│       ├── configuration.nix          # Outpost: minimal suite
│       └── hardware-configuration.nix # Generated on hardware (not committed)
└── modules/
    ├── unheaded-base.nix    # Shared baseline (security, nix, cgroups)
    ├── telemetry.nix        # Prometheus + VictoriaMetrics + Loki + Grafana
    ├── wireguard.nix        # WireGuard east-west + IPv6 /48 addressing
    ├── vllm-rocm.nix        # vLLM + ROCm (Host-A only)
    ├── ebpf-exporter.nix    # eBPF → Prometheus bridge
    └── services/            # One .nix per Unheaded service
        ├── monad.nix
        ├── sophia.nix
        └── ...
```

## Deployment (Bare Metal)

1. Boot NixOS installer ISO
2. Partition and format drives (see S61 battle plan)
3. Mount at /mnt: `mount /dev/disk/by-label/nixos /mnt`
4. Copy this config: `cp -r nixos/hosts/host-a/* /mnt/etc/nixos/`
5. Generate hardware config: `nixos-generate-config --root /mnt`
6. Install: `nixos-install`
7. Reboot

## Module Dependencies

- All hosts import `unheaded-base.nix` → security hardening, nix config, cgroup v2
- All hosts import `telemetry.nix` → node_exporter, Prometheus remote_write to Host-A
- All hosts import `wireguard.nix` → WireGuard wg0 interface
- Host-A only: `vllm-rocm.nix`, `ebpf-exporter.nix`, full service suite
- Host-B only: minimal service subset (monad, wotan, anamnesis, shield, unheaded-daemon, service-discovery)

## Environment Variables (set in each service's systemd unit)

```
UNHEADED_HOST_ROLE=forge|outpost
UNHEADED_WG_IPV6=fd00:dead:beef:wg::1/64
PROMETHEUS_URL=http://localhost:9090
WOTAN_ADDR=fd00:dead:beef:wg::1:50053
```
