# scripts/doom/ -- Doom-over-IPv6 Tooling (Legacy)

> **NOTE:** These scripts are from the pre-doom-runner era. The current pipeline
> uses `doom-runner` (crates/doom-runner/) which handles ring setup, eBPF loading,
> map population, and bridge serving in a single binary. See `docs/doom/RUNBOOK.md`
> for current operational instructions.

Hardened, argparse-equipped scripts for running and debugging the Doom-over-IPv6 PoC.

## Python Tools

| Script | Purpose | Usage |
|--------|---------|-------|
| `load_rom.py` | Load ROM/RV2MBC into BPF maps | `sudo python3 load_rom.py doom.mbc [--rv2mbc rv2mbc.bin] [--dry-run]` |
| `inject.py` | Packet injection (steady/burst/fast) | `sudo ip netns exec monad0 python3 inject.py -n 5000 --mode burst` |
| `cpu_state.py` | Read/reset/dump CPU state | `sudo python3 cpu_state.py read [--json]` |

## Shell Scripts

| Script | Purpose |
|--------|---------|
| `ring.sh` | Create/teardown 6-namespace doom ring |
| `test.sh` | Full Doom integration test suite |
| `loader.sh` | ROM/RV2MBC loader wrapper |

## Quick Start

```bash
# 1. Set up doom ring (6 namespaces, XDP attached)
sudo ./scripts/doom/ring.sh setup

# 2. Load ROM
sudo python3 scripts/doom/load_rom.py doom/doom.mbc --rv2mbc doom/rv2mbc.bin

# 3. Reset CPU
sudo python3 scripts/doom/cpu_state.py reset

# 4. Inject packets (run Doom)
sudo ip netns exec monad0 python3 scripts/doom/inject.py -n 5000 --mode burst

# 5. Check CPU state
sudo python3 scripts/doom/cpu_state.py read

# 6. Teardown
sudo ./scripts/doom/ring.sh teardown
```

## Migration History

Migrated from `scripts/` flat directory during S33 hardening sprint (Feb 23, 2026).
Original ad-hoc scripts consolidated and hardened with:
- `argparse` CLI interfaces
- Signal handling (SIGINT/SIGTERM)
- JSON output mode
- Error handling and validation
- `--dry-run` support where applicable
